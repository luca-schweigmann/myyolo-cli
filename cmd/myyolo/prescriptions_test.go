package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func TestPrescriptionMetricCLIUsesHashBoundContext(t *testing.T) {
	dbPath, receiptPath := prescriptionSummaryFixture(t, false)
	var stdout bytes.Buffer
	err := run(
		context.Background(),
		[]string{
			"report", "prescription-metric",
			"--db", dbPath,
			"--capability", "reha-prescription-summary",
			"--context-file", receiptPath,
			"--format", "json",
		},
		strings.NewReader(""),
		&stdout,
	)
	if err != nil {
		t.Fatal(err)
	}
	var got prescriptionMetricOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 5 || got.ContextFrom != "2026-08-01" ||
		got.ContextTo != "2026-08-31" ||
		got.SourceScopeStatus != "external_operator_scope_required" ||
		got.RequestContextBinding != "operator_captured_hash_bound_context_v1" {
		t.Fatalf("unexpected CLI output: %+v", got)
	}
	for _, privateValue := range []string{"Praxis A", "Praxis B"} {
		if strings.Contains(stdout.String(), privateValue) {
			t.Fatalf("CLI output contains private source value %q", privateValue)
		}
	}
}

func TestPrescriptionMetricCLIRejectsMismatchedContext(t *testing.T) {
	dbPath, receiptPath := prescriptionSummaryFixture(t, true)
	var stdout bytes.Buffer
	err := run(
		context.Background(),
		[]string{
			"report", "prescription-metric",
			"--db", dbPath,
			"--capability", "reha-prescription-summary",
			"--context-file", receiptPath,
			"--format", "json",
		},
		strings.NewReader(""),
		&stdout,
	)
	if err == nil || !strings.Contains(err.Error(), "PRESCRIPTION_CONTEXT_FAILED") {
		t.Fatalf("expected context mismatch, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("mismatched context wrote output: %q", stdout.String())
	}
}

func prescriptionSummaryFixture(t *testing.T, mismatchedHash bool) (string, string) {
	t.Helper()
	directory := t.TempDir()
	dbPath := filepath.Join(directory, "summary.sqlite")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	page := admin.Page{
		Metadata: admin.PageMetadata{
			Route:        "capability:reha-prescription-summary",
			TableHeaders: [][]string{{"", "Arzt", "Menge"}},
			TableRows:    []int{2},
			Fingerprint:  strings.Repeat("c", 64),
		},
		Tables: []admin.Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows: [][]string{
				{"", "Praxis A", "2"},
				{"", "Praxis B", "3"},
			},
		}},
	}
	if _, err := db.ImportAdminPage(context.Background(), page); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		t.Fatal(err)
	}
	ro, err := store.OpenReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	metric, err := ro.PrescriptionMetric(context.Background(), "reha-prescription-summary")
	if err != nil {
		_ = ro.Close()
		t.Fatal(err)
	}
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
	databaseBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(databaseBytes)
	hash := hex.EncodeToString(digest[:])
	if mismatchedHash {
		hash = strings.Repeat("0", 64)
	}
	receipt := store.PrescriptionContextReceipt{
		Version:        1,
		Capability:     "reha-prescription-summary",
		From:           "2026-08-01",
		To:             "2026-08-31",
		DatabaseSHA256: hash,
		ObservedAt:     metric.ObservedAt,
		CapturedAt:     time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		BindingBasis:   "operator_captured_hash_bound_context_v1",
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(directory, "context.json")
	if err := os.WriteFile(receiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dbPath, receiptPath
}
