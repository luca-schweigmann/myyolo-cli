package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRehaInactivityMarksBucketsUnavailableWithoutCompletePopulation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	snapshotPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, snapshotPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(ctx, snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	report, err := ro.RehaInactivity(ctx, receipt.Source, receipt.Location, mustUTC(t, "2026-09-06T12:00:00Z"), receipt.SnapshotSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if report.CapabilityStatus != "unavailable" || report.CoverageStatus != "observed_rows_only" || report.HistoryStatus != "incomplete" {
		t.Fatalf("capability status = %#v", report)
	}
	if report.CurrentAssignmentStatus != "unavailable" || report.MemberStatus != "unavailable" {
		t.Fatalf("population status = %#v", report)
	}
	for name, bucket := range map[string]RehaInactivityBucket{
		"over_28_days":    report.Over28Days,
		"over_3_months":   report.Over3Months,
		"unknown_history": report.UnknownHistory,
	} {
		if bucket.Status != "unavailable" || bucket.Count != nil {
			t.Fatalf("%s bucket = %#v", name, bucket)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	assertAggregateJSONOmitsPrivateData(t, encoded)
}

func assertAggregateJSONOmitsPrivateData(t *testing.T, encoded []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode aggregate JSON: %v", err)
	}
	privateFields := map[string]struct{}{
		"description": {}, "first_name": {}, "last_name": {}, "member_id": {},
		"member_number": {}, "myyolo_id": {}, "room_name": {},
	}
	privateValues := map[string]struct{}{
		"Beispiel": {}, "Muster": {}, "501": {}, "1001": {},
		"Reha Rücken": {}, "Kursraum 1": {},
	}
	var inspect func(any)
	inspect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if _, forbidden := privateFields[key]; forbidden {
					t.Errorf("aggregate JSON exposes private field %q", key)
				}
				inspect(nested)
			}
		case []any:
			for _, nested := range typed {
				inspect(nested)
			}
		case string:
			if _, forbidden := privateValues[typed]; forbidden {
				t.Errorf("aggregate JSON exposes private scalar %q", typed)
			}
		}
	}
	inspect(document)
}

func TestRehaInactivityDoesNotClassifyAtFourWeekOrThreeMonthBoundaries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	prepareRehaSource(t, sourcePath)
	scopePath := filepath.Join(dir, "scope.json")
	writeRehaScope(t, scopePath, sourcePath, "point-gerlingen")
	snapshotPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := CreateSnapshot(ctx, sourcePath, snapshotPath, scopePath, receiptPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(ctx, snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	for _, asOf := range []string{"2026-08-25T09:00:00+02:00", "2026-10-29T09:00:00+01:00"} {
		report, err := ro.RehaInactivity(ctx, receipt.Source, receipt.Location, mustUTC(t, asOf), receipt.SnapshotSHA256)
		if err != nil {
			t.Fatal(err)
		}
		if report.Over28Days.Count != nil || report.Over3Months.Count != nil || report.UnknownHistory.Count != nil {
			t.Fatalf("as-of %s unexpectedly classified incomplete history: %#v", asOf, report)
		}
	}
}

func TestRehaInactivityRequiresValidatedObservationAndStaysReadOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	prepareRehaSource(t, path)
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, err := ro.RehaInactivity(ctx, "mysign", "point-gerlingen", mustUTC(t, "2026-09-06T12:00:00Z"), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := ro.db.ExecContext(ctx, `UPDATE members SET first_name='changed'`); err == nil {
		t.Fatal("read-only inactivity store allowed a write")
	}
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Fatal("read-only inactivity report created a WAL sidecar")
	}

	legacyPath := filepath.Join(dir, "legacy.sqlite")
	legacy, err := Open(ctx, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	legacyRO, err := OpenReadOnly(ctx, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer legacyRO.Close()
	if _, err := legacyRO.RehaInactivity(ctx, "mysign", "point-gerlingen", mustUTC(t, "2026-09-06T12:00:00Z"), strings.Repeat("a", 64)); err == nil || !strings.Contains(err.Error(), "fresh single validated import") {
		t.Fatalf("legacy database error = %v", err)
	}
}

func TestValidateRehaInactivityThresholds(t *testing.T) {
	thresholds, err := ComputeRehaInactivityThresholds(mustUTC(t, "2026-09-06T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := thresholds.Over28DaysBefore.Format(time.RFC3339), "2026-08-09T14:00:00+02:00"; got != want {
		t.Fatalf("strict 28-day cutoff = %q, want %q", got, want)
	}
	if got, want := thresholds.Over3MonthsBefore.Format(time.RFC3339), "2026-06-06T14:00:00+02:00"; got != want {
		t.Fatalf("calendar three-month cutoff = %q, want %q", got, want)
	}
	dstThresholds, err := ComputeRehaInactivityThresholds(mustUTC(t, "2026-10-25T11:00:00+01:00"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dstThresholds.Over28DaysBefore.Format(time.RFC3339), "2026-09-27T11:00:00+02:00"; got != want {
		t.Fatalf("calendar 28-day DST cutoff = %q, want %q", got, want)
	}
	if _, err := ComputeRehaInactivityThresholds(time.Time{}); err == nil {
		t.Fatal("zero as-of unexpectedly accepted")
	}
}
