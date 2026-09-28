package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminRehaSessionsCLIExportsNativeManagementFacts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "admin.sqlite")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	page := admin.Page{CourseSessionIdentity: &admin.CourseSessionIdentity{BookingID: "321001", Date: "2026-09-14"}, Metadata: admin.PageMetadata{Route: "capability:course-session", Fingerprint: strings.Repeat("a", 64)}, RequestPath: "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=321&Datum=14.09.2026", CourseSessionFacts: &admin.CourseSessionFacts{Registered: 21, AttendanceMarked: 18, SignedAttendance: 18, Completeness: "validated_observed_roster", CancellationStatus: "unknown_not_provided"}}
	stamp, _ := time.Parse(time.RFC3339, "2026-09-16T07:00:00Z")
	if _, err := db.ImportAdminPageAt(ctx, page, stamp); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var output bytes.Buffer
	if err := run(ctx, []string{"report", "admin-reha-sessions", "--db", path, "--from", "2026-09-14", "--to", "2026-09-14", "--as-of", "2026-09-16T08:00:00Z"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	var report store.AdminRehaSessionReport
	if json.Unmarshal(output.Bytes(), &report) != nil {
		t.Fatal("invalid JSON")
	}
	if len(report.Sessions) != 1 || report.Sessions[0].Participated == nil || *report.Sessions[0].Participated != 18 || report.Sessions[0].Registered != 21 || report.Sessions[0].Cancelled != nil || report.Sessions[0].PeriodStatus != "past_local_day" {
		t.Fatalf("report=%#v", report)
	}
	if strings.Contains(output.String(), `"321"`) {
		t.Fatal("native raw ID leaked")
	}
}

func TestAdminRehaRangesCLIAndMissingCoverage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ranges.sqlite")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stamp, _ := time.Parse(time.RFC3339, "2026-09-16T07:00:00Z")
	for _, kind := range []string{"attended", "not-attended", "cancelled"} {
		req, err := readcatalog.Build("course-"+kind, map[string]string{"from": "2026-09-14", "to": "2026-09-14"})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		if kind == "attended" {
			count = 18
		}
		if kind == "not-attended" {
			count = 3
		}
		page := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-" + kind, Fingerprint: strings.Repeat("a", 64)}, RequestPath: req.Path, RequestBody: req.Body, CourseRangeFacts: &admin.CourseRangeFacts{Kind: kind, From: "2026-09-14", To: "2026-09-14", Completeness: "validated_complete_range", Rows: []admin.CourseRangeRow{{BookingID: "synthetic-private-native-id", CourseLabel: "Reha Synthetic", Date: "2026-09-14", StartTime: "11:35", EndTime: "12:20", Count: count}}}}
		if _, err := db.ImportAdminPageAt(ctx, page, stamp); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	var output bytes.Buffer
	args := []string{"report", "admin-reha-ranges", "--db", path, "--from", "2026-09-14", "--to", "2026-09-14", "--as-of", "2026-09-16T08:00:00Z"}
	if err := run(ctx, args, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	var report store.AdminRehaRangeReport
	if json.Unmarshal(output.Bytes(), &report) != nil || len(report.Sessions) != 1 || *report.Sessions[0].Registered != 21 || *report.Sessions[0].Participated != 18 || *report.AverageRegistered != 21 {
		t.Fatal("wrong chart output")
	}
	if strings.Contains(output.String(), "synthetic-private-native-id") {
		t.Fatal("raw ID leak")
	}
	args[6] = "2026-09-13"
	output.Reset()
	if err := run(ctx, args, strings.NewReader(""), &output); err == nil || output.Len() != 0 {
		t.Fatal("missing date became empty chart")
	}
}
