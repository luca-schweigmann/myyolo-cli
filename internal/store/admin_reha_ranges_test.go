package store

import (
	"context"
	"encoding/json"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedRange(t *testing.T, db *Store, kind string, rows []admin.CourseRangeRow, stamp time.Time) {
	t.Helper()
	req, e := readcatalog.Build("course-"+kind, map[string]string{"from": "2026-09-14", "to": "2026-09-16"})
	if e != nil {
		t.Fatal(e)
	}
	p := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-" + kind, Fingerprint: strings.Repeat("a", 64)}, RequestPath: req.Path, RequestBody: req.Body, CourseRangeFacts: &admin.CourseRangeFacts{Kind: kind, From: "2026-09-14", To: "2026-09-16", Completeness: "validated_complete_range", Rows: rows}}
	if _, e := db.ImportAdminPageAt(context.Background(), p, stamp); e != nil {
		t.Fatal(e)
	}
}
func TestAdminRangeClassificationZeroPrivacyAndPeriod(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	db, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	r := func(id, date string, count int) admin.CourseRangeRow {
		return admin.CourseRangeRow{BookingID: id, CourseLabel: "Reha Synthetic", Date: date, StartTime: "11:35", EndTime: "12:20", Count: count}
	}
	seedRange(t, db, "attended", []admin.CourseRangeRow{r("1234567", "2026-09-14", 18), r("1234568", "2026-09-15", 0), r("1234569", "2026-09-16", 0)}, stamp)
	seedRange(t, db, "not-attended", []admin.CourseRangeRow{r("1234567", "2026-09-14", 3), r("1234568", "2026-09-15", 0), r("1234569", "2026-09-16", 10)}, stamp.Add(time.Second))
	seedRange(t, db, "cancelled", []admin.CourseRangeRow{r("1234567", "2026-09-14", 2)}, stamp.Add(2*time.Second))
	db.Close()
	ro, e := OpenReadOnly(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	result, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if len(result.Sessions) != 3 || *result.Sessions[0].Registered != 21 || *result.Sessions[0].Participated != 18 || result.Sessions[0].Cancelled != 2 || *result.Sessions[1].Registered != 0 || *result.Sessions[1].Participated != 0 {
		t.Fatalf("wrong facts %#v", result)
	}
	if result.CompletedSessionCount != 2 || *result.AverageRegistered != 10.5 || *result.AverageParticipated != 9 || result.Sessions[2].PeriodStatus != "provisional" {
		t.Fatal("wrong denominator")
	}
	if result.Sessions[0].SignedParticipated != nil || result.Sessions[0].Capacity != nil || result.ExpectedSessions != nil {
		t.Fatal("invented source proof")
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{`"1234567"`, `"1234568"`, `"1234569"`, `booking_id`} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("raw identity leak")
		}
	}
	if _, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-13"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour)); e == nil {
		t.Fatal("accepted uncovered day")
	}
	if _, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp); e == nil {
		t.Fatal("accepted incomplete collection")
	}
}

func TestAdminRangeMissingClassificationDoesNotBecomeZero(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "incomplete.sqlite")
	db, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	r := admin.CourseRangeRow{BookingID: "777777", CourseLabel: "Synthetic", Date: "2026-09-14", StartTime: "11:35", EndTime: "12:20", Count: 18}
	seedRange(t, db, "attended", []admin.CourseRangeRow{r}, stamp)
	seedRange(t, db, "not-attended", nil, stamp)
	seedRange(t, db, "cancelled", nil, stamp)
	db.Close()
	ro, e := OpenReadOnly(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	report, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if e != nil || len(report.Sessions) != 1 {
		t.Fatalf("partial session lost: %v", e)
	}
	row := report.Sessions[0]
	if row.Registered != nil || row.NotAttended != nil || row.Participated == nil || *row.Participated != 18 || len(row.RegisteredObservedAt) != 0 || row.AttendanceObservedAt == nil {
		t.Fatal("missing classification lost its unknown value or proved attendance")
	}
	if report.AverageRegistered != nil || report.AverageParticipated == nil || *report.AverageParticipated != 18 {
		t.Fatal("unknown counted in average")
	}
	if row.DataGap == nil || row.DataGap.Kind != "data_gap" || row.DataGap.Code != "not_attended_list_missing" ||
		len(row.DataGap.Missing) != 1 || row.DataGap.Missing[0] != "not-attended" || row.DataGap.ActionDE == "" {
		t.Fatalf("held session with known attendance must be a real data gap: %+v", row.DataGap)
	}
	if report.IncompleteSessions != 1 || report.ExcludedSessions["not_held"] != 0 || report.ExcludedSessions["not_yet_held"] != 0 {
		t.Fatalf("gap summary wrong: %d %v", report.IncompleteSessions, report.ExcludedSessions)
	}
}

func TestAdminRangeFutureSessionIsNotYetHeld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	r := admin.CourseRangeRow{BookingID: "888888", CourseLabel: "Synthetic", Date: "2026-09-16", StartTime: "15:05", EndTime: "15:50", Count: 2}
	seedRange(t, db, "attended", nil, stamp)
	seedRange(t, db, "not-attended", []admin.CourseRangeRow{r}, stamp)
	seedRange(t, db, "cancelled", nil, stamp)
	db.Close()
	ro, e := OpenReadOnly(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	report, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if e != nil || len(report.Sessions) != 1 {
		t.Fatalf("session lost: %v", e)
	}
	gap := report.Sessions[0].DataGap
	if gap == nil || gap.Kind != "not_yet_held" || gap.Code != "session_not_yet_held" {
		t.Fatalf("today's session must be not_yet_held: %+v", gap)
	}
	if report.IncompleteSessions != 0 || report.ExcludedSessions["not_yet_held"] != 1 {
		t.Fatalf("summary wrong: %d %v", report.IncompleteSessions, report.ExcludedSessions)
	}
}

func TestAdminRangeAddsOnlyExactProvenPlannerZero(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zero.sqlite")
	db, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	for _, kind := range []string{"attended", "not-attended", "cancelled"} {
		seedRange(t, db, kind, []admin.CourseRangeRow{}, stamp)
	}
	p := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-planner-week", Fingerprint: strings.Repeat("a", 64)}, RequestPath: "/Kursplaner_WEB/Wochenplaner_Tabelle_liste.asp?Datum=14.09.2026&Raum=", CoursePlannerFacts: &admin.CoursePlannerFacts{From: "2026-09-14", To: "2026-09-20", Completeness: "validated_week_inventory", Rows: []admin.CoursePlannerRow{{CourseID: "98765", Planner: "A", Date: "2026-09-15", CourseLabel: "Synthetic zero", StartTime: "14:05", EndTime: "14:50", Capacity: 20, Registered: 0, Free: 20}}}}
	if _, e := db.ImportAdminPageAt(ctx, p, stamp); e != nil {
		t.Fatal(e)
	}
	detail := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-session", Fingerprint: strings.Repeat("b", 64)}, RequestPath: "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=98765&Datum=15.09.2026&defaultMode=A", CourseSessionIdentity: &admin.CourseSessionIdentity{BookingID: "7654321", Date: "2026-09-15"}, CourseSessionFacts: &admin.CourseSessionFacts{Completeness: "validated_observed_roster", CancellationStatus: "unknown_not_provided"}}
	if _, e := db.ImportAdminPageAt(ctx, detail, stamp); e != nil {
		t.Fatal(e)
	}
	db.Close()
	ro, e := OpenReadOnly(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	report, e := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if len(report.Sessions) != 1 || *report.Sessions[0].Registered != 0 || *report.Sessions[0].Participated != 0 || *report.Sessions[0].Capacity != 20 || report.ExpectedSessions == nil || *report.ExpectedSessions != 1 || *report.AverageRegistered != 0 || report.Sessions[0].StableCourseID == nil {
		t.Fatalf("invalid proven zero %#v", report)
	}
}

func TestAdminRangeOneMissingAttendanceKeepsOtherSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "partial.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	row := func(id, day string, count int) admin.CourseRangeRow {
		return admin.CourseRangeRow{BookingID: id, CourseLabel: "Synthetic", Date: day, StartTime: "10:00", EndTime: "10:45", Count: count}
	}
	seedRange(t, db, "attended", []admin.CourseRangeRow{row("100001", "2026-09-14", 10), row("100003", "2026-09-16", 0)}, stamp)
	seedRange(t, db, "not-attended", []admin.CourseRangeRow{row("100001", "2026-09-14", 5), row("100002", "2026-09-15", 18), row("100003", "2026-09-16", 0)}, stamp)
	seedRange(t, db, "cancelled", nil, stamp)
	db.Close()
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	report, err := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), mustUTC(t, "2026-09-17T09:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 3 {
		t.Fatal("other sessions lost")
	}
	missing := report.Sessions[1]
	if missing.Registered != nil || missing.Participated != nil || missing.AttendanceObservedAt != nil || len(missing.RegisteredObservedAt) != 0 || missing.NotAttended == nil || *missing.NotAttended != 18 {
		t.Fatal("missing value replaced or evidence lost")
	}
	if *report.AverageRegistered != 7.5 || *report.AverageParticipated != 5 {
		t.Fatal("unknown entered denominator or true zero excluded")
	}
	raw, _ := json.Marshal(missing)
	if !strings.Contains(string(raw), `"participated":null`) {
		t.Fatal("unknown not serialized as null")
	}
	if missing.DataGap == nil || missing.DataGap.Kind != "not_held" || missing.DataGap.Code != "no_attendee_marked" {
		t.Fatalf("past session without any attendee must count as not held: %+v", missing.DataGap)
	}
	for _, i := range []int{0, 2} {
		if report.Sessions[i].DataGap != nil {
			t.Fatal("complete session got a data gap")
		}
	}
	if report.IncompleteSessions != 0 || report.ExcludedSessions["not_held"] != 1 {
		t.Fatalf("summary wrong: %d %v", report.IncompleteSessions, report.ExcludedSessions)
	}
	if !strings.Contains(string(raw), `"data_gap":{"code":"no_attendee_marked","kind":"not_held"`) {
		t.Fatalf("data gap not serialized: %s", raw)
	}
}

func TestRangeDataGapClassification(t *testing.T) {
	for _, tc := range []struct {
		name                                           string
		past, attendee, hasAttended, hasNotAtt, signed bool
		code, kind                                     string
	}{
		{"future", false, false, false, true, false, "session_not_yet_held", "not_yet_held"},
		{"no attended row", true, false, false, true, false, "no_attendee_marked", "not_held"},
		{"zero attended", true, false, true, false, false, "no_attendee_marked", "not_held"},
		{"only cancellations", true, false, false, false, false, "no_attendee_marked", "not_held"},
		{"signature vetoes not held", true, false, false, true, true, "signed_but_not_marked_attended", "data_gap"},
		{"attendees but no not-attended list", true, true, true, false, false, "not_attended_list_missing", "data_gap"},
	} {
		gap := rangeDataGap(tc.past, tc.attendee, tc.hasAttended, tc.hasNotAtt, tc.signed)
		if gap.Code != tc.code || gap.Kind != tc.kind || gap.ActionDE == "" {
			t.Errorf("%s: got %s/%s", tc.name, gap.Code, gap.Kind)
		}
	}
}

func TestAdminRangeMarkedAttendanceInDetailVetoesNotHeld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "marked.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := mustUTC(t, "2026-09-16T09:00:00Z")
	row := admin.CourseRangeRow{BookingID: "200001", CourseLabel: "Synthetic", Date: "2026-09-15", StartTime: "10:00", EndTime: "10:45", Count: 4}
	seedRange(t, db, "attended", nil, stamp)
	seedRange(t, db, "not-attended", []admin.CourseRangeRow{row}, stamp)
	seedRange(t, db, "cancelled", nil, stamp)
	detail := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-session", Fingerprint: strings.Repeat("c", 64)}, RequestPath: "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=4242&Datum=15.09.2026", CourseSessionIdentity: &admin.CourseSessionIdentity{BookingID: "200001", Date: "2026-09-15"}, CourseSessionFacts: &admin.CourseSessionFacts{Registered: 6, AttendanceMarked: 2, SignedAttendance: 0, Completeness: "validated_observed_roster", CancellationStatus: "unknown_not_provided"}}
	if _, err := db.ImportAdminPageAt(ctx, detail, stamp); err != nil {
		t.Fatal(err)
	}
	db.Close()
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	report, err := ro.AdminRehaRanges(ctx, mustBerlinDate(t, "2026-09-14"), mustBerlinDate(t, "2026-09-16"), stamp.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 1 || report.Sessions[0].DataGap == nil || report.Sessions[0].DataGap.Kind != "data_gap" || report.IncompleteSessions != 1 || report.ExcludedSessions["not_held"] != 0 {
		t.Fatalf("marked attendance in detail must keep the session incomplete: %+v %d %v", report.Sessions, report.IncompleteSessions, report.ExcludedSessions)
	}
}
