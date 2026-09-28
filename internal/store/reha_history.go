package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const rehaHistorySchemaVersion = "reha-history-observations.v1"
const RehaHistoryManagementSchemaVersion = "reha-history-observations.v2"

// RehaHistoryManagementFields are opt-in management fields. Null means unknown,
// never zero. mySIGN provides no separate recurring course identity or capacity.
type RehaHistoryManagementFields struct {
	CourseLabel          string                `json:"course_label"`
	StableCourseID       *string               `json:"stable_course_id"`
	Registered           *int                  `json:"registered"`
	Participated         *int                  `json:"participated"`
	Capacity             *int                  `json:"capacity"`
	AttendanceObservedAt *string               `json:"attendance_observed_at"`
	RegistrationStatus   string                `json:"registration_status"`
	AttendanceStatus     string                `json:"attendance_status"`
	RegisteredObservedAt *string               `json:"registered_observed_at"`
	Completeness         string                `json:"completeness"`
	Provenance           RehaHistoryProvenance `json:"provenance"`
}

type RehaHistoryProvenance struct {
	Registered     string `json:"registered"`
	Participated   string `json:"participated"`
	CourseIdentity string `json:"course_identity"`
	Capacity       string `json:"capacity"`
}

// RehaHistorySession is the aggregate-safe representation of one archived
// source session. Member and prescription identities are never exported.
// Management fields are present only in the explicitly requested v2 contract.
type RehaHistorySession struct {
	*RehaHistoryManagementFields
	StableSessionID            string `json:"stable_session_id"`
	Date                       string `json:"date"`
	Time                       string `json:"time"`
	AttendanceRows             int    `json:"attendance_rows"`
	AttendedNotCancelled       int    `json:"attended_not_cancelled"`
	SignedAttendedNotCancelled int    `json:"signed_attended_not_cancelled"`
	MissingSignatureCandidate  int    `json:"missing_signature_candidate"`
	ObservedAt                 string `json:"observed_at"`
}

type RehaHistoryCoverage struct {
	SessionInventory                 string `json:"session_inventory"`
	RegistrationAvailability         string `json:"registration_availability"`
	AttendanceCollectionCompleteness string `json:"attendance_collection_completeness"`
	ObservedSessions                 int    `json:"observed_sessions"`
	RegistrationAvailableSessions    int    `json:"registration_available_sessions"`
	AttendanceAvailableSessions      int    `json:"attendance_available_sessions"`
	ExpectedSessions                 *int   `json:"expected_sessions"`
	MissingSessions                  *int   `json:"missing_sessions"`
}

type RehaHistoryReport struct {
	CoverageDetail *RehaHistoryCoverage `json:"coverage_detail,omitempty"`
	RequestedFrom  string               `json:"requested_from,omitempty"`
	RequestedTo    string               `json:"requested_to,omitempty"`
	AsOf           string               `json:"as_of,omitempty"`
	Timezone       string               `json:"timezone,omitempty"`
	SchemaVersion  string               `json:"schema_version"`
	Source         string               `json:"source"`
	Location       string               `json:"location"`
	GeneratedAt    string               `json:"generated_at"`
	SnapshotAt     string               `json:"snapshot_at"`
	SnapshotSHA256 string               `json:"snapshot_sha256"`
	Coverage       string               `json:"coverage"`
	Sessions       []RehaHistorySession `json:"sessions"`
}

type rehaHistorySQLRow struct {
	description           sql.NullString
	participantCount      sql.NullInt64
	source                string
	sessionID             string
	dateISO               sql.NullString
	dateFallback          sql.NullString
	timeValue             sql.NullString
	sessionUpdatedAt      sql.NullString
	sessionCurrentWeek    sql.NullInt64
	attendanceID          sql.NullString
	attendanceUpdated     sql.NullString
	attended              sql.NullInt64
	signed                sql.NullInt64
	signedManually        sql.NullInt64
	cancelled             sql.NullInt64
	manualMemberTime      sql.NullInt64
	previousDay           sql.NullInt64
	attendanceCurrentWeek sql.NullInt64
}

type rehaHistoryAccumulator struct {
	description                string
	participantCount           sql.NullInt64
	sessionObservedAt          time.Time
	sessionID                  string
	date                       time.Time
	timeValue                  string
	observedAt                 time.Time
	attendanceRows             int
	attendedNotCancelled       int
	signedAttendedNotCancelled int
	missingSignatureCandidate  int
}

// RehaHistory reads accumulated mySIGN rows from a hash-bound snapshot. It
// uses the latest stored row observation for each session and never creates,
// migrates, or writes a history marker. Since the snapshot stores only the
// latest aggregate row, an as-of before that observation cannot reconstruct
// the earlier state; an affected in-range session therefore fails closed.
func (store *ReadOnlyStore) RehaHistory(
	ctx context.Context,
	source string,
	location string,
	from time.Time,
	to time.Time,
	asOf time.Time,
	snapshotSHA256 string,
) (RehaHistoryReport, error) {
	return store.RehaHistoryWithSchema(ctx, source, location, from, to, asOf, snapshotSHA256, rehaHistorySchemaVersion)
}

// RehaHistoryWithSchema preserves v1 and explicitly opts in to management v2.
func (store *ReadOnlyStore) RehaHistoryWithSchema(ctx context.Context, source, location string, from, to, asOf time.Time, snapshotSHA256, schema string) (RehaHistoryReport, error) {
	if schema != rehaHistorySchemaVersion && schema != RehaHistoryManagementSchemaVersion {
		return RehaHistoryReport{}, errors.New("unsupported Reha history schema")
	}
	if store == nil || store.db == nil {
		return RehaHistoryReport{}, errors.New("read-only database is not open")
	}
	if source != "mysign" {
		return RehaHistoryReport{}, errors.New("unsupported Reha source")
	}
	if location == "" {
		return RehaHistoryReport{}, errors.New("Reha location is required")
	}
	if from.Location().String() != rehaTimezone || to.Location().String() != rehaTimezone {
		return RehaHistoryReport{}, errors.New("Reha date bounds must use Europe/Berlin")
	}
	if from.After(to) {
		return RehaHistoryReport{}, errors.New("Reha from date is after to date")
	}
	if asOf.IsZero() {
		return RehaHistoryReport{}, errors.New("Reha as-of timestamp is required")
	}

	var duplicate int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT source, member_id, course_session_id
			FROM attendance
			WHERE source=?
			GROUP BY source, member_id, course_session_id
			HAVING COUNT(*) > 1
		)`, source).Scan(&duplicate); err != nil {
		return RehaHistoryReport{}, fmt.Errorf("check Reha history attendance duplicates: %w", err)
	}
	if duplicate != 0 {
		return RehaHistoryReport{}, errors.New("duplicate attendance source/member/session pair")
	}

	var orphan int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM attendance a
		LEFT JOIN course_sessions s
		  ON s.source=a.source AND s.myyolo_id=a.course_session_id
		WHERE a.source=? AND s.myyolo_id IS NULL`, source).Scan(&orphan); err != nil {
		return RehaHistoryReport{}, fmt.Errorf("check Reha history attendance references: %w", err)
	}
	if orphan != 0 {
		return RehaHistoryReport{}, errors.New("attendance references an unknown Reha session")
	}

	proof := map[string]rehaHistoryCountProof{}
	var legacyProofAt string
	if schema == RehaHistoryManagementSchemaVersion {
		var err error
		proof, legacyProofAt, err = store.rehaHistoryCountProof(ctx, source)
		if err != nil {
			return RehaHistoryReport{}, err
		}
	}
	// v1 must still read legacy snapshots that predate management columns.
	managementColumns := "NULL, NULL,"
	if schema == RehaHistoryManagementSchemaVersion {
		managementColumns = "s.description, s.participant_count,"
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT `+managementColumns+`

			s.source,
			s.myyolo_id,
			s.date_iso,
			s.date_formatted,
			s.time_formatted,
			s.updated_at,
			s.current_week,
			a.myyolo_id,
			a.updated_at,
			a.attended,
			a.signed,
			a.signed_manually,
			a.cancelled,
			a.manual_member_time,
			a.training_previous_day,
			a.current_week
		FROM course_sessions s
		LEFT JOIN attendance a
		  ON a.source=s.source AND a.course_session_id=s.myyolo_id
		WHERE s.source=?
		ORDER BY s.date_iso, s.time_formatted, s.myyolo_id, a.myyolo_id`, source)
	if err != nil {
		return RehaHistoryReport{}, fmt.Errorf("query Reha history: %w", err)
	}
	defer rows.Close()

	locationBerlin, err := berlinLocation()
	if err != nil {
		return RehaHistoryReport{}, err
	}
	asOfUTC := asOf.UTC()
	byID := make(map[string]*rehaHistoryAccumulator)
	for rows.Next() {
		var item rehaHistorySQLRow
		if err := rows.Scan(
			&item.description,
			&item.participantCount,
			&item.source,
			&item.sessionID,
			&item.dateISO,
			&item.dateFallback,
			&item.timeValue,
			&item.sessionUpdatedAt,
			&item.sessionCurrentWeek,
			&item.attendanceID,
			&item.attendanceUpdated,
			&item.attended,
			&item.signed,
			&item.signedManually,
			&item.cancelled,
			&item.manualMemberTime,
			&item.previousDay,
			&item.attendanceCurrentWeek,
		); err != nil {
			return RehaHistoryReport{}, fmt.Errorf("scan Reha history: %w", err)
		}
		if item.source != source || item.sessionID == "" {
			return RehaHistoryReport{}, errors.New("Reha history contains an invalid session identity")
		}
		if _, err := scanFlag(item.sessionCurrentWeek); err != nil {
			return RehaHistoryReport{}, errors.New("Reha history contains an invalid session boolean")
		}
		sessionObservedAt, err := parseHistoryTimestamp(item.sessionUpdatedAt)
		if err != nil {
			return RehaHistoryReport{}, errors.New("Reha history contains an invalid session observation timestamp")
		}

		acc, exists := byID[item.sessionID]
		if !exists {
			date, parseErr := parseSessionDate(
				nullableHistoryString(item.dateISO),
				nullableHistoryString(item.dateFallback),
				locationBerlin,
			)
			if parseErr != nil {
				return RehaHistoryReport{}, errors.New("Reha history contains an unnormalizable session date")
			}
			acc = &rehaHistoryAccumulator{
				description:       strings.Join(strings.Fields(item.description.String), " "),
				participantCount:  item.participantCount,
				sessionObservedAt: sessionObservedAt,
				sessionID:         item.sessionID,
				date:              date,
				timeValue:         safeTimeValue(nullableHistoryString(item.timeValue)),
				observedAt:        sessionObservedAt,
			}
			byID[item.sessionID] = acc
		} else if !acc.observedAt.Equal(sessionObservedAt) && sessionObservedAt.After(acc.observedAt) {
			acc.observedAt = sessionObservedAt
		}
		if !item.attendanceID.Valid {
			continue
		}
		if strings.TrimSpace(item.attendanceID.String) == "" {
			return RehaHistoryReport{}, errors.New("Reha history contains an attendance row without an ID")
		}
		attendanceObservedAt, err := parseHistoryTimestamp(item.attendanceUpdated)
		if err != nil {
			return RehaHistoryReport{}, errors.New("Reha history contains an invalid attendance observation timestamp")
		}
		if attendanceObservedAt.After(acc.observedAt) {
			acc.observedAt = attendanceObservedAt
		}
		flags := []*sql.NullInt64{
			&item.attended,
			&item.signed,
			&item.signedManually,
			&item.cancelled,
			&item.manualMemberTime,
			&item.previousDay,
			&item.attendanceCurrentWeek,
		}
		values := make([]int, len(flags))
		for index, flag := range flags {
			values[index], err = scanFlag(*flag)
			if err != nil {
				return RehaHistoryReport{}, errors.New("Reha history contains an invalid attendance boolean")
			}
		}
		attended, signed, cancelled := values[0], values[1], values[3]
		acc.attendanceRows++
		if attended == 1 && cancelled == 0 {
			acc.attendedNotCancelled++
			if signed == 1 {
				acc.signedAttendedNotCancelled++
			}
			if signed == 0 {
				acc.missingSignatureCandidate++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return RehaHistoryReport{}, fmt.Errorf("read Reha history: %w", err)
	}

	fromDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, locationBerlin)
	toExclusive := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, locationBerlin).AddDate(0, 0, 1)
	result := make([]RehaHistorySession, 0, len(byID))
	for _, acc := range byID {
		day := acc.date.In(locationBerlin)
		if day.Before(fromDate) || !day.Before(toExclusive) {
			continue
		}
		if acc.observedAt.After(asOfUTC) {
			return RehaHistoryReport{}, errors.New("Reha history cannot reconstruct an in-range session before as-of from the latest stored observation")
		}
		row := RehaHistorySession{
			StableSessionID:            stableSessionID(source, acc.sessionID),
			Date:                       day.Format("2006-01-02"),
			Time:                       acc.timeValue,
			AttendanceRows:             acc.attendanceRows,
			AttendedNotCancelled:       acc.attendedNotCancelled,
			SignedAttendedNotCancelled: acc.signedAttendedNotCancelled,
			MissingSignatureCandidate:  acc.missingSignatureCandidate,
			ObservedAt:                 acc.observedAt.UTC().Format(time.RFC3339Nano),
		}
		if schema == RehaHistoryManagementSchemaVersion {
			fields := &RehaHistoryManagementFields{
				CourseLabel:        acc.description,
				RegistrationStatus: "unknown",
				AttendanceStatus:   "unknown_unvalidated_collection",
				Completeness:       "observed_rows_only",
				Provenance: RehaHistoryProvenance{
					Registered:     "unknown_unvalidated_observation",
					Participated:   "mysign.KursTeilnehmer:signed_and_attended_and_not_cancelled",
					CourseIdentity: "unknown_not_provided_by_mysign",
					Capacity:       "unknown_not_provided_by_mysign",
				},
			}
			observed := acc.sessionObservedAt.UTC().Format(time.RFC3339Nano)
			p, found := proof[acc.sessionID]
			validated := found && p.validation == "required_nonnegative_integer_and_collections_v1" && p.observedAt == observed && p.count.Valid && p.count == acc.participantCount
			// Older fresh single-import snapshots already carry equivalent proof.
			if !found && legacyProofAt == observed {
				validated = true
			}
			if validated {
				if !acc.participantCount.Valid || acc.participantCount.Int64 < 0 {
					return RehaHistoryReport{}, errors.New("Reha history contains an invalid validated participant count")
				}
				count := int(acc.participantCount.Int64)
				fields.Registered = &count
				fields.RegistrationStatus = "validated_observation"
				fields.RegisteredObservedAt = &observed
				fields.Provenance.Registered = "mysign.KursBuchungen.TeilnehmerAnzahl:validated_observation"
			}
			if found && p.attendanceValidation == "complete_for_observed_snapshot" && p.observedAt == observed {
				if !p.attendanceRows.Valid || !p.participated.Valid || p.attendanceRows.Int64 < 0 || p.participated.Int64 < 0 || p.participated.Int64 > p.attendanceRows.Int64 {
					return RehaHistoryReport{}, errors.New("Reha history contains an invalid attendance collection proof")
				}
				participated := int(p.participated.Int64)
				fields.Participated = &participated
				fields.AttendanceObservedAt = &observed
				fields.AttendanceStatus = "complete_for_observed_snapshot"
			} else if !found && legacyProofAt == observed && acc.observedAt.Equal(acc.sessionObservedAt) {
				participated := acc.signedAttendedNotCancelled
				fields.Participated = &participated
				fields.AttendanceObservedAt = &observed
				fields.AttendanceStatus = "complete_for_observed_snapshot"
			}
			row.RehaHistoryManagementFields = fields
		}
		result = append(result, row)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Date != result[right].Date {
			return result[left].Date < result[right].Date
		}
		if result[left].Time != result[right].Time {
			return result[left].Time < result[right].Time
		}
		return result[left].StableSessionID < result[right].StableSessionID
	})
	report := RehaHistoryReport{
		SchemaVersion:  schema,
		Source:         source,
		Location:       location,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SnapshotSHA256: snapshotSHA256,
		Coverage:       "observed_rows_only",
		Sessions:       result,
	}
	if schema == RehaHistoryManagementSchemaVersion {
		report.RequestedFrom = fromDate.Format("2006-01-02")
		report.RequestedTo = toExclusive.AddDate(0, 0, -1).Format("2006-01-02")
		report.AsOf = asOfUTC.Format(time.RFC3339Nano)
		report.Timezone = rehaTimezone
		coverage := &RehaHistoryCoverage{SessionInventory: "observed_rows_only", ObservedSessions: len(result)}
		for _, row := range result {
			if row.Registered != nil {
				coverage.RegistrationAvailableSessions++
			}
			if row.Participated != nil {
				coverage.AttendanceAvailableSessions++
			}
		}
		coverage.RegistrationAvailability = historyAvailability(coverage.RegistrationAvailableSessions, len(result))
		coverage.AttendanceCollectionCompleteness = historyAvailability(coverage.AttendanceAvailableSessions, len(result))
		report.CoverageDetail = coverage
	}
	return report, nil
}

func parseHistoryTimestamp(value sql.NullString) (time.Time, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	return parseDataTimestamp(value.String)
}

func nullableHistoryString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// Optional additive observation metadata allows v5/v6 archived databases to
// remain readable without mutating or falsely upgrading their evidence.
type rehaHistoryCountProof struct {
	observedAt           string
	count                sql.NullInt64
	validation           string
	attendanceRows       sql.NullInt64
	participated         sql.NullInt64
	attendanceValidation string
}

func (store *ReadOnlyStore) rehaHistoryCountProof(ctx context.Context, source string) (map[string]rehaHistoryCountProof, string, error) {
	proof := make(map[string]rehaHistoryCountProof)
	var legacyProofAt string
	var exists int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='reha_import_observation'`).Scan(&exists); err != nil {
		return nil, "", err
	}
	if exists != 0 {
		var at, validation string
		err := store.db.QueryRowContext(ctx, `SELECT observed_at, validation FROM reha_import_observation WHERE singleton=1`).Scan(&at, &validation)
		if err != nil && err != sql.ErrNoRows {
			return nil, "", err
		}
		if err == nil && validation == "required_nonnegative_integer_and_collections_v1" {
			t, err := parseDataTimestamp(at)
			if err != nil {
				return nil, "", err
			}
			legacyProofAt = t.UTC().Format(time.RFC3339Nano)
		}
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='reha_session_observations'`).Scan(&exists); err != nil {
		return nil, "", err
	}
	if exists == 0 {
		return proof, legacyProofAt, nil
	}
	columns, err := store.db.QueryContext(ctx, `PRAGMA table_info(reha_session_observations)`)
	if err != nil {
		return nil, "", err
	}
	hasAttendance := false
	for columns.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := columns.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			columns.Close()
			return nil, "", err
		}
		if name == "attendance_validation" {
			hasAttendance = true
		}
	}
	columnErr := columns.Err()
	columns.Close()
	if columnErr != nil {
		return nil, "", columnErr
	}
	attendanceColumns := "NULL,NULL,'unknown'"
	if hasAttendance {
		attendanceColumns = "attendance_rows,participated,attendance_validation"
	}
	rows, err := store.db.QueryContext(ctx, `SELECT session_id, observed_at, participant_count, validation,`+attendanceColumns+` FROM reha_session_observations WHERE source=?`, source)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, at string
		var p rehaHistoryCountProof
		if err := rows.Scan(&id, &at, &p.count, &p.validation, &p.attendanceRows, &p.participated, &p.attendanceValidation); err != nil {
			return nil, "", err
		}
		t, err := parseDataTimestamp(at)
		if err != nil {
			return nil, "", err
		}
		p.observedAt = t.UTC().Format(time.RFC3339Nano)
		proof[id] = p
	}
	return proof, legacyProofAt, rows.Err()
}

func historyAvailability(available, total int) string {
	if available == 0 {
		return "unavailable"
	}
	if available == total {
		return "available_for_all_observed_sessions"
	}
	return "partial"
}
