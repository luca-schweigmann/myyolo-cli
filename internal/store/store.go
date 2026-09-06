package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"

	_ "modernc.org/sqlite"
)

const schemaVersion = 5

var sessionTimeRangePattern = regexp.MustCompile(
	`^\s*([0-2][0-9]):([0-5][0-9])\s*[-–]\s*([0-2][0-9]):([0-5][0-9])\s*$`,
)

var adminTimeWindowPattern = regexp.MustCompile(
	`^\s*([0-2][0-9]):([0-5][0-9]).*([0-2][0-9]):([0-5][0-9])\s*$`,
)

type Store struct {
	db   *sql.DB
	path string
}

type ImportResult struct {
	RunID         int64 `json:"run_id"`
	Members       int   `json:"members"`
	Sessions      int   `json:"course_sessions"`
	Attendance    int   `json:"attendance"`
	Prescriptions int   `json:"prescriptions"`
}

type AdminImportResult struct {
	RunID       int64  `json:"run_id"`
	Route       string `json:"route"`
	Tables      int    `json:"tables"`
	Records     int    `json:"records"`
	Fingerprint string `json:"schema_fingerprint"`
}

type AdminCapabilityReport struct {
	Route             string `json:"route"`
	Title             string `json:"title"`
	TableCount        int    `json:"table_count"`
	RecordCount       int    `json:"record_count"`
	SchemaFingerprint string `json:"schema_fingerprint"`
	FirstObservedAt   string `json:"first_observed_at"`
	LastObservedAt    string `json:"last_observed_at"`
}

type AdminRecordReport struct {
	Route           string `json:"route"`
	TableIndex      int    `json:"table_index"`
	RecordHash      string `json:"record_hash"`
	ValuesJSON      string `json:"values_json"`
	FirstObservedAt string `json:"first_observed_at"`
	LastObservedAt  string `json:"last_observed_at"`
}

type AdminRehaHourReport struct {
	TimeWindow      string `json:"time_window"`
	Attendees       int    `json:"attendees"`
	DurationMinutes int    `json:"duration_minutes"`
	AttendeeMinutes int    `json:"attendee_minutes"`
}

type AdminCourseMonthReport struct {
	Month           string `json:"month"`
	AttendanceTotal int    `json:"attendance_total"`
	CoursesReported int    `json:"courses_reported"`
	LastObservedAt  string `json:"last_observed_at"`
}

type AdminMissingSignatureSummary struct {
	MembersWithMissing int    `json:"members_with_missing"`
	MissingTotal       int    `json:"missing_total"`
	LastObservedAt     string `json:"last_observed_at"`
}

type AdminRehaAttendanceDetail struct {
	MemberNumber string `json:"member_number"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	TimeWindow   string `json:"time_window"`
}

type AdminMissingSignatureDetail struct {
	MemberNumber string `json:"member_number"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Missing      int    `json:"missing"`
}

type DatabaseStatus struct {
	Path                string `json:"path"`
	SchemaVersion       int    `json:"schema_version"`
	Integrity           string `json:"integrity"`
	MySignMembers       int    `json:"mysign_members"`
	MySignSessions      int    `json:"mysign_sessions"`
	MySignAttendance    int    `json:"mysign_attendance"`
	AdminCapabilities   int    `json:"admin_capabilities"`
	AdminStructuredRows int    `json:"admin_structured_rows"`
	LastMySignSync      string `json:"last_mysign_sync,omitempty"`
	LastAdminDiscovery  string `json:"last_admin_discovery,omitempty"`
}

type Summary struct {
	Members              int    `json:"members"`
	CourseSessions       int    `json:"course_sessions"`
	AttendanceRows       int    `json:"attendance_rows"`
	Attended             int    `json:"attended"`
	Signed               int    `json:"signed"`
	Cancelled            int    `json:"cancelled"`
	MissingSignatures    int    `json:"missing_signatures"`
	NoShows              int    `json:"no_shows"`
	Pending              int    `json:"pending"`
	DistinctParticipants int    `json:"distinct_participants"`
	LastCompletedSync    string `json:"last_completed_sync,omitempty"`
}

type CourseReport struct {
	Course            string `json:"course"`
	Sessions          int    `json:"sessions"`
	Bookings          int    `json:"bookings"`
	Attended          int    `json:"attended"`
	Signed            int    `json:"signed"`
	Cancelled         int    `json:"cancelled"`
	MissingSignatures int    `json:"missing_signatures"`
	NoShows           int    `json:"no_shows"`
	Pending           int    `json:"pending"`
}

type DayReport struct {
	Date              string `json:"date"`
	Sessions          int    `json:"sessions"`
	Bookings          int    `json:"bookings"`
	Attended          int    `json:"attended"`
	Signed            int    `json:"signed"`
	Cancelled         int    `json:"cancelled"`
	MissingSignatures int    `json:"missing_signatures"`
	NoShows           int    `json:"no_shows"`
	Pending           int    `json:"pending"`
}

type HourReport struct {
	Hour              string `json:"hour"`
	Sessions          int    `json:"sessions"`
	Bookings          int    `json:"bookings"`
	Attended          int    `json:"attended"`
	Signed            int    `json:"signed"`
	Cancelled         int    `json:"cancelled"`
	MissingSignatures int    `json:"missing_signatures"`
	NoShows           int    `json:"no_shows"`
	Pending           int    `json:"pending"`
}

type SessionReport struct {
	Source            string `json:"source"`
	Date              string `json:"date"`
	Time              string `json:"time"`
	Course            string `json:"course"`
	Room              string `json:"room"`
	Bookings          int    `json:"bookings"`
	Attended          int    `json:"attended"`
	Signed            int    `json:"signed"`
	Cancelled         int    `json:"cancelled"`
	MissingSignatures int    `json:"missing_signatures"`
	NoShows           int    `json:"no_shows"`
	Pending           int    `json:"pending"`
}

type MemberReport struct {
	Source           string `json:"source"`
	MemberID         string `json:"member_id"`
	MemberNumber     string `json:"member_number"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	Bookings         int    `json:"bookings"`
	Attended         int    `json:"attended"`
	Signed           int    `json:"signed"`
	Cancelled        int    `json:"cancelled"`
	MissingSignature int    `json:"missing_signatures"`
	NoShows          int    `json:"no_shows"`
	Pending          int    `json:"pending"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create private database file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close database file: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(absolute))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db, path: absolute}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.hardenFileModes(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error {
	closeErr := store.db.Close()
	modeErr := store.hardenFileModes()
	if closeErr != nil {
		return closeErr
	}
	return modeErr
}

// sqliteFileURI builds a SQLite-compatible file URI from an absolute OS path.
// Windows paths must become file:///C:/...; url.URL{Scheme:"file", Path: `C:\...`}
// incorrectly yields file://C:%5C... and SQLite rejects that as invalid URI authority.
func sqliteFileURI(absolutePath string) string {
	p := strings.ReplaceAll(absolutePath, `\`, `/`)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func sqliteDSN(absolutePath string) string {
	return sqliteFileURI(absolutePath) +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)"
}

func (store *Store) Status(ctx context.Context) (DatabaseStatus, error) {
	var status DatabaseStatus
	status.Path = store.path
	if err := store.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&status.SchemaVersion); err != nil {
		return DatabaseStatus{}, fmt.Errorf("read database schema version: %w", err)
	}
	if err := store.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&status.Integrity); err != nil {
		return DatabaseStatus{}, fmt.Errorf("check database integrity: %w", err)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM members WHERE source='mysign'),
			(SELECT COUNT(*) FROM course_sessions WHERE source='mysign'),
			(SELECT COUNT(*) FROM attendance WHERE source='mysign'),
			(SELECT COUNT(*) FROM admin_capabilities),
			(SELECT COUNT(*) FROM admin_records),
			COALESCE((
				SELECT completed_at FROM sync_runs
				WHERE source='mysign_get_list_data' AND status='completed'
				ORDER BY id DESC LIMIT 1
			), ''),
			COALESCE((
				SELECT completed_at FROM sync_runs
				WHERE source='admin_page' AND status='completed'
				ORDER BY id DESC LIMIT 1
			), '')`,
	).Scan(
		&status.MySignMembers,
		&status.MySignSessions,
		&status.MySignAttendance,
		&status.AdminCapabilities,
		&status.AdminStructuredRows,
		&status.LastMySignSync,
		&status.LastAdminDiscovery,
	); err != nil {
		return DatabaseStatus{}, fmt.Errorf("query database status: %w", err)
	}
	return status, nil
}

func (store *Store) hardenFileModes() error {
	for _, path := range []string{store.path, store.path + "-wal", store.path + "-shm"} {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("protect database file %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func (store *Store) migrate(ctx context.Context) error {
	var current int
	if err := store.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported %d", current, schemaVersion)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS members (
			source TEXT NOT NULL DEFAULT 'mysign',
			myyolo_id TEXT PRIMARY KEY,
			member_number TEXT,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS course_sessions (
			source TEXT NOT NULL DEFAULT 'mysign',
			myyolo_id TEXT PRIMARY KEY,
			date_iso TEXT,
			date_formatted TEXT,
			description TEXT NOT NULL DEFAULT '',
			room_name TEXT NOT NULL DEFAULT '',
			time_formatted TEXT NOT NULL DEFAULT '',
			ends_at_utc TEXT,
			participant_count INTEGER NOT NULL DEFAULT 0,
			current_week INTEGER NOT NULL DEFAULT 0,
			signature_available_iso TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prescriptions (
			source TEXT NOT NULL DEFAULT 'mysign',
			myyolo_id TEXT PRIMARY KEY,
			end_text TEXT,
			treatment_count INTEGER NOT NULL DEFAULT 0,
			weekly_treatments INTEGER NOT NULL DEFAULT 0,
			visits INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS attendance (
			source TEXT NOT NULL DEFAULT 'mysign',
			myyolo_id TEXT PRIMARY KEY,
			member_id TEXT NOT NULL REFERENCES members(myyolo_id),
			course_session_id TEXT NOT NULL REFERENCES course_sessions(myyolo_id),
			prescription_id TEXT REFERENCES prescriptions(myyolo_id),
			attended INTEGER NOT NULL DEFAULT 0,
			signed INTEGER NOT NULL DEFAULT 0,
			signed_manually INTEGER NOT NULL DEFAULT 0,
			cancelled INTEGER NOT NULL DEFAULT 0,
			manual_member_time INTEGER NOT NULL DEFAULT 0,
			training_previous_day INTEGER NOT NULL DEFAULT 0,
			current_week INTEGER NOT NULL DEFAULT 0,
			date_value TEXT,
			date_formatted TEXT,
			time_formatted TEXT,
			course TEXT,
			remaining_units INTEGER NOT NULL DEFAULT 0,
			entry_type TEXT,
			prescription_type TEXT,
			manual_prescription_type TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS attendance_member_idx ON attendance(member_id)`,
		`CREATE INDEX IF NOT EXISTS attendance_session_idx ON attendance(course_session_id)`,
		`CREATE INDEX IF NOT EXISTS attendance_date_idx ON attendance(date_value)`,
		`CREATE TABLE IF NOT EXISTS sync_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			payload_sha256 TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			member_count INTEGER NOT NULL DEFAULT 0,
			session_count INTEGER NOT NULL DEFAULT 0,
			attendance_count INTEGER NOT NULL DEFAULT 0,
			prescription_count INTEGER NOT NULL DEFAULT 0,
			record_count INTEGER NOT NULL DEFAULT 0,
			error_message TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS admin_capabilities (
			route TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			table_count INTEGER NOT NULL DEFAULT 0,
			record_count INTEGER NOT NULL DEFAULT 0,
			headings_json TEXT NOT NULL DEFAULT '[]',
			table_headers_json TEXT NOT NULL DEFAULT '[]',
			table_rows_json TEXT NOT NULL DEFAULT '[]',
			links_json TEXT NOT NULL DEFAULT '[]',
			forms_json TEXT NOT NULL DEFAULT '[]',
			schema_fingerprint TEXT NOT NULL,
			first_observed_at TEXT NOT NULL,
			last_observed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_records (
			route TEXT NOT NULL,
			table_index INTEGER NOT NULL,
			record_hash TEXT NOT NULL,
			values_json TEXT NOT NULL,
			first_observed_at TEXT NOT NULL,
			last_observed_at TEXT NOT NULL,
			PRIMARY KEY(route, table_index, record_hash)
		)`,
		`CREATE INDEX IF NOT EXISTS admin_records_route_idx
		 ON admin_records(route, table_index)`,
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	for _, column := range []struct {
		table      string
		name       string
		definition string
	}{
		{"members", "source", `TEXT NOT NULL DEFAULT 'mysign'`},
		{"course_sessions", "source", `TEXT NOT NULL DEFAULT 'mysign'`},
		{"prescriptions", "source", `TEXT NOT NULL DEFAULT 'mysign'`},
		{"attendance", "source", `TEXT NOT NULL DEFAULT 'mysign'`},
		{"sync_runs", "record_count", `INTEGER NOT NULL DEFAULT 0`},
	} {
		if err := ensureColumn(
			ctx,
			tx,
			column.table,
			column.name,
			column.definition,
		); err != nil {
			return err
		}
	}
	hasSessionEnd, err := columnExists(ctx, tx, "course_sessions", "ends_at_utc")
	if err != nil {
		return err
	}
	if !hasSessionEnd {
		if _, err := tx.ExecContext(
			ctx,
			`ALTER TABLE course_sessions ADD COLUMN ends_at_utc TEXT`,
		); err != nil {
			return fmt.Errorf("add course session end timestamp: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS members_member_number_unique`); err != nil {
		return fmt.Errorf("drop legacy member-number index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS members_source_number_unique
		ON members(source, member_number)
		WHERE member_number IS NOT NULL AND member_number <> ''`); err != nil {
		return fmt.Errorf("create source-scoped member-number index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	if err := store.backfillSessionEnds(ctx); err != nil {
		return fmt.Errorf("backfill course session end timestamps: %w", err)
	}
	return nil
}

func (store *Store) ImportMySign(
	ctx context.Context,
	snapshot mysign.Snapshot,
	payloadSHA256 string,
) (result ImportResult, returnErr error) {
	if err := snapshot.Validate(); err != nil {
		return ImportResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result = ImportResult{
		Members:       len(snapshot.Members),
		Sessions:      len(snapshot.CourseSessions),
		Attendance:    len(snapshot.Attendance),
		Prescriptions: len(snapshot.Prescriptions),
	}
	run, err := store.db.ExecContext(ctx, `
		INSERT INTO sync_runs(source, payload_sha256, status, started_at)
		VALUES('mysign_get_list_data', ?, 'running', ?)`,
		payloadSHA256, now,
	)
	if err != nil {
		return ImportResult{}, fmt.Errorf("start sync run: %w", err)
	}
	result.RunID, err = run.LastInsertId()
	if err != nil {
		return ImportResult{}, fmt.Errorf("read sync run id: %w", err)
	}
	runID := result.RunID

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		importErr := fmt.Errorf("begin import: %w", err)
		store.markSyncFailed(ctx, runID, importErr)
		return ImportResult{}, importErr
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = tx.Rollback()
		if returnErr != nil {
			store.markSyncFailed(ctx, runID, returnErr)
		}
	}()

	// Only a fresh database and validated wire counts prove one observation.
	var priorRows int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM course_sessions) + (SELECT COUNT(*) FROM attendance) + (SELECT COUNT(*) FROM sync_runs WHERE id <> ?)`, runID).Scan(&priorRows); err != nil {
		return ImportResult{}, err
	}
	validObservation := priorRows == 0 && snapshot.RequiredCollectionsValidated()
	for _, session := range snapshot.CourseSessions {
		validObservation = validObservation && session.ParticipantCountValidated()
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS reha_import_observation (singleton INTEGER PRIMARY KEY CHECK(singleton=1), observed_at TEXT NOT NULL, validation TEXT NOT NULL)`); err != nil {
		return ImportResult{}, err
	}
	validation := "invalid"
	if validObservation {
		validation = "required_nonnegative_integer_and_collections_v1"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reha_import_observation VALUES(1, ?, ?) ON CONFLICT(singleton) DO UPDATE SET observed_at=excluded.observed_at, validation=excluded.validation`, now, validation); err != nil {
		return ImportResult{}, err
	}

	// Clear incoming member numbers before assigning the new snapshot values.
	// This makes number swaps deterministic while the unique index remains
	// active for conflicts with members outside a partial snapshot.
	for _, member := range snapshot.Members {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE members SET member_number=NULL WHERE myyolo_id=?`,
			string(member.ID),
		); err != nil {
			return ImportResult{}, fmt.Errorf("prepare member number %s: %w", member.ID, err)
		}
	}

	for _, member := range snapshot.Members {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO members(source, myyolo_id, member_number, first_name, last_name, updated_at)
			VALUES('mysign', ?, NULLIF(?, ''), ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
				source='mysign',
				member_number=excluded.member_number,
				first_name=excluded.first_name,
				last_name=excluded.last_name,
				updated_at=excluded.updated_at`,
			string(member.ID), member.MemberNumber, member.FirstName, member.LastName, now,
		); err != nil {
			return ImportResult{}, fmt.Errorf("upsert member %s: %w", member.ID, err)
		}
	}

	for _, prescription := range snapshot.Prescriptions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO prescriptions(
				source, myyolo_id, end_text, treatment_count, weekly_treatments, visits, updated_at
			) VALUES('mysign', ?, ?, ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
				source='mysign',
				end_text=excluded.end_text,
				treatment_count=excluded.treatment_count,
				weekly_treatments=excluded.weekly_treatments,
				visits=excluded.visits,
				updated_at=excluded.updated_at`,
			string(prescription.ID),
			prescription.EndText,
			prescription.TreatmentCount,
			prescription.WeeklyTreatments,
			prescription.Visits,
			now,
		); err != nil {
			return ImportResult{}, fmt.Errorf("upsert prescription %s: %w", prescription.ID, err)
		}
	}

	for _, session := range snapshot.CourseSessions {
		endsAtUTC := sessionEndUTC(session)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO course_sessions(
				source, myyolo_id, date_iso, date_formatted, description, room_name,
				time_formatted, ends_at_utc, participant_count, current_week,
				signature_available_iso, updated_at
			) VALUES('mysign', ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
				source='mysign',
				date_iso=excluded.date_iso,
				date_formatted=excluded.date_formatted,
				description=excluded.description,
				room_name=excluded.room_name,
				time_formatted=excluded.time_formatted,
				ends_at_utc=excluded.ends_at_utc,
				participant_count=excluded.participant_count,
				current_week=excluded.current_week,
				signature_available_iso=excluded.signature_available_iso,
				updated_at=excluded.updated_at`,
			string(session.ID),
			session.DateISO,
			session.DateFormatted,
			session.Description,
			session.RoomName,
			session.TimeFormatted,
			endsAtUTC,
			session.ParticipantCount,
			boolInt(session.CurrentWeek),
			session.SignatureAvailableISO,
			now,
		); err != nil {
			return ImportResult{}, fmt.Errorf("upsert course session %s: %w", session.ID, err)
		}
	}

	for _, attendance := range snapshot.Attendance {
		var prescriptionID any
		if attendance.PrescriptionID != "" {
			prescriptionID = string(attendance.PrescriptionID)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO attendance(
				source, myyolo_id, member_id, course_session_id, prescription_id,
				attended, signed, signed_manually, cancelled, manual_member_time,
				training_previous_day, current_week, date_value, date_formatted,
				time_formatted, course, remaining_units, entry_type,
				prescription_type, manual_prescription_type, updated_at
			) VALUES('mysign', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
				source='mysign',
				member_id=excluded.member_id,
				course_session_id=excluded.course_session_id,
				prescription_id=excluded.prescription_id,
				attended=excluded.attended,
				signed=excluded.signed,
				signed_manually=excluded.signed_manually,
				cancelled=excluded.cancelled,
				manual_member_time=excluded.manual_member_time,
				training_previous_day=excluded.training_previous_day,
				current_week=excluded.current_week,
				date_value=excluded.date_value,
				date_formatted=excluded.date_formatted,
				time_formatted=excluded.time_formatted,
				course=excluded.course,
				remaining_units=excluded.remaining_units,
				entry_type=excluded.entry_type,
				prescription_type=excluded.prescription_type,
				manual_prescription_type=excluded.manual_prescription_type,
				updated_at=excluded.updated_at`,
			string(attendance.ID),
			string(attendance.MemberID),
			string(attendance.CourseSessionID),
			prescriptionID,
			boolInt(attendance.Attended),
			boolInt(attendance.Signed),
			boolInt(attendance.SignedManually),
			boolInt(attendance.Cancelled),
			boolInt(attendance.ManualMemberTime),
			boolInt(attendance.TrainingPreviousDay),
			boolInt(attendance.CurrentWeek),
			attendance.Date,
			attendance.DateFormatted,
			attendance.TimeFormatted,
			attendance.Course,
			attendance.RemainingUnits,
			string(attendance.Type),
			string(attendance.PrescriptionType),
			string(attendance.ManualPrescriptionType),
			now,
		); err != nil {
			return ImportResult{}, fmt.Errorf("upsert attendance %s: %w", attendance.ID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_runs SET
			status='completed',
			completed_at=?,
			member_count=?,
			session_count=?,
			attendance_count=?,
			prescription_count=?
		WHERE id=?`,
		now,
		result.Members,
		result.Sessions,
		result.Attendance,
		result.Prescriptions,
		result.RunID,
	); err != nil {
		return ImportResult{}, fmt.Errorf("complete sync run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit import: %w", err)
	}
	committed = true
	if err := store.hardenFileModes(); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func (store *Store) ImportAdminPage(
	ctx context.Context,
	page admin.Page,
) (AdminImportResult, error) {
	return store.ImportAdminPageAt(ctx, page, time.Now())
}

func (store *Store) ImportAdminPageAt(
	ctx context.Context,
	page admin.Page,
	observedAt time.Time,
) (result AdminImportResult, returnErr error) {
	if !validAdminRoute(page.Metadata.Route) {
		return AdminImportResult{}, fmt.Errorf(
			"admin page route must be an absolute path or a known capability key",
		)
	}
	if len(page.Metadata.Fingerprint) != 64 {
		return AdminImportResult{}, fmt.Errorf("admin page schema fingerprint is invalid")
	}
	if page.Metadata.LoginForm {
		return AdminImportResult{}, fmt.Errorf("refusing to import an admin login page")
	}
	now := observedAt.UTC().Format(time.RFC3339Nano)
	result = AdminImportResult{
		Route:       page.Metadata.Route,
		Tables:      len(page.Tables),
		Fingerprint: page.Metadata.Fingerprint,
	}
	for _, table := range page.Tables {
		result.Records += len(table.Rows)
	}

	run, err := store.db.ExecContext(ctx, `
		INSERT INTO sync_runs(
			source, payload_sha256, status, started_at, record_count
		) VALUES('admin_page', ?, 'running', ?, ?)`,
		page.Metadata.Fingerprint,
		now,
		result.Records,
	)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("start admin sync run: %w", err)
	}
	result.RunID, err = run.LastInsertId()
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("read admin sync run id: %w", err)
	}
	runID := result.RunID

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		importErr := fmt.Errorf("begin admin import: %w", err)
		store.markSyncFailed(ctx, runID, importErr)
		return AdminImportResult{}, importErr
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = tx.Rollback()
		if returnErr != nil {
			store.markSyncFailed(ctx, runID, returnErr)
		}
	}()

	headingsJSON, err := json.Marshal(page.Metadata.Headings)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("encode admin headings: %w", err)
	}
	tableHeadersJSON, err := json.Marshal(page.Metadata.TableHeaders)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("encode admin table headers: %w", err)
	}
	tableRowsJSON, err := json.Marshal(page.Metadata.TableRows)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("encode admin table counts: %w", err)
	}
	linksJSON, err := json.Marshal(page.Metadata.Links)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("encode admin links: %w", err)
	}
	formsJSON, err := json.Marshal(page.Metadata.Forms)
	if err != nil {
		return AdminImportResult{}, fmt.Errorf("encode admin forms: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_capabilities(
			route, title, table_count, record_count, headings_json,
			table_headers_json, table_rows_json, links_json, forms_json,
			schema_fingerprint, first_observed_at, last_observed_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(route) DO UPDATE SET
			title=excluded.title,
			table_count=excluded.table_count,
			record_count=excluded.record_count,
			headings_json=excluded.headings_json,
			table_headers_json=excluded.table_headers_json,
			table_rows_json=excluded.table_rows_json,
			links_json=excluded.links_json,
			forms_json=excluded.forms_json,
			schema_fingerprint=excluded.schema_fingerprint,
			last_observed_at=excluded.last_observed_at`,
		page.Metadata.Route,
		page.Metadata.Title,
		len(page.Tables),
		result.Records,
		string(headingsJSON),
		string(tableHeadersJSON),
		string(tableRowsJSON),
		string(linksJSON),
		string(formsJSON),
		page.Metadata.Fingerprint,
		now,
		now,
	); err != nil {
		return AdminImportResult{}, fmt.Errorf("upsert admin capability: %w", err)
	}

	for tableIndex, table := range page.Tables {
		for _, row := range table.Rows {
			valuesJSON, err := json.Marshal(adminRowValues(table.Headers, row))
			if err != nil {
				return AdminImportResult{}, fmt.Errorf("encode admin table row: %w", err)
			}
			hashInput := fmt.Sprintf(
				"%s\x00%d\x00%s",
				page.Metadata.Route,
				tableIndex,
				valuesJSON,
			)
			hash := sha256.Sum256([]byte(hashInput))
			recordHash := hex.EncodeToString(hash[:])
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO admin_records(
					route, table_index, record_hash, values_json,
					first_observed_at, last_observed_at
				) VALUES(?, ?, ?, ?, ?, ?)
				ON CONFLICT(route, table_index, record_hash) DO UPDATE SET
					last_observed_at=excluded.last_observed_at`,
				page.Metadata.Route,
				tableIndex,
				recordHash,
				string(valuesJSON),
				now,
				now,
			); err != nil {
				return AdminImportResult{}, fmt.Errorf("upsert admin table row: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_runs SET status='completed', completed_at=?, record_count=?
		WHERE id=?`,
		now,
		result.Records,
		result.RunID,
	); err != nil {
		return AdminImportResult{}, fmt.Errorf("complete admin sync run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AdminImportResult{}, fmt.Errorf("commit admin import: %w", err)
	}
	committed = true
	if err := store.hardenFileModes(); err != nil {
		return AdminImportResult{}, err
	}
	return result, nil
}

func validAdminRoute(route string) bool {
	if strings.HasPrefix(route, "/") {
		return true
	}
	_, ok := readcatalog.SensitivityForRouteKey(route)
	return ok
}

func adminRowValues(headers []string, row []string) map[string]string {
	values := make(map[string]string, len(row))
	used := make(map[string]int, len(row))
	for index, value := range row {
		key := fmt.Sprintf("column_%d", index+1)
		if index < len(headers) && headers[index] != "" {
			key = headers[index]
		}
		used[key]++
		if used[key] > 1 {
			key = fmt.Sprintf("%s_%d", key, used[key])
		}
		values[key] = value
	}
	return values
}

func (store *Store) markSyncFailed(ctx context.Context, runID int64, importErr error) {
	message := importErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, _ = store.db.ExecContext(
		context.WithoutCancel(ctx),
		`UPDATE sync_runs
		 SET status='failed', completed_at=?, error_message=?
		 WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		message,
		runID,
	)
}

type queryExecer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func columnExists(
	ctx context.Context,
	queryer queryExecer,
	table string,
	column string,
) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			index      int
			name       string
			columnType string
			notNull    int
			defaultSQL sql.NullString
			primaryKey int
		)
		if err := rows.Scan(
			&index,
			&name,
			&columnType,
			&notNull,
			&defaultSQL,
			&primaryKey,
		); err != nil {
			return false, fmt.Errorf("scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate %s columns: %w", table, err)
	}
	return false, nil
}

func ensureColumn(
	ctx context.Context,
	tx *sql.Tx,
	table string,
	column string,
	definition string,
) error {
	exists, err := columnExists(ctx, tx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := tx.ExecContext(
		ctx,
		`ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition,
	); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func (store *Store) backfillSessionEnds(ctx context.Context) error {
	rows, err := store.db.QueryContext(ctx, `
		SELECT myyolo_id, COALESCE(date_iso, ''), COALESCE(time_formatted, '')
		FROM course_sessions
		WHERE ends_at_utc IS NULL OR ends_at_utc = ''`)
	if err != nil {
		return err
	}
	type pendingEnd struct {
		id            string
		dateISO       string
		timeFormatted string
	}
	var pending []pendingEnd
	for rows.Next() {
		var item pendingEnd
		if err := rows.Scan(&item.id, &item.dateISO, &item.timeFormatted); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range pending {
		endsAtUTC := sessionEndUTC(mysign.CourseSession{
			DateISO:       item.dateISO,
			TimeFormatted: item.timeFormatted,
		})
		if endsAtUTC == "" {
			continue
		}
		if _, err := store.db.ExecContext(
			ctx,
			`UPDATE course_sessions SET ends_at_utc=? WHERE myyolo_id=?`,
			endsAtUTC,
			item.id,
		); err != nil {
			return err
		}
	}
	return nil
}

func sessionEndUTC(session mysign.CourseSession) string {
	start, err := time.Parse(time.RFC3339, session.DateISO)
	if err != nil {
		return ""
	}
	match := sessionTimeRangePattern.FindStringSubmatch(session.TimeFormatted)
	if len(match) != 5 {
		return ""
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return ""
	}
	start = start.In(location)
	endClock, err := time.Parse("15:04", match[3]+":"+match[4])
	if err != nil {
		return ""
	}
	startClock, err := time.Parse("15:04", match[1]+":"+match[2])
	if err != nil {
		return ""
	}
	day := time.Date(start.Year(), start.Month(), start.Day(), 12, 0, 0, 0, location)
	if endClock.Before(startClock) {
		day = day.AddDate(0, 0, 1)
	}
	end := time.Date(
		day.Year(),
		day.Month(),
		day.Day(),
		endClock.Hour(),
		endClock.Minute(),
		0,
		0,
		location,
	)
	if end.Hour() != endClock.Hour() || end.Minute() != endClock.Minute() {
		return ""
	}
	for _, delta := range []time.Duration{-time.Hour, time.Hour} {
		other := end.Add(delta).In(location)
		if other.Year() == end.Year() && other.YearDay() == end.YearDay() && other.Hour() == end.Hour() && other.Minute() == end.Minute() {
			return ""
		}
	}
	return end.UTC().Format(time.RFC3339)
}

func (store *Store) Summary(ctx context.Context) (Summary, error) {
	return store.SummaryAt(ctx, time.Now())
}

func (store *Store) SummaryAt(ctx context.Context, asOf time.Time) (Summary, error) {
	var summary Summary
	row := store.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM members),
			(SELECT COUNT(*) FROM course_sessions),
			(SELECT COUNT(*) FROM attendance),
			(SELECT COUNT(*) FROM attendance WHERE attended = 1),
			(SELECT COUNT(*) FROM attendance WHERE signed = 1),
			(SELECT COUNT(*) FROM attendance WHERE cancelled = 1),
			(SELECT COUNT(*) FROM attendance
			 WHERE attended = 1 AND signed = 0 AND cancelled = 0),
			(SELECT COUNT(*)
			 FROM attendance a
			 JOIN course_sessions s
			   ON s.source = a.source AND s.myyolo_id = a.course_session_id
			 WHERE a.attended = 0 AND a.cancelled = 0
			   AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			   AND s.ends_at_utc < ?),
			(SELECT COUNT(*)
			 FROM attendance a
			 JOIN course_sessions s
			   ON s.source = a.source AND s.myyolo_id = a.course_session_id
			 WHERE a.attended = 0 AND a.cancelled = 0
			   AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			        OR s.ends_at_utc >= ?)),
			(SELECT COUNT(DISTINCT member_id) FROM attendance),
			COALESCE((
				SELECT completed_at FROM sync_runs
				WHERE status = 'completed' AND source = 'mysign_get_list_data'
				ORDER BY id DESC LIMIT 1
			), '')`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err := row.Scan(
		&summary.Members,
		&summary.CourseSessions,
		&summary.AttendanceRows,
		&summary.Attended,
		&summary.Signed,
		&summary.Cancelled,
		&summary.MissingSignatures,
		&summary.NoShows,
		&summary.Pending,
		&summary.DistinctParticipants,
		&summary.LastCompletedSync,
	); err != nil {
		return Summary{}, fmt.Errorf("query summary: %w", err)
	}
	return summary, nil
}

func (store *Store) Courses(ctx context.Context) ([]CourseReport, error) {
	return store.CoursesAt(ctx, time.Now())
}

func (store *Store) CoursesAt(ctx context.Context, asOf time.Time) ([]CourseReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(a.course, ''), NULLIF(s.description, ''), 'Unbekannt') AS course,
			COUNT(DISTINCT a.course_session_id),
			COUNT(*),
			SUM(a.attended),
			SUM(a.signed),
			SUM(a.cancelled),
			SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			              AND s.ends_at_utc < ?
			         THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			                   OR s.ends_at_utc >= ?)
			         THEN 1 ELSE 0 END)
		FROM attendance a
		JOIN course_sessions s
		  ON s.source = a.source AND s.myyolo_id = a.course_session_id
		GROUP BY course
		ORDER BY course`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err != nil {
		return nil, fmt.Errorf("query course report: %w", err)
	}
	defer rows.Close()
	var report []CourseReport
	for rows.Next() {
		var item CourseReport
		if err := rows.Scan(
			&item.Course,
			&item.Sessions,
			&item.Bookings,
			&item.Attended,
			&item.Signed,
			&item.Cancelled,
			&item.MissingSignatures,
			&item.NoShows,
			&item.Pending,
		); err != nil {
			return nil, fmt.Errorf("scan course report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Days(ctx context.Context) ([]DayReport, error) {
	return store.DaysAt(ctx, time.Now())
}

func (store *Store) DaysAt(ctx context.Context, asOf time.Time) ([]DayReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(a.date_value, ''), substr(s.date_iso, 1, 10), s.date_formatted) AS day,
			COUNT(DISTINCT a.course_session_id),
			COUNT(*),
			SUM(a.attended),
			SUM(a.signed),
			SUM(a.cancelled),
			SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			              AND s.ends_at_utc < ?
			         THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			                   OR s.ends_at_utc >= ?)
			         THEN 1 ELSE 0 END)
		FROM attendance a
		JOIN course_sessions s
		  ON s.source = a.source AND s.myyolo_id = a.course_session_id
		GROUP BY day
		ORDER BY day`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err != nil {
		return nil, fmt.Errorf("query daily report: %w", err)
	}
	defer rows.Close()
	var report []DayReport
	for rows.Next() {
		var item DayReport
		if err := rows.Scan(
			&item.Date,
			&item.Sessions,
			&item.Bookings,
			&item.Attended,
			&item.Signed,
			&item.Cancelled,
			&item.MissingSignatures,
			&item.NoShows,
			&item.Pending,
		); err != nil {
			return nil, fmt.Errorf("scan daily report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Hours(ctx context.Context) ([]HourReport, error) {
	return store.HoursAt(ctx, time.Now())
}

func (store *Store) HoursAt(ctx context.Context, asOf time.Time) ([]HourReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			CASE
				WHEN a.time_formatted GLOB '[0-2][0-9]:[0-5][0-9]*'
				THEN substr(a.time_formatted, 1, 2) || ':00'
				ELSE 'Unbekannt'
			END AS hour,
			COUNT(DISTINCT a.course_session_id),
			COUNT(*),
			SUM(a.attended),
			SUM(a.signed),
			SUM(a.cancelled),
			SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			              AND s.ends_at_utc < ?
			         THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			              AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			                   OR s.ends_at_utc >= ?)
			         THEN 1 ELSE 0 END)
		FROM attendance a
		JOIN course_sessions s
		  ON s.source = a.source AND s.myyolo_id = a.course_session_id
		GROUP BY hour
		ORDER BY hour`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err != nil {
		return nil, fmt.Errorf("query hourly report: %w", err)
	}
	defer rows.Close()
	var report []HourReport
	for rows.Next() {
		var item HourReport
		if err := rows.Scan(
			&item.Hour,
			&item.Sessions,
			&item.Bookings,
			&item.Attended,
			&item.Signed,
			&item.Cancelled,
			&item.MissingSignatures,
			&item.NoShows,
			&item.Pending,
		); err != nil {
			return nil, fmt.Errorf("scan hourly report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Sessions(ctx context.Context) ([]SessionReport, error) {
	return store.SessionsAt(ctx, time.Now())
}

func (store *Store) SessionsAt(ctx context.Context, asOf time.Time) ([]SessionReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			s.source,
			COALESCE(NULLIF(s.date_iso, ''), s.date_formatted),
			s.time_formatted,
			s.description,
			s.room_name,
			COUNT(a.myyolo_id),
			COALESCE(SUM(a.attended), 0),
			COALESCE(SUM(a.signed), 0),
			COALESCE(SUM(a.cancelled), 0),
			COALESCE(SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			                       AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			                       AND s.ends_at_utc < ?
			                  THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			                       AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			                            OR s.ends_at_utc >= ?)
			                  THEN 1 ELSE 0 END), 0)
		FROM course_sessions s
		LEFT JOIN attendance a
		  ON a.source = s.source AND a.course_session_id = s.myyolo_id
		GROUP BY s.source, s.myyolo_id
		ORDER BY s.date_iso, s.time_formatted, s.description`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err != nil {
		return nil, fmt.Errorf("query session report: %w", err)
	}
	defer rows.Close()
	var report []SessionReport
	for rows.Next() {
		var item SessionReport
		if err := rows.Scan(
			&item.Source,
			&item.Date,
			&item.Time,
			&item.Course,
			&item.Room,
			&item.Bookings,
			&item.Attended,
			&item.Signed,
			&item.Cancelled,
			&item.MissingSignatures,
			&item.NoShows,
			&item.Pending,
		); err != nil {
			return nil, fmt.Errorf("scan session report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Members(ctx context.Context) ([]MemberReport, error) {
	return store.MembersAt(ctx, time.Now())
}

func (store *Store) MembersAt(ctx context.Context, asOf time.Time) ([]MemberReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			m.source,
			m.myyolo_id,
			COALESCE(m.member_number, ''),
			m.first_name,
			m.last_name,
			COUNT(a.myyolo_id),
			COALESCE(SUM(a.attended), 0),
			COALESCE(SUM(a.signed), 0),
			COALESCE(SUM(a.cancelled), 0),
			COALESCE(SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			                       AND s.ends_at_utc IS NOT NULL AND s.ends_at_utc <> ''
			                       AND s.ends_at_utc < ?
			                  THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0
			                       AND (s.ends_at_utc IS NULL OR s.ends_at_utc = ''
			                            OR s.ends_at_utc >= ?)
			                  THEN 1 ELSE 0 END), 0)
		FROM members m
		LEFT JOIN attendance a
		  ON a.source = m.source AND a.member_id = m.myyolo_id
		LEFT JOIN course_sessions s
		  ON s.source = a.source AND s.myyolo_id = a.course_session_id
		GROUP BY m.source, m.myyolo_id
		ORDER BY m.last_name, m.first_name, m.source, m.myyolo_id`,
		reportAsOf(asOf),
		reportAsOf(asOf),
	)
	if err != nil {
		return nil, fmt.Errorf("query member report: %w", err)
	}
	defer rows.Close()
	var report []MemberReport
	for rows.Next() {
		var item MemberReport
		if err := rows.Scan(
			&item.Source,
			&item.MemberID,
			&item.MemberNumber,
			&item.FirstName,
			&item.LastName,
			&item.Bookings,
			&item.Attended,
			&item.Signed,
			&item.Cancelled,
			&item.MissingSignature,
			&item.NoShows,
			&item.Pending,
		); err != nil {
			return nil, fmt.Errorf("scan member report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) AdminCapabilities(
	ctx context.Context,
) ([]AdminCapabilityReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			route,
			title,
			table_count,
			record_count,
			schema_fingerprint,
			first_observed_at,
			last_observed_at
		FROM admin_capabilities
		ORDER BY route`)
	if err != nil {
		return nil, fmt.Errorf("query admin capabilities: %w", err)
	}
	defer rows.Close()
	var report []AdminCapabilityReport
	for rows.Next() {
		var item AdminCapabilityReport
		if err := rows.Scan(
			&item.Route,
			&item.Title,
			&item.TableCount,
			&item.RecordCount,
			&item.SchemaFingerprint,
			&item.FirstObservedAt,
			&item.LastObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admin capability: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) AdminRecords(
	ctx context.Context,
	route string,
) ([]AdminRecordReport, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if route == "" {
		rows, err = store.db.QueryContext(ctx, `
			SELECT
				route, table_index, record_hash, values_json,
				first_observed_at, last_observed_at
			FROM admin_records
			ORDER BY route, table_index, record_hash`)
	} else {
		rows, err = store.db.QueryContext(ctx, `
			SELECT
				route, table_index, record_hash, values_json,
				first_observed_at, last_observed_at
			FROM admin_records
			WHERE route=?
			ORDER BY table_index, record_hash`,
			route,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query admin records: %w", err)
	}
	defer rows.Close()
	var report []AdminRecordReport
	for rows.Next() {
		var item AdminRecordReport
		if err := rows.Scan(
			&item.Route,
			&item.TableIndex,
			&item.RecordHash,
			&item.ValuesJSON,
			&item.FirstObservedAt,
			&item.LastObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admin record: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) CurrentAdminRecords(
	ctx context.Context,
	route string,
) ([]AdminRecordReport, error) {
	if !validAdminRoute(route) {
		return nil, fmt.Errorf("unknown admin route %q", route)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			records.route,
			records.table_index,
			records.record_hash,
			records.values_json,
			records.first_observed_at,
			records.last_observed_at
		FROM admin_records AS records
		INNER JOIN admin_capabilities AS capabilities
			ON capabilities.route = records.route
			AND capabilities.last_observed_at = records.last_observed_at
		WHERE records.route = ?
		ORDER BY records.table_index, records.record_hash`,
		route,
	)
	if err != nil {
		return nil, fmt.Errorf("query current admin records: %w", err)
	}
	defer rows.Close()
	var report []AdminRecordReport
	for rows.Next() {
		var item AdminRecordReport
		if err := rows.Scan(
			&item.Route,
			&item.TableIndex,
			&item.RecordHash,
			&item.ValuesJSON,
			&item.FirstObservedAt,
			&item.LastObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan current admin record: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) AdminRehaHours(
	ctx context.Context,
) ([]AdminRehaHourReport, error) {
	const route = "/Anwesenheit/Reha_Anwesend_Datum_Liste.asp"
	tableIndex, err := store.adminTableIndexForHeaders(
		ctx,
		route,
		"Nr.:",
		"Uhrzeit",
	)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			json_extract(r.values_json, '$.Uhrzeit') AS time_window,
			COUNT(*)
		FROM admin_records r
		JOIN admin_capabilities c
		  ON c.route = r.route AND c.last_observed_at = r.last_observed_at
		WHERE r.route = ? AND r.table_index = ?
		  AND COALESCE(json_extract(r.values_json, '$."Nr.:"'), '') <> ''
		  AND COALESCE(json_extract(r.values_json, '$.Uhrzeit'), '') <> ''
		GROUP BY time_window
		ORDER BY time_window`,
		route,
		tableIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("query Reha attendance hours: %w", err)
	}
	defer rows.Close()
	var report []AdminRehaHourReport
	for rows.Next() {
		var item AdminRehaHourReport
		if err := rows.Scan(&item.TimeWindow, &item.Attendees); err != nil {
			return nil, fmt.Errorf("scan Reha attendance hour: %w", err)
		}
		item.DurationMinutes = adminTimeWindowDuration(item.TimeWindow)
		item.AttendeeMinutes = item.Attendees * item.DurationMinutes
		report = append(report, item)
	}
	return report, rows.Err()
}

func adminTimeWindowDuration(value string) int {
	match := adminTimeWindowPattern.FindStringSubmatch(value)
	if len(match) != 5 {
		return 0
	}
	start, err := time.Parse("15:04", match[1]+":"+match[2])
	if err != nil {
		return 0
	}
	end, err := time.Parse("15:04", match[3]+":"+match[4])
	if err != nil {
		return 0
	}
	if end.Before(start) {
		end = end.Add(24 * time.Hour)
	}
	return int(end.Sub(start) / time.Minute)
}

func (store *Store) AdminCourseMonths(
	ctx context.Context,
) ([]AdminCourseMonthReport, error) {
	const route = "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp"
	tableIndex, err := store.adminTableIndexForHeaders(
		ctx,
		route,
		"Name",
		"Angebotsnummer",
	)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			values_by_month.key,
			SUM(CASE
				WHEN values_by_month.value GLOB '[0-9]*'
				THEN CAST(values_by_month.value AS INTEGER)
				ELSE 0
			END),
			SUM(CASE
				WHEN COALESCE(json_extract(r.values_json, '$.Name'), '') <> ''
				 AND values_by_month.value GLOB '[0-9]*'
				THEN 1 ELSE 0
			END),
			MAX(r.last_observed_at)
		FROM admin_records r
		JOIN admin_capabilities c
		  ON c.route = r.route AND c.last_observed_at = r.last_observed_at
		JOIN json_each(r.values_json) AS values_by_month
		WHERE r.route = ? AND r.table_index = ?
		  AND values_by_month.key GLOB '[0-9]*/[0-9][0-9][0-9][0-9]'
		GROUP BY values_by_month.key
		ORDER BY
			CAST(substr(values_by_month.key, instr(values_by_month.key, '/') + 1) AS INTEGER),
			CAST(substr(values_by_month.key, 1, instr(values_by_month.key, '/') - 1) AS INTEGER)`,
		route,
		tableIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("query monthly course attendance: %w", err)
	}
	defer rows.Close()
	var report []AdminCourseMonthReport
	for rows.Next() {
		var item AdminCourseMonthReport
		if err := rows.Scan(
			&item.Month,
			&item.AttendanceTotal,
			&item.CoursesReported,
			&item.LastObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan monthly course attendance: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) AdminMissingSignatures(
	ctx context.Context,
) (AdminMissingSignatureSummary, error) {
	const route = "/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp"
	tableIndex, err := store.adminTableIndexForHeaders(
		ctx,
		route,
		"Nr.:",
		"Menge",
	)
	if err != nil {
		return AdminMissingSignatureSummary{}, err
	}
	var report AdminMissingSignatureSummary
	if err := store.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE
				WHEN json_extract(r.values_json, '$.Menge') GLOB '[0-9]*'
				THEN CAST(json_extract(r.values_json, '$.Menge') AS INTEGER)
				ELSE 0
			END), 0),
			COALESCE(MAX(r.last_observed_at), '')
		FROM admin_records r
		JOIN admin_capabilities c
		  ON c.route = r.route AND c.last_observed_at = r.last_observed_at
		WHERE r.route = ? AND r.table_index = ?
		  AND COALESCE(json_extract(r.values_json, '$."Nr.:"'), '') <> ''`,
		route,
		tableIndex,
	).Scan(
		&report.MembersWithMissing,
		&report.MissingTotal,
		&report.LastObservedAt,
	); err != nil {
		return AdminMissingSignatureSummary{}, fmt.Errorf("query missing signatures: %w", err)
	}
	return report, nil
}

func (store *Store) AdminRehaAttendanceDetails(
	ctx context.Context,
) ([]AdminRehaAttendanceDetail, error) {
	const route = "/Anwesenheit/Reha_Anwesend_Datum_Liste.asp"
	tableIndex, err := store.adminTableIndexForHeaders(
		ctx,
		route,
		"Nr.:",
		"Name",
		"Vorname",
		"Uhrzeit",
	)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(json_extract(r.values_json, '$."Nr.:"'), ''),
			COALESCE(json_extract(r.values_json, '$.Vorname'), ''),
			COALESCE(json_extract(r.values_json, '$.Name'), ''),
			COALESCE(json_extract(r.values_json, '$.Uhrzeit'), '')
		FROM admin_records r
		JOIN admin_capabilities c
		  ON c.route = r.route AND c.last_observed_at = r.last_observed_at
		WHERE r.route = ? AND r.table_index = ?
		  AND COALESCE(json_extract(r.values_json, '$."Nr.:"'), '') <> ''
		ORDER BY time(rtrim(substr(
			json_extract(r.values_json, '$.Uhrzeit') || ' ',
			1,
			instr(json_extract(r.values_json, '$.Uhrzeit') || ' ', ' ') - 1
		))), json_extract(r.values_json, '$.Name')`,
		route,
		tableIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("query Reha attendance details: %w", err)
	}
	defer rows.Close()
	var report []AdminRehaAttendanceDetail
	for rows.Next() {
		var item AdminRehaAttendanceDetail
		if err := rows.Scan(
			&item.MemberNumber,
			&item.FirstName,
			&item.LastName,
			&item.TimeWindow,
		); err != nil {
			return nil, fmt.Errorf("scan Reha attendance detail: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) AdminMissingSignatureDetails(
	ctx context.Context,
) ([]AdminMissingSignatureDetail, error) {
	const route = "/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp"
	tableIndex, err := store.adminTableIndexForHeaders(
		ctx,
		route,
		"Nr.:",
		"Vorname",
		"Name",
		"Menge",
	)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(json_extract(r.values_json, '$."Nr.:"'), ''),
			COALESCE(json_extract(r.values_json, '$.Vorname'), ''),
			COALESCE(json_extract(r.values_json, '$.Name'), ''),
			CASE
				WHEN json_extract(r.values_json, '$.Menge') GLOB '[0-9]*'
				THEN CAST(json_extract(r.values_json, '$.Menge') AS INTEGER)
				ELSE 0
			END
		FROM admin_records r
		JOIN admin_capabilities c
		  ON c.route = r.route AND c.last_observed_at = r.last_observed_at
		WHERE r.route = ? AND r.table_index = ?
		  AND COALESCE(json_extract(r.values_json, '$."Nr.:"'), '') <> ''
		ORDER BY json_extract(r.values_json, '$.Name'),
		         json_extract(r.values_json, '$.Vorname')`,
		route,
		tableIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("query missing-signature details: %w", err)
	}
	defer rows.Close()
	var report []AdminMissingSignatureDetail
	for rows.Next() {
		var item AdminMissingSignatureDetail
		if err := rows.Scan(
			&item.MemberNumber,
			&item.FirstName,
			&item.LastName,
			&item.Missing,
		); err != nil {
			return nil, fmt.Errorf("scan missing-signature detail: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) adminTableIndexForHeaders(
	ctx context.Context,
	route string,
	required ...string,
) (int, error) {
	var encoded string
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT table_headers_json FROM admin_capabilities WHERE route=?`,
		route,
	).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("admin capability %s has not been collected", route)
		}
		return 0, fmt.Errorf("read admin capability headers: %w", err)
	}
	var tables [][]string
	if err := json.Unmarshal([]byte(encoded), &tables); err != nil {
		return 0, fmt.Errorf("decode admin capability headers: %w", err)
	}
	for index, headers := range tables {
		available := make(map[string]struct{}, len(headers))
		for _, header := range headers {
			available[header] = struct{}{}
		}
		matches := true
		for _, header := range required {
			if _, ok := available[header]; !ok {
				matches = false
				break
			}
		}
		if matches {
			return index, nil
		}
	}
	return 0, fmt.Errorf(
		"admin capability %s lacks required columns %s",
		route,
		strings.Join(required, ", "),
	)
}

func reportAsOf(asOf time.Time) string {
	return asOf.UTC().Format(time.RFC3339)
}

func boolInt(value mysign.Bool) int {
	if bool(value) {
		return 1
	}
	return 0
}
