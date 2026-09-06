package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type rehaSQLRow struct {
	source         string
	sessionID      string
	dateISO        string
	dateFallback   string
	timeValue      string
	planned        int
	endUTC         sql.NullString
	attendanceID   sql.NullString
	prescriptionID sql.NullString
	attended       sql.NullInt64
	signed         sql.NullInt64
	cancelled      sql.NullInt64
}

type rehaSessionAccumulator struct {
	source                           string
	sessionID                        string
	date                             time.Time
	timeValue                        string
	planned                          int
	endUTC                           sql.NullString
	attendanceRows                   int
	attendedFlagTrue                 int
	signedFlagTrue                   int
	cancelledFlagTrue                int
	attendedNotCancelled             int
	signedAttendedNotCancelled       int
	missingSignatureCandidate        int
	nonAttendedNotCancelledCandidate int
	prescriptionLinkedRows           int
	contradictory                    int
}

// RehaSessions reads one aggregate-safe row per concrete source course
// session. It deliberately selects no member, prescription, or course text.
func (store *ReadOnlyStore) RehaSessions(
	ctx context.Context,
	source string,
	location string,
	from time.Time,
	to time.Time,
	asOf time.Time,
	snapshotSHA256 string,
) (RehaSessionReport, error) {
	if store == nil || store.db == nil {
		return RehaSessionReport{}, errors.New("read-only database is not open")
	}
	if source != "mysign" {
		return RehaSessionReport{}, errors.New("unsupported Reha source")
	}
	if location == "" {
		return RehaSessionReport{}, errors.New("Reha location is required")
	}
	if from.Location().String() != rehaTimezone || to.Location().String() != rehaTimezone {
		return RehaSessionReport{}, errors.New("Reha date bounds must use Europe/Berlin")
	}
	if from.After(to) {
		return RehaSessionReport{}, errors.New("Reha from date is after to date")
	}

	var observedAt, validation string
	if err := store.db.QueryRowContext(ctx, `SELECT observed_at, validation FROM reha_import_observation WHERE singleton=1`).Scan(&observedAt, &validation); err != nil || validation != "required_nonnegative_integer_and_collections_v1" {
		return RehaSessionReport{}, errors.New("Reha report requires a fresh single validated import")
	}
	if _, err := parseDataTimestamp(observedAt); err != nil {
		return RehaSessionReport{}, errors.New("invalid import observation timestamp")
	}
	var duplicate int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT member_id, course_session_id FROM attendance WHERE source=? GROUP BY member_id, course_session_id HAVING COUNT(*)>1)`, source).Scan(&duplicate); err != nil || duplicate != 0 {
		return RehaSessionReport{}, errors.New("duplicate attendance member-session pair or invalid source")
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			s.source,
			s.myyolo_id,
			s.date_iso,
			s.date_formatted,
			s.time_formatted,
			s.participant_count,
			s.ends_at_utc,
			a.myyolo_id,
			a.prescription_id,
			a.attended,
			a.signed,
			a.cancelled
		FROM course_sessions s
		LEFT JOIN attendance a
		  ON a.source = s.source AND a.course_session_id = s.myyolo_id
		WHERE s.source = ?
		ORDER BY s.date_iso, s.time_formatted, s.myyolo_id, a.myyolo_id`, source)
	if err != nil {
		return RehaSessionReport{}, fmt.Errorf("query Reha sessions: %w", err)
	}
	defer rows.Close()

	locationBerlin, err := berlinLocation()
	if err != nil {
		return RehaSessionReport{}, err
	}
	byID := make(map[string]*rehaSessionAccumulator)
	var invalidDates, invalidEnds int
	var sourceSessions int
	for rows.Next() {
		var item rehaSQLRow
		if err := rows.Scan(
			&item.source,
			&item.sessionID,
			&item.dateISO,
			&item.dateFallback,
			&item.timeValue,
			&item.planned,
			&item.endUTC,
			&item.attendanceID,
			&item.prescriptionID,
			&item.attended,
			&item.signed,
			&item.cancelled,
		); err != nil {
			return RehaSessionReport{}, fmt.Errorf("scan Reha session: %w", err)
		}
		if item.sessionID == "" {
			return RehaSessionReport{}, errors.New("Reha source contains a session without an ID")
		}
		acc, exists := byID[item.sessionID]
		if !exists {
			sourceSessions++
			date, parseErr := parseSessionDate(item.dateISO, item.dateFallback, locationBerlin)
			if parseErr != nil {
				invalidDates++
				return RehaSessionReport{}, fmt.Errorf("Reha source contains an unnormalizable session date")
			}
			acc = &rehaSessionAccumulator{
				source:    item.source,
				sessionID: item.sessionID,
				date:      date,
				timeValue: safeTimeValue(item.timeValue),
				planned:   item.planned,
				endUTC:    item.endUTC,
			}
			byID[item.sessionID] = acc
		} else if acc.endUTC.String != item.endUTC.String || acc.endUTC.Valid != item.endUTC.Valid {
			return RehaSessionReport{}, errors.New("Reha source contains inconsistent session end values")
		}
		if item.planned < 0 {
			return RehaSessionReport{}, errors.New("Reha source contains a negative participant count")
		}
		if exists && acc.planned != item.planned {
			return RehaSessionReport{}, errors.New("Reha source contains inconsistent participant counts")
		}
		if !item.attendanceID.Valid {
			continue
		}
		attended, err := scanFlag(item.attended)
		if err != nil {
			return RehaSessionReport{}, errors.New("Reha source contains an invalid attendance flag")
		}
		signed, err := scanFlag(item.signed)
		if err != nil {
			return RehaSessionReport{}, errors.New("Reha source contains an invalid signature flag")
		}
		cancelled, err := scanFlag(item.cancelled)
		if err != nil {
			return RehaSessionReport{}, errors.New("Reha source contains an invalid cancellation flag")
		}
		acc.attendanceRows++
		acc.attendedFlagTrue += attended
		acc.signedFlagTrue += signed
		acc.cancelledFlagTrue += cancelled
		if cancelled == 0 {
			acc.attendedNotCancelled += attended
			acc.signedAttendedNotCancelled += attended * signed
		}
		if attended == 1 && signed == 0 && cancelled == 0 {
			acc.missingSignatureCandidate++
		}
		if attended == 0 && cancelled == 0 {
			acc.nonAttendedNotCancelledCandidate++
		}
		if item.prescriptionID.Valid && strings.TrimSpace(item.prescriptionID.String) != "" {
			acc.prescriptionLinkedRows++
		}
		// A signature without usable attendance or attendance plus cancellation
		// is a contradictory source row; count each row only once here.
		if (attended == 1 && cancelled == 1) || (signed == 1 && (attended == 0 || cancelled == 1)) {
			acc.contradictory++
		}
	}
	if err := rows.Err(); err != nil {
		return RehaSessionReport{}, fmt.Errorf("read Reha sessions: %w", err)
	}

	fromDate := time.Date(from.In(locationBerlin).Year(), from.In(locationBerlin).Month(), from.In(locationBerlin).Day(), 0, 0, 0, 0, locationBerlin)
	toExclusive := time.Date(to.In(locationBerlin).Year(), to.In(locationBerlin).Month(), to.In(locationBerlin).Day(), 0, 0, 0, 0, locationBerlin).AddDate(0, 0, 1)
	var resultRows []RehaSessionRow
	observedFrom, observedTo := "", ""
	for _, acc := range byID {
		day := acc.date.In(locationBerlin)
		if day.Before(fromDate) || !day.Before(toExclusive) {
			continue
		}
		if observedFrom == "" || day.Format("2006-01-02") < observedFrom {
			observedFrom = day.Format("2006-01-02")
		}
		if observedTo == "" || day.Format("2006-01-02") > observedTo {
			observedTo = day.Format("2006-01-02")
		}
		if acc.endUTC.Valid && strings.TrimSpace(acc.endUTC.String) != "" {
			if _, parseErr := time.Parse(time.RFC3339, acc.endUTC.String); parseErr != nil {
				invalidEnds++
			}
		}
		resultRows = append(resultRows, RehaSessionRow{
			Source:                             source,
			Location:                           location,
			StableSessionID:                    stableSessionID(source, acc.sessionID),
			Date:                               day.Format("2006-01-02"),
			Time:                               acc.timeValue,
			ParticipantCountCurrentObservation: acc.planned,
			AttendanceRows:                     acc.attendanceRows,
			AttendedFlagTrue:                   acc.attendedFlagTrue,
			SignedFlagTrue:                     acc.signedFlagTrue,
			CancelledFlagTrue:                  acc.cancelledFlagTrue,
			AttendedNotCancelled:               acc.attendedNotCancelled,
			SignedAttendedNotCancelled:         acc.signedAttendedNotCancelled,
			MissingSignatureCandidate:          acc.missingSignatureCandidate,
			NonAttendedNotCancelledCandidate:   acc.nonAttendedNotCancelledCandidate,
			PrescriptionLinkedRows:             acc.prescriptionLinkedRows,
			PrescriptionUnlinkedRows:           acc.attendanceRows - acc.prescriptionLinkedRows,
			ContradictoryFlags:                 acc.contradictory,
		})
	}
	if resultRows == nil {
		resultRows = []RehaSessionRow{}
	}
	// Map iteration is deliberately not used as output order.
	sortRehaRows(resultRows)
	_, err = store.rehaDataAsOf(ctx, source)
	if err != nil {
		return RehaSessionReport{}, err
	}
	return RehaSessionReport{
		ReportVersion:              3,
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339Nano),
		ImportObservedAt:           observedAt,
		ObservationBasis:           "single_validated_import",
		ParticipantCountValidation: "required_nonnegative_integer_v1",
		PopulationStatus:           "reha_membership_unproven",
		RequestedFrom:              from.Format("2006-01-02"),
		RequestedTo:                to.Format("2006-01-02"),
		Timezone:                   rehaTimezone,
		AsOf:                       asOf.UTC().Format(time.RFC3339Nano),
		Source:                     source,
		Location:                   location,
		ScopeBasis:                 "external_verified_scope",
		SnapshotSHA256:             snapshotSHA256,
		DataAsOf:                   observedAt,
		Coverage: RehaCoverage{
			Status:          "observed_rows_only",
			SourceSessions:  sourceSessions,
			SessionsInRange: len(resultRows),
			InvalidDateRows: invalidDates,
			InvalidEndRows:  invalidEnds,
			ObservedFrom:    observedFrom,
			ObservedTo:      observedTo,
			Gaps: []string{
				"source_completeness_not_proven",
				"historical_plan_observations_unavailable",
			},
		},
		Sessions: resultRows,
	}, nil
}

func (store *ReadOnlyStore) rehaDataAsOf(ctx context.Context, source string) (string, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT updated_at FROM course_sessions WHERE source = ?
		UNION ALL
		SELECT updated_at FROM attendance WHERE source = ?`, source, source)
	if err != nil {
		return "", fmt.Errorf("query Reha data timestamp: %w", err)
	}
	defer rows.Close()
	var latest time.Time
	seen := false
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			return "", fmt.Errorf("scan Reha data timestamp: %w", err)
		}
		if !value.Valid {
			return "", errors.New("Reha source contains a missing data timestamp")
		}
		parsed, err := parseDataTimestamp(value.String)
		if err != nil {
			return "", errors.New("Reha source contains an invalid data timestamp")
		}
		if !seen || parsed.After(latest) {
			latest = parsed
			seen = true
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read Reha data timestamp: %w", err)
	}
	if !seen {
		return "unknown", nil
	}
	return latest.UTC().Format(time.RFC3339Nano), nil
}

func parseDataTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, errors.New("empty data timestamp")
	}
	return time.Parse(time.RFC3339Nano, trimmed)
}

func parseSessionDate(dateISO, fallback string, location *time.Location) (time.Time, error) {
	if strings.TrimSpace(dateISO) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(dateISO))
		if err != nil {
			return time.Time{}, err
		}
		return parsed.In(location), nil
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "02/01/2006"} {
		if parsed, err := time.ParseInLocation(layout, strings.TrimSpace(fallback), location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("no session date")
}

func scanFlag(value sql.NullInt64) (int, error) {
	if !value.Valid {
		return 0, errors.New("flag is missing")
	}
	if value.Int64 != 0 && value.Int64 != 1 {
		return 0, errors.New("flag is not boolean")
	}
	return int(value.Int64), nil
}

func safeTimeValue(value string) string {
	value = strings.TrimSpace(value)
	match := sessionTimeRangePattern.FindStringSubmatch(value)
	if len(match) != 5 {
		return "unknown"
	}
	for _, clock := range []string{match[1], match[3]} {
		if len(clock) != 2 || clock[0] < '0' || clock[0] > '2' || clock[1] < '0' || clock[1] > '9' {
			return "unknown"
		}
		if clock[0] == '2' && clock[1] > '3' {
			return "unknown"
		}
	}
	return match[1] + ":" + match[2] + " - " + match[3] + ":" + match[4]
}

func stableSessionID(source, remoteID string) string {
	sum := sha256.Sum256([]byte("myyolo-reha-session-v1\x00" + source + "\x00" + remoteID))
	return hex.EncodeToString(sum[:])
}

func sortRehaRows(rows []RehaSessionRow) {
	for i := 1; i < len(rows); i++ {
		current := rows[i]
		j := i - 1
		for j >= 0 && rehaRowLess(current, rows[j]) {
			rows[j+1] = rows[j]
			j--
		}
		rows[j+1] = current
	}
}

func rehaRowLess(left, right RehaSessionRow) bool {
	if left.Date != right.Date {
		return left.Date < right.Date
	}
	if left.Time != right.Time {
		return left.Time < right.Time
	}
	return left.StableSessionID < right.StableSessionID
}
