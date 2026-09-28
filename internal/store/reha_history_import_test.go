package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
)

func TestRehaHistoryCountProofSurvivesAccumulatedImports(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for range 2 {
		if _, err := db.ImportMySign(ctx, snapshot, "synthetic"); err != nil {
			t.Fatal(err)
		}
	}
	var validation string
	if err := db.db.QueryRow(`SELECT validation FROM reha_import_observation`).Scan(&validation); err != nil {
		t.Fatal(err)
	}
	if validation != "invalid" {
		t.Fatal("test requires accumulated database")
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM reha_session_observations p JOIN course_sessions s ON s.source=p.source AND s.myyolo_id=p.session_id AND s.updated_at=p.observed_at AND s.participant_count=p.participant_count WHERE p.validation='required_nonnegative_integer_and_collections_v1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("validated rows = %d", count)
	}
	// Reimporting a partial observation preserves proof for the archived session.
	snapshot.CourseSessions = snapshot.CourseSessions[:1]
	snapshot.Attendance = snapshot.Attendance[:2]
	if _, err := db.ImportMySign(ctx, snapshot, "synthetic-partial"); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM reha_session_observations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("historical proof lost: %d", count)
	}
	// Programmatic inputs lacking required-field decoding may not bless zero.
	snapshot.CourseSessions[0] = mysign.CourseSession{ID: "101", DateISO: "2026-07-28", ParticipantCount: 0}
	if _, err := db.ImportMySign(ctx, snapshot, "synthetic-unvalidated"); err != nil {
		t.Fatal(err)
	}
	var value sql.NullInt64
	if err := db.db.QueryRow(`SELECT participant_count,validation FROM reha_session_observations WHERE session_id='101'`).Scan(&value, &validation); err != nil {
		t.Fatal(err)
	}
	if value.Valid || validation != "unknown" {
		t.Fatalf("unvalidated zero received proof: %#v %s", value, validation)
	}
}

func TestRehaHistoryMigrationNeverBlessesLegacyZero(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	insertHistorySession(t, db.db, "legacy", "2026-07-01", "2026-07-01T09:00:00Z", "10:00 - 10:45")
	if _, err := db.db.Exec(`DROP TABLE reha_session_observations`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var proofCount, sessionCount int
	if err := db.db.QueryRow(`SELECT (SELECT COUNT(*) FROM reha_session_observations),(SELECT COUNT(*) FROM course_sessions)`).Scan(&proofCount, &sessionCount); err != nil {
		t.Fatal(err)
	}
	if proofCount != 0 || sessionCount != 1 {
		t.Fatalf("migration changed evidence: proof=%d sessions=%d", proofCount, sessionCount)
	}
}

func TestRehaHistoryV2UsesOneCollectionNotAccumulatedAttendance(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportMySign(ctx, snapshot, "first"); err != nil {
		t.Fatal(err)
	}
	// The second complete wire collection contains only the unsigned and cancelled rows.
	snapshot.Attendance = snapshot.Attendance[1:]
	if _, err := db.ImportMySign(ctx, snapshot, "second"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	report, err := readonly.RehaHistoryWithSchema(ctx, "mysign", "point-gerlingen", mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-07-31"), mustUTC(t, "2030-01-01T00:00:00Z"), "synthetic", RehaHistoryManagementSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	first := report.Sessions[0]
	if first.SignedAttendedNotCancelled != 1 {
		t.Fatal("fixture must include stale accumulated signed row")
	}
	if first.Participated == nil || *first.Participated != 0 || first.AttendanceStatus != "complete_for_observed_snapshot" {
		t.Fatalf("stale row contaminated collection: %#v", first)
	}
}
