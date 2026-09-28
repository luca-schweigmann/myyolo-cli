package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRehaHistoryReturnsInclusiveRowsUpToObservationAsOf(t *testing.T) {
	snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
		insertHistoryMember(t, db, "member-a")
		insertHistoryMember(t, db, "member-b")
		insertHistoryMember(t, db, "member-c")
		insertHistorySession(t, db, "oldest", "2026-06-16T08:00:00+02:00", "2026-06-17T09:00:00Z", "10:00 - 10:45")
		insertHistorySession(t, db, "same-looking-a", "2026-07-01T08:00:00+02:00", "2026-07-02T09:00:00Z", "10:00 - 10:45")
		insertHistorySession(t, db, "same-looking-b", "2026-07-01T08:00:00+02:00", "2026-07-02T09:00:00Z", "11:00 - 11:45")
		insertHistorySession(t, db, "latest-attendance", "2026-08-21T08:00:00+02:00", "2026-08-22T09:00:00Z", "12:00 - 12:45")
		insertHistorySession(t, db, "future-observation", "2026-09-20T08:00:00+02:00", "2026-09-02T09:00:00Z", "13:00 - 13:45")
		insertHistoryAttendance(t, db, "attendance-oldest", "member-a", "oldest", 1, 1, 0, "2026-06-17T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-a", "member-b", "same-looking-a", 1, 0, 0, "2026-07-02T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-b", "member-c", "same-looking-b", 1, 1, 1, "2026-07-02T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-latest", "member-a", "latest-attendance", 1, 1, 0, "2026-08-23T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-future", "member-a", "future-observation", 1, 1, 0, "2026-09-02T09:05:00Z")
	})
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(context.Background(), snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()

	report, err := readonly.RehaHistory(
		context.Background(),
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-06-16"),
		mustBerlinDate(t, "2026-08-21"),
		mustUTC(t, "2026-09-01T00:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != rehaHistorySchemaVersion || report.Coverage != "observed_rows_only" {
		t.Fatalf("report metadata = %#v", report)
	}
	if got, want := len(report.Sessions), 4; got != want {
		t.Fatalf("sessions = %d, want %d: %#v", got, want, report.Sessions)
	}
	if report.Sessions[0].Date != "2026-06-16" || report.Sessions[len(report.Sessions)-1].Date != "2026-08-21" {
		t.Fatalf("date bounds were not inclusive: %#v", report.Sessions)
	}
	byID := make(map[string]RehaHistorySession, len(report.Sessions))
	for _, row := range report.Sessions {
		byID[row.StableSessionID] = row
	}
	oldest := byID[stableSessionID("mysign", "oldest")]
	if oldest.AttendanceRows != 1 || oldest.AttendedNotCancelled != 1 || oldest.SignedAttendedNotCancelled != 1 || oldest.MissingSignatureCandidate != 0 || oldest.ObservedAt != "2026-06-17T09:05:00Z" {
		t.Fatalf("oldest row = %#v", oldest)
	}
	a := byID[stableSessionID("mysign", "same-looking-a")]
	if a.MissingSignatureCandidate != 1 || a.SignedAttendedNotCancelled != 0 {
		t.Fatalf("missing-signature row = %#v", a)
	}
	b := byID[stableSessionID("mysign", "same-looking-b")]
	if b.AttendedNotCancelled != 0 || b.SignedAttendedNotCancelled != 0 {
		t.Fatalf("cancelled row = %#v", b)
	}
	latest := byID[stableSessionID("mysign", "latest-attendance")]
	if latest.ObservedAt != "2026-08-23T09:05:00Z" {
		t.Fatalf("latest stored observation = %#v", latest)
	}
	if stableSessionID("mysign", "same-looking-a") == stableSessionID("mysign", "same-looking-b") {
		t.Fatal("distinct source IDs produced the same stable session ID")
	}
	serialized, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"member-a", "member-b", "attendance-a", "same-looking-a"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("history report leaked %q: %s", forbidden, serialized)
		}
	}
	var markerCount int
	if err := readonly.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='reha_import_observation'`).Scan(&markerCount); err != nil {
		t.Fatal(err)
	}
	if markerCount != 0 {
		t.Fatalf("history report fabricated a marker table: %d", markerCount)
	}
}

func TestRehaHistoryFailsClosedForFutureObservation(t *testing.T) {
	snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
		insertHistoryMember(t, db, "member-a")
		insertHistorySession(t, db, "future-observation", "2026-07-20T08:00:00+02:00", "2026-09-02T09:00:00Z", "13:00 - 13:45")
		insertHistoryAttendance(t, db, "attendance-a", "member-a", "future-observation", 1, 1, 0, "2026-09-02T09:05:00Z")
	})
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(context.Background(), snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	_, err = readonly.RehaHistory(
		context.Background(),
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-07-01"),
		mustBerlinDate(t, "2026-07-31"),
		mustUTC(t, "2026-09-01T00:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err == nil || !strings.Contains(err.Error(), "cannot reconstruct an in-range session before as-of") {
		t.Fatalf("future observation error = %v", err)
	}
}

func TestRehaHistoryRejectsDuplicateAndInvalidBooleans(t *testing.T) {
	t.Run("duplicate source member session", func(t *testing.T) {
		snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
			insertHistoryMember(t, db, "member-a")
			insertHistorySession(t, db, "session-a", "2026-07-01T08:00:00+02:00", "2026-07-01T09:00:00Z", "10:00 - 10:45")
			insertHistoryAttendance(t, db, "attendance-a", "member-a", "session-a", 1, 1, 0, "2026-07-01T09:05:00Z")
			insertHistoryAttendance(t, db, "attendance-b", "member-a", "session-a", 1, 1, 0, "2026-07-01T09:06:00Z")
		})
		assertHistoryFails(t, snapshotPath, receiptPath, "duplicate attendance source/member/session pair")
	})
	t.Run("invalid boolean", func(t *testing.T) {
		snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
			insertHistoryMember(t, db, "member-a")
			insertHistorySession(t, db, "session-a", "2026-07-01T08:00:00+02:00", "2026-07-01T09:00:00Z", "10:00 - 10:45")
			insertHistoryAttendance(t, db, "attendance-a", "member-a", "session-a", 1, 2, 0, "2026-07-01T09:05:00Z")
		})
		assertHistoryFails(t, snapshotPath, receiptPath, "invalid attendance boolean")
	})
	t.Run("null boolean", func(t *testing.T) {
		snapshotPath, receiptPath := createNullableHistorySnapshot(t)
		assertHistoryFails(t, snapshotPath, receiptPath, "invalid attendance boolean")
	})
}

func assertHistoryFails(t *testing.T, snapshotPath, receiptPath, expected string) {
	t.Helper()
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(context.Background(), snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	_, err = readonly.RehaHistory(
		context.Background(),
		receipt.Source,
		receipt.Location,
		mustBerlinDate(t, "2026-07-01"),
		mustBerlinDate(t, "2026-07-01"),
		mustUTC(t, "2026-09-01T00:00:00Z"),
		receipt.SnapshotSHA256,
	)
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("error = %v, want %q", err, expected)
	}
}

func createHistorySnapshot(t *testing.T, setup func(*testing.T, *sql.DB)) (string, string) {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	db, err := Open(context.Background(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	setup(t, db.db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	snapshotPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "snapshot.scope.json")
	if _, err := CreateSnapshot(context.Background(), sourcePath, snapshotPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	return snapshotPath, receiptPath
}

func insertHistoryMember(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO members(source,myyolo_id,member_number,first_name,last_name,updated_at) VALUES('mysign',?,?,?,?,?)`, id, id, "Synthetic", "Member", "2026-07-01T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
}

func insertHistorySession(t *testing.T, db *sql.DB, id, dateISO, updatedAt, timeValue string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO course_sessions(source,myyolo_id,date_iso,date_formatted,time_formatted,current_week,updated_at) VALUES('mysign',?,?,?,?,0,?)`, id, dateISO, dateISO[:10], timeValue, updatedAt); err != nil {
		t.Fatal(err)
	}
}

func insertHistoryAttendance(t *testing.T, db *sql.DB, id, memberID, sessionID string, attended, signed, cancelled int, updatedAt string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO attendance(source,myyolo_id,member_id,course_session_id,attended,signed,signed_manually,cancelled,manual_member_time,training_previous_day,current_week,updated_at) VALUES('mysign',?,?,?,?,?,0,?,0,0,0,?)`, id, memberID, sessionID, attended, signed, cancelled, updatedAt); err != nil {
		t.Fatal(err)
	}
}

func createNullableHistorySnapshot(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "nullable-source.sqlite")
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		PRAGMA user_version=5;
		CREATE TABLE course_sessions(source TEXT, myyolo_id TEXT, date_iso TEXT, date_formatted TEXT, time_formatted TEXT, current_week INTEGER, updated_at TEXT);
		CREATE TABLE attendance(source TEXT, myyolo_id TEXT, member_id TEXT, course_session_id TEXT, attended INTEGER, signed INTEGER, signed_manually INTEGER, cancelled INTEGER, manual_member_time INTEGER, training_previous_day INTEGER, current_week INTEGER, updated_at TEXT);
		INSERT INTO course_sessions VALUES('mysign','session-a','2026-07-01T08:00:00+02:00','01.07.2026','10:00 - 10:45',0,'2026-07-01T09:00:00Z');
		INSERT INTO attendance VALUES('mysign','attendance-a','member-a','session-a',1,NULL,0,0,0,0,0,'2026-07-01T09:05:00Z');`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sourcePath, 0o600); err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	snapshotPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "snapshot.scope.json")
	if _, err := CreateSnapshot(context.Background(), sourcePath, snapshotPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	return snapshotPath, receiptPath
}

func TestRehaHistoryV2ManagementCountsAndIdentity(t *testing.T) {
	snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
		for _, id := range []string{"member-a", "member-b", "member-c", "member-d"} {
			insertHistoryMember(t, db, id)
		}
		for _, id := range []string{"session-a", "session-b", "legacy-zero"} {
			insertHistorySession(t, db, id, "2026-07-01T08:00:00+02:00", "2026-07-01T09:00:00Z", "10:00 - 10:45")
		}
		if _, err := db.Exec(`UPDATE course_sessions SET description='Reha gleichnamig';
			UPDATE course_sessions SET participant_count=4 WHERE myyolo_id='session-a';
			INSERT INTO reha_session_observations(source,session_id,observed_at,participant_count,validation) VALUES('mysign','session-a','2026-07-01T09:00:00Z',4,'required_nonnegative_integer_and_collections_v1');
			INSERT INTO reha_session_observations(source,session_id,observed_at,participant_count,validation) VALUES('mysign','session-b','2026-07-01T09:00:00Z',0,'required_nonnegative_integer_and_collections_v1');`); err != nil {
			t.Fatal(err)
		}
		insertHistoryAttendance(t, db, "attendance-a", "member-a", "session-a", 1, 1, 0, "2026-07-01T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-b", "member-b", "session-a", 1, 0, 0, "2026-07-01T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-c", "member-c", "session-a", 1, 1, 1, "2026-07-01T09:05:00Z")
		insertHistoryAttendance(t, db, "attendance-d", "member-d", "session-a", 0, 1, 0, "2026-07-01T09:05:00Z")
		if _, err := db.Exec(`UPDATE reha_session_observations SET attendance_rows=4,participated=1,attendance_validation='complete_for_observed_snapshot' WHERE session_id='session-a'; UPDATE reha_session_observations SET attendance_rows=0,participated=0,attendance_validation='complete_for_observed_snapshot' WHERE session_id='session-b'`); err != nil {
			t.Fatal(err)
		}
	})
	report := readHistoryV2(t, snapshotPath, receiptPath)
	if report.SchemaVersion != RehaHistoryManagementSchemaVersion || len(report.Sessions) != 3 {
		t.Fatalf("metadata/rows: %#v", report)
	}
	if report.RequestedFrom != "2026-07-01" || report.RequestedTo != "2026-07-31" || report.Timezone != "Europe/Berlin" || report.CoverageDetail == nil || report.CoverageDetail.ExpectedSessions != nil || report.CoverageDetail.MissingSessions != nil || report.CoverageDetail.RegistrationAvailableSessions != 2 || report.CoverageDetail.AttendanceAvailableSessions != 2 {
		t.Fatalf("coverage contract: %#v", report)
	}
	byID := map[string]RehaHistorySession{}
	for _, row := range report.Sessions {
		byID[row.StableSessionID] = row
		if row.CourseLabel != "Reha gleichnamig" || row.StableCourseID != nil || row.Capacity != nil || row.Completeness != "observed_rows_only" {
			t.Fatalf("management row: %#v", row)
		}
	}
	a := byID[stableSessionID("mysign", "session-a")]
	if a.Registered == nil || *a.Registered != 4 || (a.Participated == nil || *a.Participated != 1) || a.AttendanceRows != 4 {
		t.Fatalf("counts: %#v", a)
	}
	if a.RegisteredObservedAt == nil || *a.RegisteredObservedAt != "2026-07-01T09:00:00Z" || a.ObservedAt != "2026-07-01T09:05:00Z" {
		t.Fatalf("observation times: %#v", a)
	}
	b := byID[stableSessionID("mysign", "session-b")]
	if b.Registered == nil || *b.Registered != 0 || (b.Participated == nil || *b.Participated != 0) {
		t.Fatalf("true zero lost: %#v", b)
	}
	legacy := byID[stableSessionID("mysign", "legacy-zero")]
	if legacy.Registered != nil || legacy.RegisteredObservedAt != nil || legacy.Participated != nil || legacy.AttendanceObservedAt != nil {
		t.Fatalf("legacy default zero falsely validated: %#v", legacy)
	}
	again := readHistoryV2(t, snapshotPath, receiptPath)
	for i := range report.Sessions {
		if report.Sessions[i].StableSessionID != again.Sessions[i].StableSessionID {
			t.Fatal("unstable pseudonym")
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"member-a", "member-b", "attendance-a", "session-a", "session-b", "legacy-zero", "member_id", "prescription_id"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("privacy leak %q", forbidden)
		}
	}
	if !strings.Contains(string(data), `"registered":null`) || !strings.Contains(string(data), `"registered":0`) {
		t.Fatalf("unknown/zero contract missing: %s", data)
	}
}

func readHistoryV2(t *testing.T, snapshotPath, receiptPath string) RehaHistoryReport {
	t.Helper()
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenReadOnly(context.Background(), snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := db.RehaHistoryWithSchema(context.Background(), receipt.Source, receipt.Location, mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-07-31"), mustUTC(t, "2030-01-01T00:00:00Z"), receipt.SnapshotSHA256, RehaHistoryManagementSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestRehaHistoryV2MissingStaleAndInvalidCountProof(t *testing.T) {
	for _, tc := range []struct {
		name, value, proofValue, validation, at string
		wantError                               bool
	}{
		{"missing proof", "0", "NULL", "unknown", "2026-07-01T09:00:00Z", false},
		{"stale proof", "2", "2", "required_nonnegative_integer_and_collections_v1", "2026-06-01T09:00:00Z", false},
		{"changed value", "2", "1", "required_nonnegative_integer_and_collections_v1", "2026-07-01T09:00:00Z", false},
		{"negative validated", "-1", "-1", "required_nonnegative_integer_and_collections_v1", "2026-07-01T09:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshotPath, receiptPath := createHistorySnapshot(t, func(t *testing.T, db *sql.DB) {
				insertHistorySession(t, db, "session-a", "2026-07-01T08:00:00+02:00", "2026-07-01T09:00:00Z", "10:00 - 10:45")
				if _, err := db.Exec(`UPDATE course_sessions SET participant_count=`+tc.value+`; INSERT INTO reha_session_observations(source,session_id,observed_at,participant_count,validation) VALUES('mysign','session-a',?,`+tc.proofValue+`,?)`, tc.at, tc.validation); err != nil {
					t.Fatal(err)
				}
			})
			receipt, err := ReadSnapshotReceipt(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			db, err := OpenReadOnly(context.Background(), snapshotPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			report, err := db.RehaHistoryWithSchema(context.Background(), receipt.Source, receipt.Location, mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-07-31"), mustUTC(t, "2030-01-01T00:00:00Z"), receipt.SnapshotSHA256, RehaHistoryManagementSchemaVersion)
			if tc.wantError {
				if err == nil {
					t.Fatal("invalid validated value accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Sessions) != 1 || report.Sessions[0].Registered != nil {
				t.Fatalf("unproven value treated as known: %#v", report)
			}
		})
	}
}
