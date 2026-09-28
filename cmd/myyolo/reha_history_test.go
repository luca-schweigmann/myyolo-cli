package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func TestRehaHistoryCLIReportsJSONWithoutPersonalOrPlanFields(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"import", "mysign", "--file", fixture, "--db", sourcePath,
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	scopePath, receiptPath, snapshotPath := writeCLIRehaScope(t, dir, sourcePath)
	output.Reset()
	if err := run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", sourcePath,
		"--output-db", snapshotPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := run(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-gerlingen",
		"--as-of", "2026-09-30T00:00:00Z",
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
}

func TestRehaHistoryCLIRejectsFormatAndGuardsSnapshotAndScope(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	if err := run(context.Background(), []string{
		"import", "mysign", "--file", fixture, "--db", sourcePath,
	}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	scopePath, receiptPath, snapshotPath := writeCLIRehaScope(t, dir, sourcePath)
	if err := run(context.Background(), []string{
		"db", "snapshot",
		"--source-db", sourcePath,
		"--output-db", snapshotPath,
		"--scope-input", scopePath,
		"--receipt", receiptPath,
	}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	err := run(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-gerlingen",
		"--as-of", "2000-01-01T00:00:00Z",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_SCOPE_FAILED") {
		t.Fatalf("as-of before snapshot error = %v", err)
	}

	err = run(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-gerlingen",
		"--format", "json",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_INVALID_ARGUMENTS") {
		t.Fatalf("format option error = %v", err)
	}

	err = run(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-ditzingen",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_SCOPE_MISMATCH") {
		t.Fatalf("location mismatch error = %v", err)
	}

	file, err := os.OpenFile(snapshotPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tampered")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), []string{
		"report", "reha-history",
		"--db", snapshotPath,
		"--scope-file", receiptPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--location", "point-gerlingen",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_SNAPSHOT_FAILED") {
		t.Fatalf("tampered snapshot error = %v", err)
	}
}

func TestRehaHistoryAppearsInHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "report reha-history") {
		t.Fatalf("help lacks reha-history: %s", output.String())
	}
}

func TestRehaHistoryCLIV2IsExplicitAndIncludesManagementFields(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	if err := run(context.Background(), []string{"import", "mysign", "--file", fixture, "--db", sourcePath}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	scopePath, receiptPath, snapshotPath := writeCLIRehaScope(t, dir, sourcePath)
	if err := run(context.Background(), []string{"db", "snapshot", "--source-db", sourcePath, "--output-db", snapshotPath, "--scope-input", scopePath, "--receipt", receiptPath}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	args := []string{"report", "reha-history", "--db", snapshotPath, "--scope-file", receiptPath, "--from", "2026-07-01", "--to", "2026-07-31", "--location", "point-gerlingen", "--as-of", "2030-01-01T00:00:00Z", "--schema-version", "v2"}
	var output bytes.Buffer
	if err := run(context.Background(), args, strings.NewReader(""), &output); err != nil {
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
	if err := run(context.Background(), args, strings.NewReader(""), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "REHA_HISTORY_INVALID_ARGUMENTS") {
		t.Fatalf("invalid version: %v", err)
	}
}
