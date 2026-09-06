package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func TestRehaCLIReportsAllFormatsWithoutPrivateFields(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportMySign(context.Background(), snapshot, "synthetic"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(dir, "scope.json")
	canonical, err := filepath.Abs(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	scope := store.RehaScopeInput{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   "point-gerlingen",
		Provenance: "external_verified_scope",
		Evidence: store.RehaScopeEvidence{
			Kind:       "independent-local-receipt",
			Reference:  filepath.Join(dir, "evidence.json"),
			VerifiedAt: "2026-09-05T00:00:00Z",
			SourceDB:   canonical,
			Source:     "mysign",
			Location:   "point-gerlingen",
		},
	}
	evidencePath, err := filepath.Abs(scope.Evidence.Reference)
	if err != nil {
		t.Fatal(err)
	}
	scope.Evidence.Reference = evidencePath
	evidence := store.RehaEvidenceReceipt{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   "point-gerlingen",
		VerifiedAt: "2026-09-05T00:00:00Z",
		Kind:       "independent-local-receipt",
	}
	evidenceBytes, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidenceBytes = append(evidenceBytes, '\n')
	if err := os.WriteFile(evidencePath, evidenceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	evidenceHash := sha256.Sum256(evidenceBytes)
	scope.Evidence.SHA256 = hex.EncodeToString(evidenceHash[:])
	scopeBytes, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scopePath, append(scopeBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "snapshot.sqlite")
	receiptPath := filepath.Join(dir, "snapshot.scope.json")
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", sourcePath,
		"--output-db", outputPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"status": "created"`) {
		t.Fatalf("snapshot output = %s", stdout.String())
	}
	for _, format := range []string{"table", "json", "csv"} {
		t.Run(format, func(t *testing.T) {
			stdout.Reset()
			err := run(context.Background(), []string{
				"report", "reha-sessions",
				"--db", outputPath,
				"--scope-file", receiptPath,
				"--from", "2026-07-28",
				"--to", "2026-07-29",
				"--location", "point-gerlingen",
				"--as-of", "2026-07-30T00:00:00+02:00",
				"--format", format,
			}, strings.NewReader(""), &stdout)
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{"stable_session_id", "participant_count_current_observation", "missing_signature_candidate", "prescription_linked_rows", "contradictory_flags", "point-gerlingen"} {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("%s output lacks %q: %s", format, expected, stdout.String())
				}
			}
			assertOutputOmitsPrivateData(t, stdout.String(), format)
			if format == "json" {
				var report store.RehaSessionReport
				if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
					t.Fatal(err)
				}
				if len(report.Sessions) != 2 || report.Coverage.SessionsInRange != 2 {
					t.Fatalf("JSON report = %#v", report)
				}
			}
		})
	}
	stdout.Reset()
	err = run(context.Background(), []string{
		"report", "reha-sessions",
		"--db", outputPath,
		"--scope-file", receiptPath,
		"--from", "2026-07-28",
		"--to", "2026-07-29",
		"--location", "point-ditzingen",
	}, strings.NewReader(""), &stdout)
	if err == nil || !strings.Contains(err.Error(), "REHA_SCOPE_MISMATCH") {
		t.Fatalf("location mismatch error = %v", err)
	}
}

func assertOutputOmitsPrivateData(t *testing.T, outputValue, format string) {
	t.Helper()
	for _, privateText := range []string{"Beispiel", "Muster", "Reha Rücken", "Kursraum 1"} {
		if strings.Contains(outputValue, privateText) {
			t.Fatalf("%s output leaked %q: %s", format, privateText, outputValue)
		}
	}
	privateIDs := map[string]struct{}{"501": {}, "1001": {}}
	checkScalar := func(value string) {
		if _, forbidden := privateIDs[value]; forbidden {
			t.Errorf("%s output exposes private scalar %q", format, value)
		}
	}
	switch format {
	case "json":
		var document any
		if err := json.Unmarshal([]byte(outputValue), &document); err != nil {
			t.Fatalf("decode JSON output: %v", err)
		}
		privateFields := map[string]struct{}{
			"first_name": {}, "last_name": {}, "member_id": {},
			"member_number": {}, "myyolo_id": {}, "room_name": {},
		}
		var inspect func(any)
		inspect = func(value any) {
			switch typed := value.(type) {
			case map[string]any:
				for key, nested := range typed {
					if _, forbidden := privateFields[key]; forbidden {
						t.Errorf("JSON output exposes private field %q", key)
					}
					inspect(nested)
				}
			case []any:
				for _, nested := range typed {
					inspect(nested)
				}
			case string:
				checkScalar(typed)
			}
		}
		inspect(document)
	case "csv":
		records, err := csv.NewReader(strings.NewReader(outputValue)).ReadAll()
		if err != nil {
			t.Fatalf("decode CSV output: %v", err)
		}
		for _, record := range records {
			for _, value := range record {
				checkScalar(value)
			}
		}
	case "table":
		for _, value := range strings.Fields(outputValue) {
			checkScalar(value)
		}
	default:
		t.Fatalf("unsupported test output format %q", format)
	}
}

func TestRehaCommandsAppearInHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, strings.NewReader(""), &stdout); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"db snapshot", "report reha-sessions", "--scope-input", "--scope-file"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("help lacks %q: %s", expected, stdout.String())
		}
	}
}

func TestRehaCLIErrorsDoNotEchoRawInputsOrSQLiteDetails(t *testing.T) {
	dir := t.TempDir()
	missingSource := filepath.Join(dir, "private-member-123.sqlite")
	scopePath := filepath.Join(dir, "scope.json")
	canonical, err := filepath.Abs(missingSource)
	if err != nil {
		t.Fatal(err)
	}
	scope := store.RehaScopeInput{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   "point-gerlingen",
		Provenance: "external_verified_scope",
		Evidence: store.RehaScopeEvidence{
			Kind:       "independent-local-receipt",
			Reference:  "synthetic-scope-ref",
			SHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			VerifiedAt: "2026-09-05T00:00:00Z",
			SourceDB:   canonical,
			Source:     "mysign",
			Location:   "point-gerlingen",
		},
	}
	scopeBytes, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scopePath, append(scopeBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "private-output.sqlite")
	receiptPath := filepath.Join(dir, "private-receipt.json")
	var stdout bytes.Buffer
	err = run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", missingSource,
		"--output-db", outputPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &stdout)
	if err == nil {
		t.Fatal("missing source unexpectedly accepted")
	}
	for _, secret := range []string{missingSource, "private-member-123", "no such table", "quick_check"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("snapshot error leaked %q: %v", secret, err)
		}
	}
	if !strings.HasPrefix(err.Error(), "REHA_SNAPSHOT_FAILED:") {
		t.Fatalf("snapshot error = %v", err)
	}

	stdout.Reset()
	err = run(context.Background(), []string{
		"report", "reha-sessions",
		"--db", "snapshot.sqlite",
		"--scope-file", "scope.json",
		"--from", "2026-99-01",
		"--to", "2026-07-29",
		"--location", "point-gerlingen",
		"--format", "=private-formula",
		"--as-of", "not-a-timestamp-secret",
	}, strings.NewReader(""), &stdout)
	if err == nil {
		t.Fatal("invalid report arguments unexpectedly accepted")
	}
	for _, secret := range []string{"2026-99-01", "=private-formula", "not-a-timestamp-secret", "snapshot.sqlite"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("report argument error leaked %q: %v", secret, err)
		}
	}
	if !strings.HasPrefix(err.Error(), "REHA_INVALID_ARGUMENTS:") {
		t.Fatalf("report argument error = %v", err)
	}

	unknownScope := filepath.Join(dir, "unknown-scope.json")
	if err := os.WriteFile(unknownScope, []byte(`{"version":1,"source_db":"`+canonical+`","source":"mysign","location":"point-gerlingen","provenance":"external_verified_scope","evidence":{},"secret_field":"member-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", missingSource,
		"--output-db", filepath.Join(dir, "unknown-output.sqlite"),
		"--scope-input", unknownScope,
		"--receipt", filepath.Join(dir, "unknown-receipt.json"),
	}, strings.NewReader(""), &stdout)
	if err == nil {
		t.Fatal("unknown scope field unexpectedly accepted")
	}
	for _, secret := range []string{"secret_field", "member-secret", "DisallowUnknownFields"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("scope error leaked %q: %v", secret, err)
		}
	}
	if !strings.HasPrefix(err.Error(), "REHA_SNAPSHOT_FAILED:") {
		t.Fatalf("scope error = %v", err)
	}
}
