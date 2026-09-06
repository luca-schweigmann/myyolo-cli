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

	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func TestRehaInactivityCLIReportsUnavailableAggregateCapability(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	var imported bytes.Buffer
	if err := run(context.Background(), []string{"import", "mysign", "--file", fixture, "--db", sourcePath}, strings.NewReader(""), &imported); err != nil {
		t.Fatal(err)
	}
	scopePath, receiptPath, snapshotPath := writeCLIRehaScope(t, dir, sourcePath)
	var snapshotOutput bytes.Buffer
	if err := run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", sourcePath,
		"--output-db", snapshotPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &snapshotOutput); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"report", "reha-inactivity",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--as-of", "2026-09-06T12:00:00+02:00",
		"--location", "point-gerlingen",
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	var report struct {
		CapabilityStatus string `json:"capability_status"`
		Over28Days       struct {
			Status string `json:"status"`
			Count  *int   `json:"count"`
		} `json:"over_28_days"`
		Over3Months struct {
			Status string `json:"status"`
			Count  *int   `json:"count"`
		} `json:"over_3_months"`
		UnknownHistory struct {
			Status string `json:"status"`
			Count  *int   `json:"count"`
		} `json:"unknown_history"`
		CurrentAssignment string `json:"current_assignment"`
		MemberStatus      string `json:"member_status"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.CapabilityStatus != "unavailable" || report.CurrentAssignment != "unavailable" || report.MemberStatus != "unavailable" {
		t.Fatalf("report status = %#v", report)
	}
	for name, bucket := range map[string]struct {
		status string
		count  *int
	}{
		"over_28_days":    {report.Over28Days.Status, report.Over28Days.Count},
		"over_3_months":   {report.Over3Months.Status, report.Over3Months.Count},
		"unknown_history": {report.UnknownHistory.Status, report.UnknownHistory.Count},
	} {
		if bucket.status != "unavailable" || bucket.count != nil {
			t.Fatalf("%s = %#v", name, bucket)
		}
	}
	assertOutputOmitsPrivateData(t, output.String(), "json")
}

func writeCLIRehaScope(t *testing.T, dir, sourcePath string) (string, string, string) {
	t.Helper()
	canonical, err := filepath.Abs(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	location := "point-gerlingen"
	evidencePath := filepath.Join(dir, "evidence.json")
	evidence := store.RehaEvidenceReceipt{
		Version:    1,
		SourceDB:   canonical,
		Source:     "mysign",
		Location:   location,
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
	digest := sha256.Sum256(evidenceBytes)
	scope := store.RehaScopeInput{
		Version:    1,
		Source:     "mysign",
		SourceDB:   canonical,
		Location:   location,
		Provenance: "external_verified_scope",
		Evidence: store.RehaScopeEvidence{
			Kind:       evidence.Kind,
			Reference:  evidencePath,
			SHA256:     hex.EncodeToString(digest[:]),
			VerifiedAt: evidence.VerifiedAt,
			SourceDB:   canonical,
			Source:     "mysign",
			Location:   location,
		},
	}
	scopeBytes, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(dir, "scope.json")
	if err := os.WriteFile(scopePath, append(scopeBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return scopePath, filepath.Join(dir, "receipt.json"), filepath.Join(dir, "snapshot.sqlite")
}

func TestRehaInactivityCLIReusesReceiptChecks(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	err := run(context.Background(), []string{
		"report", "reha-inactivity",
		"--db", filepath.Join(dir, "missing.sqlite"),
		"--scope-file", filepath.Join(dir, "missing-receipt.json"),
		"--as-of", "2026-09-06T12:00:00Z",
		"--location", "point-gerlingen",
	}, strings.NewReader(""), &output)
	if err == nil || !strings.Contains(err.Error(), "REHA_INACTIVITY_SCOPE_FAILED") {
		t.Fatalf("missing receipt error = %v", err)
	}
	if strings.Contains(err.Error(), "missing-receipt.json") {
		t.Fatalf("receipt path leaked: %v", err)
	}
}
