package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestRehaHistoryCLIReportsJSONWithoutPersonalOrPlanFields(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	var output bytes.Buffer
	if err := runCLI(context.Background(), []string{
		"import", "mysign", "--file", fixture, "--db", sourcePath,
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	scopePath, receiptPath, snapshotPath := writeCLIRehaScope(t, dir, sourcePath)
	output.Reset()
	if err := runCLI(context.Background(), []string{
		"db", "snapshot",
		"--source-db", sourcePath,
		"--output-db", snapshotPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := runCLI(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-gerlingen",
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	var report store.RehaHistoryReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("history output is not JSON: %v; output=%s", err, output.String())
	}
	if report.SchemaVersion != "reha-history-observations.v1" || report.Coverage != "observed_rows_only" {
		t.Fatalf("history report metadata = %#v", report)
	}
	receipt, err := store.ReadSnapshotReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.SnapshotAt != receipt.CompletedAt {
		t.Fatalf("snapshot_at = %q, receipt completed_at = %q", report.SnapshotAt, receipt.CompletedAt)
	}
	if len(report.Sessions) == 0 {
		t.Fatalf("history report unexpectedly empty: %s", output.String())
	}
	assertOutputOmitsPrivateData(t, output.String(), "json")
	for _, forbiddenField := range []string{"member_id", "first_name", "last_name", "prescription", "capacity", "plan"} {
		if strings.Contains(output.String(), forbiddenField) {
			t.Fatalf("history output leaked field %q: %s", forbiddenField, output.String())
		}
	}

	{
		args := []string{"report", "reha-history", "--db", snapshotPath, "--scope-file", receiptPath, "--from", "2026-07-01", "--to", "2026-07-31", "--location", "point-gerlingen", "--schema-version", "v2"}
		var output bytes.Buffer
		if err := runCLI(context.Background(), args, strings.NewReader(""), &output); err != nil {
			t.Fatal(err)
		}
		var report store.RehaHistoryReport
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.SchemaVersion != store.RehaHistoryManagementSchemaVersion || len(report.Sessions) != 2 {
			t.Fatalf("report: %#v", report)
		}
		if report.Sessions[0].CourseLabel != "Reha Rücken" || report.Sessions[0].Registered == nil || *report.Sessions[0].Registered != 2 || (report.Sessions[0].Participated == nil || *report.Sessions[0].Participated != 1) {
			t.Fatalf("management row: %#v", report.Sessions[0])
		}
		for _, forbidden := range []string{"Beispiel", "Muster", "Kursraum 1", `"member_id"`, `"prescription_id"`} {
			if strings.Contains(output.String(), forbidden) {
				t.Fatalf("leak %q", forbidden)
			}
		}
		args[len(args)-1] = "v3"
		if err := runCLI(context.Background(), args, strings.NewReader(""), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_INVALID_ARGUMENTS") {
			t.Fatalf("invalid version: %v", err)
		}
	}
}
