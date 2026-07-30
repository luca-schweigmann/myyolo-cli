package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"github.com/luca-schweigmann/myyolo-cli/internal/transport"
	"golang.org/x/term"
)

func authLogin(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	profile := flags.String("profile", "default", "credential profile")
	partner := flags.String("partner", "", "myYOLO partner number")
	username := flags.String("username", "", "myYOLO username")
	passwordStdin := flags.Bool("password-stdin", false, "read password from standard input")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := secrets.ValidateProfile(*profile); err != nil {
		return err
	}
	if *partner == "" || *username == "" {
		return errors.New("--partner and --username are required")
	}
	password, err := readPassword(stdin, stdout, *passwordStdin)
	if err != nil {
		return err
	}
	credentials := secrets.Credentials{
		PartnerNumber: *partner,
		Username:      *username,
		Password:      password,
	}
	client, err := transport.New(&http.Client{})
	if err != nil {
		return err
	}
	session, err := client.Login(ctx, credentials)
	if err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	if err := secretStore.SaveCredentials(*profile, credentials); err != nil {
		return err
	}
	if err := secretStore.SaveSession(*profile, session); err != nil {
		return err
	}
	return output.Write(stdout, struct {
		Status  string `json:"status"`
		Profile string `json:"profile"`
	}{Status: "authenticated", Profile: *profile}, "json")
}

func authStatus(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth status", flag.ContinueOnError)
	profile := flags.String("profile", "default", "credential profile")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	_, credentialErr := secretStore.LoadCredentials(*profile)
	_, sessionErr := secretStore.LoadSession(*profile)
	if credentialErr != nil && !secrets.IsNotFound(credentialErr) {
		return credentialErr
	}
	if sessionErr != nil && !secrets.IsNotFound(sessionErr) {
		return sessionErr
	}
	return output.Write(stdout, struct {
		Profile       string `json:"profile"`
		Configured    bool   `json:"configured"`
		SessionCached bool   `json:"session_cached"`
	}{
		Profile:       *profile,
		Configured:    credentialErr == nil,
		SessionCached: sessionErr == nil,
	}, "json")
}

func authLogout(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	profile := flags.String("profile", "default", "credential profile")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := secrets.NewKeyringStore().DeleteProfile(*profile); err != nil {
		return err
	}
	return output.Write(stdout, struct {
		Status  string `json:"status"`
		Profile string `json:"profile"`
	}{Status: "local credentials removed", Profile: *profile}, "json")
}

func syncMyYOLO(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	profile := flags.String("profile", "default", "credential profile")
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("profile %q is not configured; run myyolo auth login", *profile)
		}
		return err
	}
	session, err := secretStore.LoadSession(*profile)
	if err != nil && !secrets.IsNotFound(err) {
		return err
	}

	client, err := transport.New(&http.Client{})
	if err != nil {
		return err
	}
	snapshot, nextSession, err := client.Fetch(ctx, credentials, session)
	if err != nil {
		return err
	}
	if err := secretStore.SaveSession(*profile, nextSession); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("hash normalized snapshot: %w", err)
	}
	hash := sha256.Sum256(data)
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ImportMySign(ctx, snapshot, hex.EncodeToString(hash[:]))
	if err != nil {
		return err
	}
	return output.Write(stdout, result, "json")
}

func initDB(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("db init", flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return output.Write(stdout, struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}{Status: "ready", Database: *dbPath}, "json")
}

func importMySign(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("import mysign", flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	filePath := flags.String("file", "", "mySIGN GetListData JSON file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	data, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	snapshot, err := mysign.Parse(data)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ImportMySign(ctx, snapshot, hex.EncodeToString(hash[:]))
	if err != nil {
		return err
	}
	return output.Write(stdout, result, "json")
}

func printReport(ctx context.Context, reportName string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report "+reportName, flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	format := flags.String("format", "table", "table, json or csv")
	includePersonalData := flags.Bool(
		"include-personal-data",
		false,
		"allow member names and identifiers in local output",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	var report any
	switch reportName {
	case "summary":
		report, err = db.Summary(ctx)
	case "courses":
		report, err = db.Courses(ctx)
	case "days":
		report, err = db.Days(ctx)
	case "hours":
		report, err = db.Hours(ctx)
	case "sessions":
		report, err = db.Sessions(ctx)
	case "members":
		if !*includePersonalData {
			return errors.New("member report requires --include-personal-data")
		}
		report, err = db.Members(ctx)
	default:
		return fmt.Errorf("unknown report %q", reportName)
	}
	if err != nil {
		return err
	}
	return output.Write(stdout, report, *format)
}

func readPassword(stdin io.Reader, stdout io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(io.LimitReader(stdin, 4097))
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if len(data) > 4096 {
			return "", errors.New("password input is too long")
		}
		password := strings.TrimRight(string(data), "\r\n")
		if password == "" {
			return "", errors.New("password is empty")
		}
		return password, nil
	}
	if configured := os.Getenv("MYYOLO_PASSWORD"); configured != "" {
		if len(configured) > 4096 {
			return "", errors.New("MYYOLO_PASSWORD is too long")
		}
		return configured, nil
	}
	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("interactive password prompt requires a terminal; use --password-stdin")
	}
	if _, err := fmt.Fprint(stdout, "Password: "); err != nil {
		return "", err
	}
	data, err := term.ReadPassword(int(file.Fd()))
	if _, printErr := fmt.Fprintln(stdout); err == nil {
		err = printErr
	}
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("password is empty")
	}
	return string(data), nil
}

func defaultDBPath() string {
	if configured := os.Getenv("MYYOLO_DB_PATH"); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "data", "myyolo.sqlite")
	}
	return filepath.Join(home, ".local", "share", "myyolo-cli", "myyolo.sqlite")
}
