package main

import (
	"bytes"
	"context"
	"crypto/hmac"
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

// Flow: a collected "Resttermine" statistic becomes keyed unit states; no
// name or member number leaves the CLI, and a foreign or unclear operator
// location, a lower threshold or a weak key file fail closed.
func TestPrescriptionObservationKeysRemainingUnits(t *testing.T) {
	headers := []string{"", "Nr.:", "Name", "Vorname", "Art", "Termine", "wahrgenommen", "Rest", "Termine", "Ende", ""}
	page := func(operator, body string, rows ...[]string) admin.Page {
		return admin.Page{
			Metadata: admin.PageMetadata{
				Route:        "capability:prescription-remaining-units",
				TableHeaders: [][]string{nil, headers},
				TableRows:    []int{1, len(rows)},
				Fingerprint:  strings.Repeat("c", 64),
			},
			Tables: []admin.Table{
				{Rows: [][]string{{"", "Benutzer: Test Person", operator}}},
				{Headers: headers, Rows: rows},
			},
			RequestPath: "/Statistiken/Verordnungen/Verordnung_Personen_Rest_Termine.asp",
			RequestBody: body,
		}
	}
	rows := [][]string{
		{"1", "4711", "Mustermann", "Erika", "Reha", "18", "16", "2", "5", "30.11.2026", "Details"},
		{"2", "815", "Beispiel", "Max", "Reha", "50", "0", "50", "12", "15.10.2029", "Details"},
	}
	directory := t.TempDir()
	keyPath := filepath.Join(directory, "key")
	key := []byte(strings.Repeat("k", 32))
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	observe := func(page admin.Page, keyFile string) (store.PrescriptionObservation, string, error) {
		dbPath := filepath.Join(t.TempDir(), "observation.sqlite")
		db, err := store.Open(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ImportAdminPage(context.Background(), page); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		err = runCLI(context.Background(),
			[]string{"report", "prescription-observation", "--db", dbPath, "--key-file", keyFile},
			strings.NewReader(""), &stdout)
		var got store.PrescriptionObservation
		if err == nil {
			if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
		}
		return got, stdout.String(), err
	}

	got, raw, err := observe(page("rehavital e.v. JA Sport Gerlingen 70839 Gerlingen", "Menge=999", rows...), keyPath)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("4711|2026-11-30"))
	want := admin.PrescriptionState{Key: hex.EncodeToString(mac.Sum(nil)), Termine: 18, Wahrgenommen: 16, Rest: 2, Ende: "2026-11-30"}
	if got.Location != "70839 Gerlingen" || len(got.Rows) != 2 || got.KeyID != admin.KeyID(key) {
		t.Fatalf("unexpected observation: %+v", got)
	}
	found := false
	for _, row := range got.Rows {
		found = found || row == want
	}
	if !found {
		t.Fatalf("row is not HMAC(member|end) with its unit counts: %+v", got.Rows)
	}
	for _, private := range []string{"Mustermann", "Erika", "Beispiel", "4711", "815", "Test Person"} {
		if strings.Contains(raw, private) {
			t.Fatalf("observation leaks %q", private)
		}
	}

	for name, failing := range map[string]admin.Page{
		"no location":  page("rehavital e.v.", "Menge=999", rows...),
		"threshold 10": page("rehavital e.v. 70839 Gerlingen", "Menge=10", rows...),
		"rest drift":   page("rehavital e.v. 70839 Gerlingen", "Menge=999", []string{"1", "9", "A", "B", "Reha", "18", "16", "3", "5", "30.11.2026", ""}),
	} {
		if _, raw, err := observe(failing, keyPath); err == nil || raw != "" {
			t.Fatalf("%s must fail closed, got %v %q", name, err, raw)
		}
	}
	twin := []string{"3", "4711", "Mustermann", "Erika", "Reha", "10", "1", "9", "2", "30.11.2026", ""}
	if _, raw, err := observe(page("70839 Gerlingen", "Menge=999", append(rows, twin)...), keyPath); err == nil ||
		!strings.Contains(err.Error(), "PRESCRIPTION_KEY_COLLISION") || raw != "" {
		t.Fatalf("same member and end date must fail with its own code, got %v %q", err, raw)
	}
	if got, _, err := observe(page("rehavital e.v. 71254 Ditzingen", "Menge=999", rows...), keyPath); err != nil || got.Location != "71254 Ditzingen" {
		t.Fatalf("a foreign location is reported as it is for the caller to reject, got %v %+v", err, got.Location)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := observe(page("70839 Gerlingen", "Menge=999", rows...), keyPath); err == nil || !strings.Contains(err.Error(), "PRESCRIPTION_KEY_FAILED") {
		t.Fatalf("a readable key file must be rejected, got %v", err)
	}
}
