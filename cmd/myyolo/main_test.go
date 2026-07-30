package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineImportAndReports(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	fixture := filepath.Join("..", "..", "internal", "mysign", "testdata", "get_list_data.synthetic.json")
	var stdout bytes.Buffer
	if err := run(
		context.Background(),
		[]string{"import", "mysign", "--file", fixture, "--db", dbPath},
		strings.NewReader(""),
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run(
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

func TestMemberReportRequiresExplicitPIIGate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	var stdout bytes.Buffer
	err := run(
		context.Background(),
		[]string{"report", "members", "--db", dbPath},
		strings.NewReader(""),
		&stdout,
	)
	if err == nil || !strings.Contains(err.Error(), "--include-personal-data") {
		t.Fatalf("error = %v", err)
	}
}

func TestPasswordStdinIsBounded(t *testing.T) {
	var stdout bytes.Buffer
	password, err := readPassword(strings.NewReader("secret\n"), &stdout, true)
	if err != nil {
		t.Fatal(err)
	}
	if password != "secret" {
		t.Fatalf("password = %q", password)
	}
	_, err = readPassword(strings.NewReader(strings.Repeat("x", 4097)), &stdout, true)
	if err == nil {
		t.Fatal("expected oversized password to fail")
	}
}

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(
		context.Background(),
		[]string{"version"},
		os.Stdin,
		&stdout,
	); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("version output is empty")
	}
}
