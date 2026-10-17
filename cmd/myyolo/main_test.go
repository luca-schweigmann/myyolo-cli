package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineImportAndReports(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	var stdout bytes.Buffer
	if err := runCLI(
		context.Background(),
		[]string{"import", "mysign", "--file", fixture, "--db", dbPath},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := runCLI(
		context.Background(),
		[]string{"report", "days", "--format", "csv", "--db", dbPath},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "2026-07-28") {
		t.Fatalf("daily report = %q", stdout.String())
	}
}

func TestCatalogIsLocalAndIncludesTypedCapabilities(t *testing.T) {
	var stdout bytes.Buffer
	if err := runCLI(
		context.Background(),
		[]string{"catalog", "--group", "analytics", "--format", "json"},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"name": "studio-hourly-load"`,
		`"sensitivity": "aggregate"`,
		`"--from`,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("catalog output %q lacks %q", stdout.String(), expected)
		}
	}
}

func TestCollectDryRunIsOfflineAndRedactsDynamicValues(t *testing.T) {
	var stdout bytes.Buffer
	const memberID = "987654321"
	if err := runCLI(
		context.Background(),
		[]string{
			"collect", "member-detail",
			"--member-id", memberID,
			"--include-personal-data",
			"--dry-run",
		},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), memberID) || strings.Contains(stdout.String(), "?ID=") {
		t.Fatalf("dry-run leaked a dynamic value: %q", stdout.String())
	}
	for _, expected := range []string{
		`"remote_requests": 0`,
		`"local_database_write": false`,
		`"--member-id`,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("dry-run output %q lacks %q", stdout.String(), expected)
		}
	}
}

func TestDatabaseStatusCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	var stdout bytes.Buffer
	if err := runCLI(
		context.Background(),
		[]string{"db", "status", "--db", dbPath},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"integrity": "ok"`, `"schema_version":`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("status output %q lacks %q", stdout.String(), expected)
		}
	}
}
