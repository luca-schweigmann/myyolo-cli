package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
)

func TestImportIsIdempotent(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for range 2 {
		if _, err := db.ImportMySign(ctx, snapshot, "synthetic-hash"); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := db.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Members != 2 || summary.CourseSessions != 2 || summary.AttendanceRows != 3 {
		t.Fatalf("unexpected idempotent counts: %+v", summary)
	}
	if summary.Attended != 2 || summary.Signed != 1 || summary.Cancelled != 1 {
		t.Fatalf("unexpected attendance flags: %+v", summary)
	}
}

func TestReimportUpdatesAttendanceFlags(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ImportMySign(ctx, snapshot, "before"); err != nil {
		t.Fatal(err)
	}
	snapshot.Attendance[1].Signed = true
	if _, err := db.ImportMySign(ctx, snapshot, "after"); err != nil {
		t.Fatal(err)
	}

	summary, err := db.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Signed != 2 {
		t.Fatalf("signed = %d, want 2", summary.Signed)
	}
}

func TestDatabaseFileIsPrivate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "myyolo.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("database mode = %o, want %o", got, want)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMemberNumbersCanSwapInOneImport(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ImportMySign(ctx, snapshot, "before-swap"); err != nil {
		t.Fatal(err)
	}

	snapshot.Members[0].MemberNumber, snapshot.Members[1].MemberNumber =
		snapshot.Members[1].MemberNumber, snapshot.Members[0].MemberNumber
	if _, err := db.ImportMySign(ctx, snapshot, "after-swap"); err != nil {
		t.Fatalf("member-number swap failed: %v", err)
	}
}

func TestFailedImportIsAudited(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ImportMySign(ctx, snapshot, "initial"); err != nil {
		t.Fatal(err)
	}

	conflict := mysign.Snapshot{
		Members: []mysign.Member{{
			ID:           "999",
			MemberNumber: snapshot.Members[0].MemberNumber,
		}},
	}
	if _, err := db.ImportMySign(ctx, conflict, "conflict"); err == nil {
		t.Fatal("expected member-number conflict")
	}

	var status, message string
	if err := db.db.QueryRow(`
		SELECT status, COALESCE(error_message, '')
		FROM sync_runs
		ORDER BY id DESC LIMIT 1`,
	).Scan(&status, &message); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || message == "" {
		t.Fatalf("failed import audit = status %q message %q", status, message)
	}
}

func TestAggregateReports(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ImportMySign(ctx, snapshot, "reports"); err != nil {
		t.Fatal(err)
	}

	summary, err := db.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.MissingSignatures != 1 || summary.NoShows != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	courses, err := db.Courses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 2 || courses[1].MissingSignatures != 1 {
		t.Fatalf("courses = %+v", courses)
	}
	days, err := db.Days(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Attended != 2 {
		t.Fatalf("days = %+v", days)
	}
	hours, err := db.Hours(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 2 || hours[0].Hour != "09:00" {
		t.Fatalf("hours = %+v", hours)
	}
	sessions, err := db.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].Bookings != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	members, err := db.Members(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].Attended != 1 {
		t.Fatalf("members = %+v", members)
	}
}
