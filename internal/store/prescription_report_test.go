package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
)

func TestReadOnlyPrescriptionMetric(t *testing.T) {
	page := admin.Page{
		Metadata: admin.PageMetadata{
			Route: "capability:reha-prescriptions",
			TableHeaders: [][]string{
				{"Layout"},
				{"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", ""},
			},
			TableRows:   []int{2, 3},
			Fingerprint: strings.Repeat("a", 64),
		},
		Tables: []admin.Table{
			{
				Headers: []string{"Layout"},
				Rows:    [][]string{{"same"}, {"same"}},
			},
			{
				Headers: []string{"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", ""},
				Rows: [][]string{
					{"vo-1", "member-1", "Muster", "Eins", "50", "4", "46", "yes", ""},
					{"vo-2", "member-2", "Muster", "Zwei", "50", "3", "47", "yes", ""},
					{"footer"},
				},
			},
		},
	}
	path := importPrescriptionPage(t, page)
	db, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.PrescriptionMetric(context.Background(), "reha-prescriptions")
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || got.SkippedRows != 1 || got.ObservedAt == "" ||
		got.SchemaFingerprint != strings.Repeat("a", 64) {
		t.Fatalf("unexpected report: %+v", got)
	}
}

func TestReadOnlyPrescriptionMetricRejectsMixedSameTimestampSnapshot(t *testing.T) {
	observedAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	first := prescriptionSummaryPage([][]string{
		{"", "Praxis A", "2"},
		{"", "Praxis B", "3"},
	})
	second := prescriptionSummaryPage([][]string{
		{"", "Praxis C", "4"},
		{"", "Praxis D", "5"},
	})

	path := filepath.Join(t.TempDir(), "prescriptions.sqlite")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportAdminPageAt(context.Background(), first, observedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportAdminPageAt(context.Background(), second, observedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	_, err = ro.PrescriptionMetric(context.Background(), "reha-prescription-summary")
	if err == nil || !strings.Contains(err.Error(), "cannot be reconstructed completely") {
		t.Fatalf("expected mixed snapshot rejection, got %v", err)
	}
}

func TestReadOnlyPrescriptionMetricRejectsDeduplicatedIdenticalRows(t *testing.T) {
	page := prescriptionSummaryPage([][]string{
		{"", "Praxis A", "2"},
		{"", "Praxis A", "2"},
		{"", "Praxis B", "3"},
	})
	path := importPrescriptionPage(t, page)
	db, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.PrescriptionMetric(context.Background(), "reha-prescription-summary")
	if err == nil || !strings.Contains(err.Error(), "cannot be reconstructed completely") {
		t.Fatalf("expected deduplicated row rejection, got %v", err)
	}
}

func prescriptionSummaryPage(rows [][]string) admin.Page {
	return admin.Page{
		Metadata: admin.PageMetadata{
			Route:        "capability:reha-prescription-summary",
			TableHeaders: [][]string{{"", "Arzt", "Menge"}},
			TableRows:    []int{len(rows)},
			Fingerprint:  strings.Repeat("d", 64),
		},
		Tables: []admin.Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows:    rows,
		}},
	}
}

func TestPrescriptionContextBindsDatabaseAndObservation(t *testing.T) {
	page := admin.Page{
		Metadata: admin.PageMetadata{
			Route:        "capability:reha-prescription-summary",
			TableHeaders: [][]string{{"", "Arzt", "Menge"}},
			TableRows:    []int{2},
			Fingerprint:  strings.Repeat("b", 64),
		},
		Tables: []admin.Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows: [][]string{
				{"", "Praxis A", "2"},
				{"", "Praxis B", "3"},
			},
		}},
	}
	path := importPrescriptionPage(t, page)
	db, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	metric, err := db.PrescriptionMetric(context.Background(), "reha-prescription-summary")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	hash, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	receipt := PrescriptionContextReceipt{
		Version:        1,
		Capability:     "reha-prescription-summary",
		From:           "2026-08-01",
		To:             "2026-08-31",
		DatabaseSHA256: hash,
		ObservedAt:     metric.ObservedAt,
		CapturedAt:     time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		BindingBasis:   "operator_captured_hash_bound_context_v1",
	}
	if err := validatePrescriptionContextReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrescriptionContext(path, receipt, metric); err != nil {
		t.Fatal(err)
	}
	receipt.DatabaseSHA256 = strings.Repeat("0", 64)
	if err := ValidatePrescriptionContext(path, receipt, metric); err == nil ||
		!strings.Contains(err.Error(), "database hash") {
		t.Fatalf("expected database mismatch, got %v", err)
	}
}

func importPrescriptionPage(t *testing.T, page admin.Page) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prescriptions.sqlite")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportAdminPage(context.Background(), page); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
