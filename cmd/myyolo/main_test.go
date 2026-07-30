package main

import (
	"bytes"
	"context"
	"io"
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

func TestAdminDetailReportsRequireExplicitPIIGate(t *testing.T) {
	for _, report := range []string{
		"admin-reha-attendance",
		"admin-missing-signature-members",
		"admin-records",
	} {
		t.Run(report, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
			var stdout bytes.Buffer
			err := run(
				context.Background(),
				[]string{"report", report, "--db", dbPath},
				strings.NewReader(""),
				&stdout,
			)
			if err == nil || !strings.Contains(err.Error(), "--include-personal-data") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestReportAsOfMustBeRFC3339(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	var stdout bytes.Buffer
	err := run(
		context.Background(),
		[]string{"report", "summary", "--db", dbPath, "--as-of", "not-a-time"},
		strings.NewReader(""),
		&stdout,
	)
	if err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdminCommandsRejectUnsafeDelayBeforeAccessingCredentials(t *testing.T) {
	for _, args := range [][]string{
		{"sync", "admin", "--delay", "1s"},
		{"discover", "admin", "--delay", "1s"},
	} {
		var stdout bytes.Buffer
		err := run(
			context.Background(),
			args,
			strings.NewReader(""),
			&stdout,
		)
		if err == nil || !strings.Contains(err.Error(), "at least 2s") {
			t.Fatalf("args = %#v, error = %v", args, err)
		}
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

func TestAuthLoginRejectsUnknownSourceBeforeReadingPassword(t *testing.T) {
	err := run(
		context.Background(),
		[]string{
			"auth", "login",
			"--source", "unknown",
			"--partner", "synthetic",
			"--username", "synthetic",
			"--password-stdin",
		},
		strings.NewReader("should-not-be-read"),
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "--source must be") {
		t.Fatalf("error = %v", err)
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

func TestDatabaseStatusCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "myyolo.sqlite")
	var stdout bytes.Buffer
	if err := run(
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

func TestBuildVersionPrefersLinkerValue(t *testing.T) {
	previous := version
	version = "v0.1.0"
	t.Cleanup(func() {
		version = previous
	})
	if got := buildVersion(); got != "v0.1.0" {
		t.Fatalf("buildVersion() = %q", got)
	}
}
