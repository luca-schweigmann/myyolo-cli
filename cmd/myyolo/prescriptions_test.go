package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrescriptionMetricCLIUsesHashBoundContext(t *testing.T) {
	dbPath, receiptPath := prescriptionSummaryFixture(t, false)
	var stdout bytes.Buffer
	err := runCLI(
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

	{
		dbPath, receiptPath := prescriptionSummaryFixture(t, true)
		var stdout bytes.Buffer
		err := runCLI(
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

func TestPrescriptionObservationExportsOnlyHashes(t *testing.T) {
	activeHeaders := []string{"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", ""}
	archivedHeaders := []string{"Nr.", "M-Nr.:", "Name", "Vorname", "Ende"}
	activePage := admin.Page{
		Metadata: admin.PageMetadata{
			Route:        "capability:reha-prescriptions",
			TableHeaders: [][]string{activeHeaders},
			TableRows:    []int{2},
			Fingerprint:  strings.Repeat("a", 64),
		},
		Tables: []admin.Table{{Headers: activeHeaders, Rows: [][]string{
			{"9001", "M-77", "Mustermann", "Erika", "12", "3", "9", "", ""},
			{"9002", "M-78", "Beispiel", "Max", "12", "5", "7", "", ""},
		}}},
	}
	archivedPage := func(headers []string, rows [][]string) admin.Page {
		return admin.Page{
			Metadata: admin.PageMetadata{
				Route:        "capability:reha-prescriptions-archived",
				TableHeaders: [][]string{headers},
				TableRows:    []int{len(rows)},
				Fingerprint:  strings.Repeat("b", 64),
			},
			Tables: []admin.Table{{Headers: headers, Rows: rows}},
		}
	}
	observe := func(pages ...admin.Page) (prescriptionObservationFixture, string) {
		dbPath := filepath.Join(t.TempDir(), "observation.sqlite")
		db, err := store.Open(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, page := range pages {
			if _, err := db.ImportAdminPage(context.Background(), page); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		err = runCLI(context.Background(),
			[]string{"report", "prescription-observation", "--db", dbPath},
			strings.NewReader(""), &stdout)
		if err != nil {
			t.Fatal(err)
		}
		var got prescriptionObservationFixture
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got, stdout.String()
	}

	got, raw := observe(activePage, archivedPage(archivedHeaders, [][]string{
		{"8001", "M-10", "Alt", "Anna", "01.09.2026"},
	}))
	if got.Active.Status != "ok" || got.Active.CountedRows != 2 || len(got.Active.Hashes) != 2 ||
		got.Archived.Status != "ok" || len(got.Archived.Hashes) != 1 {
		t.Fatalf("unexpected observation: %+v", got)
	}
	digest := sha256.Sum256([]byte("myyolo-prescription-v1\x008001"))
	if got.Archived.Hashes[0] != hex.EncodeToString(digest[:]) {
		t.Fatalf("archived hash is not the documented scheme: %s", got.Archived.Hashes[0])
	}
	for _, private := range []string{"Mustermann", "Erika", "Beispiel", "Alt", "M-77", "9001", "8001"} {
		if strings.Contains(raw, private) {
			t.Fatalf("observation leaks %q", private)
		}
	}

	if got, _ := observe(activePage); got.Archived.Status != "not_collected" || len(got.Archived.Hashes) != 0 {
		t.Fatalf("missing archive must be not_collected, got %+v", got.Archived)
	}
	if got, _ := observe(activePage, archivedPage([]string{"Name", "Ende"}, [][]string{{"x", "y"}})); got.Archived.Status != "schema_unmatched" {
		t.Fatalf("unknown archive schema must be schema_unmatched, got %+v", got.Archived)
	}
}

type prescriptionObservationFixture struct {
	Active   store.PrescriptionHashSource `json:"active"`
	Archived store.PrescriptionHashSource `json:"archived"`
}
