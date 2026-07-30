package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
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

func TestDatabaseStatusIsAggregateAndHealthy(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	status, err := db.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != schemaVersion ||
		status.Integrity != "ok" ||
		status.Path == "" {
		t.Fatalf("status = %#v", status)
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

func TestNoShowBeginsOnlyAfterSessionEnd(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.CourseSessions = append(snapshot.CourseSessions, mysign.CourseSession{
		ID:            "103",
		DateISO:       "2026-07-30T10:00:00+02:00",
		DateFormatted: "30.07.2026",
		Description:   "Reha Zukunft",
		TimeFormatted: "10:00 - 11:00",
	})
	snapshot.Attendance = append(snapshot.Attendance, mysign.Attendance{
		ID:              "1004",
		MemberID:        "501",
		CourseSessionID: "103",
		Date:            "2026-07-30",
		TimeFormatted:   "10:00 - 11:00",
		Course:          "Reha Zukunft",
	})

	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ImportMySign(ctx, snapshot, "time-boundary"); err != nil {
		t.Fatal(err)
	}

	during := time.Date(2026, 7, 30, 8, 30, 0, 0, time.UTC)
	summary, err := db.SummaryAt(ctx, during)
	if err != nil {
		t.Fatal(err)
	}
	if summary.NoShows != 0 || summary.Pending != 1 {
		t.Fatalf("during session summary = %+v", summary)
	}

	atEnd := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	summary, err = db.SummaryAt(ctx, atEnd)
	if err != nil {
		t.Fatal(err)
	}
	if summary.NoShows != 0 || summary.Pending != 1 {
		t.Fatalf("at session end summary = %+v", summary)
	}

	after := atEnd.Add(time.Second)
	summary, err = db.SummaryAt(ctx, after)
	if err != nil {
		t.Fatal(err)
	}
	if summary.NoShows != 1 || summary.Pending != 0 {
		t.Fatalf("after session summary = %+v", summary)
	}

	sessions, err := db.SessionsAt(ctx, after)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions[len(sessions)-1].NoShows; got != 1 {
		t.Fatalf("future session no-shows = %d, want 1 after end", got)
	}
}

func TestUnknownSessionEndRemainsPending(t *testing.T) {
	ctx := context.Background()
	snapshot := mysign.Snapshot{
		Members: []mysign.Member{{ID: "member-1"}},
		CourseSessions: []mysign.CourseSession{{
			ID:            "session-1",
			DateISO:       "unknown",
			TimeFormatted: "unknown",
		}},
		Attendance: []mysign.Attendance{{
			ID:              "attendance-1",
			MemberID:        "member-1",
			CourseSessionID: "session-1",
		}},
	}
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ImportMySign(ctx, snapshot, "unknown-end"); err != nil {
		t.Fatal(err)
	}
	summary, err := db.SummaryAt(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.NoShows != 0 || summary.Pending != 1 {
		t.Fatalf("unknown end summary = %+v", summary)
	}
}

func TestSessionEndUTCUsesSourceOffsetAndSupportsOvernight(t *testing.T) {
	session := mysign.CourseSession{
		DateISO:       "2026-10-25T23:30:00+01:00",
		TimeFormatted: "23:30 – 00:15",
	}
	if got, want := sessionEndUTC(session), "2026-10-25T23:15:00Z"; got != want {
		t.Fatalf("sessionEndUTC() = %q, want %q", got, want)
	}
}

func TestMigrationAddsSourceAndSessionEndToV2Database(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.sqlite")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE members (
			myyolo_id TEXT PRIMARY KEY,
			member_number TEXT,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX members_member_number_unique
		 ON members(member_number)
		 WHERE member_number IS NOT NULL AND member_number <> ''`,
		`CREATE TABLE course_sessions (
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
		`INSERT INTO members(
			myyolo_id, member_number, first_name, last_name, updated_at
		) VALUES('member-1', '7', '', '', '2026-07-30T00:00:00Z')`,
		`INSERT INTO course_sessions(
			myyolo_id, date_iso, date_formatted, description, room_name,
			time_formatted, participant_count, current_week,
			signature_available_iso, updated_at
		) VALUES(
			'session-1', '2026-07-30T10:00:00+02:00', '30.07.2026',
			'', '', '10:00 - 11:00', 0, 0, '', '2026-07-30T00:00:00Z'
		)`,
		`PRAGMA user_version = 2`,
	}
	for _, statement := range statements {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			_ = raw.Close()
			t.Fatal(err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var source, endsAt string
	if err := db.db.QueryRowContext(ctx, `
		SELECT source, ends_at_utc
		FROM course_sessions
		WHERE myyolo_id='session-1'`,
	).Scan(&source, &endsAt); err != nil {
		t.Fatal(err)
	}
	if source != "mysign" || endsAt != "2026-07-30T09:00:00Z" {
		t.Fatalf("source = %q, ends_at_utc = %q", source, endsAt)
	}
	var indexCount int
	if err := db.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND name='members_source_number_unique'`,
	).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("source-scoped index count = %d, want 1", indexCount)
	}
}

func TestAdminPageImportIsIdempotentAndSeparateFromMySign(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	snapshot := mysign.Snapshot{
		Members: []mysign.Member{{ID: "same-raw-id", MemberNumber: "7"}},
	}
	if _, err := db.ImportMySign(ctx, snapshot, "mysign-hash"); err != nil {
		t.Fatal(err)
	}
	page := admin.Page{
		Metadata: admin.PageMetadata{
			Route:        "/start_Anwesenheit.asp",
			Title:        "Attendance",
			TableHeaders: [][]string{{"Remote ID", "Present"}},
			TableRows:    []int{1},
			Fingerprint:  strings.Repeat("a", 64),
		},
		Tables: []admin.Table{{
			Headers: []string{"Remote ID", "Present"},
			Rows:    [][]string{{"same-raw-id", "yes"}},
		}},
	}
	firstObserved := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	secondObserved := firstObserved.Add(time.Hour)
	for _, observedAt := range []time.Time{firstObserved, secondObserved} {
		if _, err := db.ImportAdminPageAt(ctx, page, observedAt); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := db.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.LastCompletedSync == "" {
		t.Fatal("admin imports replaced the last mySIGN sync timestamp")
	}

	capabilities, err := db.AdminCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 1 ||
		capabilities[0].RecordCount != 1 ||
		capabilities[0].TableCount != 1 {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	records, err := db.AdminRecords(ctx, "/start_Anwesenheit.asp")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 ||
		!strings.Contains(records[0].ValuesJSON, "same-raw-id") ||
		records[0].FirstObservedAt == records[0].LastObservedAt {
		t.Fatalf("records = %#v", records)
	}
	var mySignMembers int
	if err := db.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM members WHERE source='mysign' AND myyolo_id='same-raw-id'`,
	).Scan(&mySignMembers); err != nil {
		t.Fatal(err)
	}
	if mySignMembers != 1 {
		t.Fatalf("mySIGN member count = %d, want 1", mySignMembers)
	}
}

func TestCapabilityImportAndCurrentRecords(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	route := "capability:attendance-monthly"
	firstObserved := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	secondObserved := firstObserved.Add(time.Hour)
	page := admin.Page{
		Metadata: admin.PageMetadata{
			Route:        route,
			Title:        "Monthly attendance",
			TableHeaders: [][]string{{"Month", "Attendance"}},
			TableRows:    []int{2},
			Fingerprint:  strings.Repeat("b", 64),
		},
		Tables: []admin.Table{{
			Headers: []string{"Month", "Attendance"},
			Rows: [][]string{
				{"June", "4"},
				{"July", "7"},
			},
		}},
	}
	if _, err := db.ImportAdminPageAt(ctx, page, firstObserved); err != nil {
		t.Fatal(err)
	}
	page.Tables[0].Rows = [][]string{{"July", "8"}}
	page.Metadata.TableRows = []int{1}
	if _, err := db.ImportAdminPageAt(ctx, page, secondObserved); err != nil {
		t.Fatal(err)
	}

	all, err := db.AdminRecords(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("historical records = %d, want 3", len(all))
	}
	current, err := db.CurrentAdminRecords(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || !strings.Contains(current[0].ValuesJSON, `"8"`) {
		t.Fatalf("current records = %#v", current)
	}
}

func TestCapabilityImportRejectsUnknownRouteKey(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.ImportAdminPage(ctx, admin.Page{
		Metadata: admin.PageMetadata{
			Route:       "capability:not-in-the-catalog",
			Fingerprint: strings.Repeat("c", 64),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "known capability key") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdminDomainReportsUseLatestStructuredSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "myyolo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	observedAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pages := []admin.Page{
		{
			Metadata: admin.PageMetadata{
				Route:        "/Anwesenheit/Reha_Anwesend_Datum_Liste.asp",
				TableHeaders: [][]string{{"", "Nr.:", "Name", "Vorname", "Uhrzeit", ""}},
				TableRows:    []int{2},
				Fingerprint:  strings.Repeat("b", 64),
			},
			Tables: []admin.Table{{
				Headers: []string{"", "Nr.:", "Name", "Vorname", "Uhrzeit", ""},
				Rows: [][]string{
					{"", "7", "Example", "Ada", "09:05 09:50", ""},
					{"", "8", "Sample", "Ben", "09:05 09:50", ""},
				},
			}},
		},
		{
			Metadata: admin.PageMetadata{
				Route:        "/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp",
				TableHeaders: [][]string{{"", "Nr.:", "Vorname", "Name", "Menge", ""}},
				TableRows:    []int{2},
				Fingerprint:  strings.Repeat("c", 64),
			},
			Tables: []admin.Table{{
				Headers: []string{"", "Nr.:", "Vorname", "Name", "Menge", ""},
				Rows: [][]string{
					{"", "7", "Ada", "Example", "2", ""},
					{"", "8", "Ben", "Sample", "1", ""},
				},
			}},
		},
		{
			Metadata: admin.PageMetadata{
				Route: "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp",
				TableHeaders: [][]string{{
					"",
					"Name",
					"Angebotsnummer",
					"6/2026",
					"7/2026",
					"",
				}},
				TableRows:   []int{2},
				Fingerprint: strings.Repeat("d", 64),
			},
			Tables: []admin.Table{{
				Headers: []string{"", "Name", "Angebotsnummer", "6/2026", "7/2026", ""},
				Rows: [][]string{
					{"", "Course A", "A-1", "3", "4", ""},
					{"", "Course B", "B-1", "2", "0", ""},
				},
			}},
		},
	}
	for _, page := range pages {
		if _, err := db.ImportAdminPageAt(ctx, page, observedAt); err != nil {
			t.Fatal(err)
		}
	}

	hours, err := db.AdminRehaHours(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 1 || hours[0].Attendees != 2 ||
		hours[0].TimeWindow != "09:05 09:50" ||
		hours[0].DurationMinutes != 45 ||
		hours[0].AttendeeMinutes != 90 {
		t.Fatalf("hours = %#v", hours)
	}
	attendanceDetails, err := db.AdminRehaAttendanceDetails(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(attendanceDetails) != 2 || attendanceDetails[0].MemberNumber == "" {
		t.Fatalf("attendance details = %#v", attendanceDetails)
	}
	missing, err := db.AdminMissingSignatures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if missing.MembersWithMissing != 2 || missing.MissingTotal != 3 {
		t.Fatalf("missing signatures = %#v", missing)
	}
	missingDetails, err := db.AdminMissingSignatureDetails(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(missingDetails) != 2 || missingDetails[0].Missing == 0 {
		t.Fatalf("missing details = %#v", missingDetails)
	}
	months, err := db.AdminCourseMonths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 ||
		months[0].Month != "6/2026" ||
		months[0].AttendanceTotal != 5 ||
		months[1].AttendanceTotal != 4 {
		t.Fatalf("months = %#v", months)
	}
}
