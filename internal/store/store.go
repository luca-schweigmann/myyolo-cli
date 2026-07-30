package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"

	_ "modernc.org/sqlite"
)

const schemaVersion = 2

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

type Summary struct {
	Members              int    `json:"members"`
	CourseSessions       int    `json:"course_sessions"`
	AttendanceRows       int    `json:"attendance_rows"`
	Attended             int    `json:"attended"`
	Signed               int    `json:"signed"`
	Cancelled            int    `json:"cancelled"`
	MissingSignatures    int    `json:"missing_signatures"`
	NoShows              int    `json:"no_shows"`
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
}

type SessionReport struct {
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
}

type MemberReport struct {
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

	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
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
			myyolo_id TEXT PRIMARY KEY,
			member_number TEXT,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS members_member_number_unique
		 ON members(member_number) WHERE member_number IS NOT NULL AND member_number <> ''`,
		`CREATE TABLE IF NOT EXISTS course_sessions (
			myyolo_id TEXT PRIMARY KEY,
			date_iso TEXT,
			date_formatted TEXT,
			description TEXT NOT NULL DEFAULT '',
			room_name TEXT NOT NULL DEFAULT '',
			time_formatted TEXT NOT NULL DEFAULT '',
			participant_count INTEGER NOT NULL DEFAULT 0,
			current_week INTEGER NOT NULL DEFAULT 0,
			signature_available_iso TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prescriptions (
			myyolo_id TEXT PRIMARY KEY,
			end_text TEXT,
			treatment_count INTEGER NOT NULL DEFAULT 0,
			weekly_treatments INTEGER NOT NULL DEFAULT 0,
			visits INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS attendance (
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
			error_message TEXT
		)`,
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
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (store *Store) ImportMySign(
	ctx context.Context,
	snapshot mysign.Snapshot,
	payloadSHA256 string,
) (result ImportResult, returnErr error) {
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
			INSERT INTO members(myyolo_id, member_number, first_name, last_name, updated_at)
			VALUES(?, NULLIF(?, ''), ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
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
				myyolo_id, end_text, treatment_count, weekly_treatments, visits, updated_at
			) VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
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
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO course_sessions(
				myyolo_id, date_iso, date_formatted, description, room_name,
				time_formatted, participant_count, current_week,
				signature_available_iso, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
				date_iso=excluded.date_iso,
				date_formatted=excluded.date_formatted,
				description=excluded.description,
				room_name=excluded.room_name,
				time_formatted=excluded.time_formatted,
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
				myyolo_id, member_id, course_session_id, prescription_id,
				attended, signed, signed_manually, cancelled, manual_member_time,
				training_previous_day, current_week, date_value, date_formatted,
				time_formatted, course, remaining_units, entry_type,
				prescription_type, manual_prescription_type, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(myyolo_id) DO UPDATE SET
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

func (store *Store) Summary(ctx context.Context) (Summary, error) {
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
			(SELECT COUNT(*) FROM attendance
			 WHERE attended = 0 AND cancelled = 0),
			(SELECT COUNT(DISTINCT member_id) FROM attendance),
			COALESCE((
				SELECT completed_at FROM sync_runs
				WHERE status = 'completed'
				ORDER BY id DESC LIMIT 1
			), '')`)
	if err := row.Scan(
		&summary.Members,
		&summary.CourseSessions,
		&summary.AttendanceRows,
		&summary.Attended,
		&summary.Signed,
		&summary.Cancelled,
		&summary.MissingSignatures,
		&summary.NoShows,
		&summary.DistinctParticipants,
		&summary.LastCompletedSync,
	); err != nil {
		return Summary{}, fmt.Errorf("query summary: %w", err)
	}
	return summary, nil
}

func (store *Store) Courses(ctx context.Context) ([]CourseReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(a.course, ''), NULLIF(s.description, ''), 'Unbekannt') AS course,
			COUNT(DISTINCT a.course_session_id),
			COUNT(*),
			SUM(a.attended),
			SUM(a.signed),
			SUM(a.cancelled),
			SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0 THEN 1 ELSE 0 END)
		FROM attendance a
		JOIN course_sessions s ON s.myyolo_id = a.course_session_id
		GROUP BY course
		ORDER BY course`)
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
		); err != nil {
			return nil, fmt.Errorf("scan course report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Days(ctx context.Context) ([]DayReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(a.date_value, ''), substr(s.date_iso, 1, 10), s.date_formatted) AS day,
			COUNT(DISTINCT a.course_session_id),
			COUNT(*),
			SUM(a.attended),
			SUM(a.signed),
			SUM(a.cancelled),
			SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0 THEN 1 ELSE 0 END)
		FROM attendance a
		JOIN course_sessions s ON s.myyolo_id = a.course_session_id
		GROUP BY day
		ORDER BY day`)
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
		); err != nil {
			return nil, fmt.Errorf("scan daily report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Hours(ctx context.Context) ([]HourReport, error) {
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
			SUM(CASE WHEN a.attended=0 AND a.cancelled=0 THEN 1 ELSE 0 END)
		FROM attendance a
		GROUP BY hour
		ORDER BY hour`)
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
		); err != nil {
			return nil, fmt.Errorf("scan hourly report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Sessions(ctx context.Context) ([]SessionReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(s.date_iso, ''), s.date_formatted),
			s.time_formatted,
			s.description,
			s.room_name,
			COUNT(a.myyolo_id),
			COALESCE(SUM(a.attended), 0),
			COALESCE(SUM(a.signed), 0),
			COALESCE(SUM(a.cancelled), 0),
			COALESCE(SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0)
		FROM course_sessions s
		LEFT JOIN attendance a ON a.course_session_id = s.myyolo_id
		GROUP BY s.myyolo_id
		ORDER BY s.date_iso, s.time_formatted, s.description`)
	if err != nil {
		return nil, fmt.Errorf("query session report: %w", err)
	}
	defer rows.Close()
	var report []SessionReport
	for rows.Next() {
		var item SessionReport
		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("scan session report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func (store *Store) Members(ctx context.Context) ([]MemberReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			m.myyolo_id,
			COALESCE(m.member_number, ''),
			m.first_name,
			m.last_name,
			COUNT(a.myyolo_id),
			COALESCE(SUM(a.attended), 0),
			COALESCE(SUM(a.signed), 0),
			COALESCE(SUM(a.cancelled), 0),
			COALESCE(SUM(CASE WHEN a.attended=1 AND a.signed=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.attended=0 AND a.cancelled=0 THEN 1 ELSE 0 END), 0)
		FROM members m
		LEFT JOIN attendance a ON a.member_id = m.myyolo_id
		GROUP BY m.myyolo_id
		ORDER BY m.last_name, m.first_name, m.myyolo_id`)
	if err != nil {
		return nil, fmt.Errorf("query member report: %w", err)
	}
	defer rows.Close()
	var report []MemberReport
	for rows.Next() {
		var item MemberReport
		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("scan member report: %w", err)
		}
		report = append(report, item)
	}
	return report, rows.Err()
}

func boolInt(value mysign.Bool) int {
	if bool(value) {
		return 1
	}
	return 0
}
