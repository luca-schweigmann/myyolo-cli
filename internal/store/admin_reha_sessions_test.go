package store

import (
	"context"
	"encoding/json"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminRehaSessionFactsNativeIdentityLatestAndPrivacy(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "admin.sqlite")
	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	page := admin.Page{CourseSessionIdentity: &admin.CourseSessionIdentity{BookingID: "321001", Date: "2026-09-14"}, Metadata: admin.PageMetadata{Route: "capability:course-session", Fingerprint: strings.Repeat("a", 64)}, RequestPath: "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=321&Datum=14.09.2026&defaultMode=A", CourseSessionFacts: &admin.CourseSessionFacts{Registered: 21, AttendanceMarked: 18, SignedAttendance: 18, Completeness: "validated_observed_roster", CancellationStatus: "unknown_not_provided"}, Tables: []admin.Table{{Rows: [][]string{{"Private Synthetic Name", "member-secret-123"}}}}}
	stamp, _ := time.Parse(time.RFC3339, "2026-09-16T09:00:00Z")
	if _, err := db.ImportAdminPageAt(ctx, page, stamp); err != nil {
		t.Fatal(err)
	}
	page.CourseSessionFacts.Registered = 20
	page.CourseSessionFacts.AttendanceMarked = 17
	page.CourseSessionFacts.SignedAttendance = 17
	if _, err := db.ImportAdminPageAt(ctx, page, stamp.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	report, err := readonly.AdminRehaSessions(ctx, mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 1 || report.Sessions[0].Registered != 20 || report.Sessions[0].SignedAttendance != 17 || (report.Sessions[0].Participated == nil || *report.Sessions[0].Participated != 17) {
		t.Fatalf("facts=%#v", report)
	}
	row := report.Sessions[0]
	if row.StableCourseID != adminRehaPseudonym("course", "321") || row.StableSessionID == stableSessionID("mysign", "321") || row.Planner == nil || *row.Planner != "A" {
		t.Fatal("native identity was not preserved")
	}
	raw, _ := json.Marshal(report)
	for _, secret := range []string{"Private Synthetic Name", "member-secret-123", `"321"`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("privacy leak %q", secret)
		}
	}
	older, err := readonly.AdminRehaSessions(ctx, mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-09-16"), stamp)
	if err != nil {
		t.Fatal(err)
	}
	if older.Sessions[0].Registered != 21 {
		t.Fatal("archive did not preserve exact as-of observation")
	}
}
func TestAdminRehaSessionFactsRejectsLegacyUntypedArchive(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "admin.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	page := admin.Page{CourseSessionIdentity: &admin.CourseSessionIdentity{BookingID: "321001", Date: "2026-09-14"}, Metadata: admin.PageMetadata{Route: "capability:course-session", Fingerprint: strings.Repeat("b", 64)}, RequestPath: "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=321&Datum=14.09.2026"}
	if _, err := db.ImportAdminPageAt(ctx, page, mustUTC(t, "2026-09-16T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	db.Close()
	readonly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if _, err := readonly.AdminRehaSessions(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-14"), mustUTC(t, "2030-01-01T00:00:00Z")); err == nil {
		t.Fatal("untyped legacy archive counted as zero")
	}
}
