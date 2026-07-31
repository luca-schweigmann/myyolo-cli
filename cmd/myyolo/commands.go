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
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"github.com/luca-schweigmann/myyolo-cli/internal/transport"
	"golang.org/x/term"
)

func authLogin(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	partner := flags.String("partner", "", "myYOLO-Partnernummer")
	username := flags.String("username", "", "myYOLO-Benutzername")
	passwordStdin := flags.Bool("password-stdin", false, "Passwort von der Standardeingabe lesen")
	source := flags.String("source", "all", "mysign, admin oder all authentifizieren")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := secrets.ValidateProfile(*profile); err != nil {
		return err
	}
	if *source != "mysign" && *source != "admin" && *source != "all" {
		return errors.New("--source muss mysign, admin oder all sein")
	}
	if *partner == "" || *username == "" {
		return errors.New("--partner und --username sind erforderlich")
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
	var (
		mySignSession secrets.Session
		adminSession  secrets.AdminSession
	)
	if *source == "mysign" || *source == "all" {
		client, err := transport.New(&http.Client{})
		if err != nil {
			return err
		}
		mySignSession, err = client.Login(ctx, credentials)
		if err != nil {
			return err
		}
	}
	if *source == "admin" || *source == "all" {
		client, err := admin.New(&http.Client{})
		if err != nil {
			return err
		}
		adminSession, err = client.Login(ctx, credentials)
		if err != nil {
			return err
		}
	}

	secretStore := secrets.NewKeyringStore()
	if err := secretStore.SaveCredentials(*profile, credentials); err != nil {
		return err
	}
	if *source == "mysign" || *source == "all" {
		if err := secretStore.SaveMySignSession(*profile, mySignSession); err != nil {
			return err
		}
	}
	if *source == "admin" || *source == "all" {
		if err := secretStore.SaveAdminSession(*profile, adminSession); err != nil {
			return err
		}
	}
	return output.Write(stdout, struct {
		Status  string `json:"status"`
		Profile string `json:"profile"`
		Source  string `json:"source"`
	}{Status: "authenticated", Profile: *profile, Source: *source}, "json")
}

func authStatus(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth status", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	_, credentialErr := secretStore.LoadCredentials(*profile)
	_, mySignSessionErr := secretStore.LoadMySignSession(*profile)
	_, adminSessionErr := secretStore.LoadAdminSession(*profile)
	if credentialErr != nil && !secrets.IsNotFound(credentialErr) {
		return credentialErr
	}
	if mySignSessionErr != nil && !secrets.IsNotFound(mySignSessionErr) {
		return mySignSessionErr
	}
	if adminSessionErr != nil && !secrets.IsNotFound(adminSessionErr) {
		return adminSessionErr
	}
	return output.Write(stdout, struct {
		Profile             string `json:"profile"`
		Configured          bool   `json:"configured"`
		MySignSessionCached bool   `json:"mysign_session_cached"`
		AdminSessionCached  bool   `json:"admin_session_cached"`
	}{
		Profile:             *profile,
		Configured:          credentialErr == nil,
		MySignSessionCached: mySignSessionErr == nil,
		AdminSessionCached:  adminSessionErr == nil,
	}, "json")
}

func authLogout(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
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
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("Profil %q ist nicht konfiguriert; führe myyolo auth login aus", *profile)
		}
		return err
	}
	session, err := secretStore.LoadMySignSession(*profile)
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
	if err := secretStore.SaveMySignSession(*profile, nextSession); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("normalisierten Snapshot hashen: %w", err)
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

func syncAdmin(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("sync admin", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	delay := flags.Duration("delay", admin.DefaultDelay, "Mindestverzögerung zwischen Admin-Anfragen")
	requestBudget := flags.Int(
		"request-budget",
		admin.DefaultFetchBudget,
		"maximale Anfragen inklusive Login und Relogin",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *delay < admin.DefaultDelay {
		return fmt.Errorf("--delay muss mindestens %s betragen", admin.DefaultDelay)
	}
	if *requestBudget < 1 || *requestBudget > admin.DefaultFetchBudget {
		return fmt.Errorf(
			"--request-budget muss für sync admin zwischen 1 und %d liegen",
			admin.DefaultFetchBudget,
		)
	}
	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("Profil %q ist nicht konfiguriert; führe myyolo auth login aus", *profile)
		}
		return err
	}
	session, err := secretStore.LoadAdminSession(*profile)
	if err != nil && !secrets.IsNotFound(err) {
		return err
	}
	client, err := admin.New(
		&http.Client{},
		admin.WithDelay(*delay),
		admin.WithRequestBudget(*requestBudget),
	)
	if err != nil {
		return err
	}
	page, nextSession, err := client.FetchAttendanceHome(ctx, credentials, session)
	if err != nil {
		return err
	}
	if err := secretStore.SaveAdminSession(*profile, nextSession); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ImportAdminPage(ctx, page)
	if err != nil {
		return err
	}
	return output.Write(stdout, result, "json")
}

func discoverAdmin(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("discover admin", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	delay := flags.Duration("delay", admin.DefaultDelay, "Mindestverzögerung zwischen Admin-Anfragen")
	requestBudget := flags.Int(
		"request-budget",
		admin.MaxRequestBudget,
		"maximale Anfragen inklusive Login und Relogin",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *delay < admin.DefaultDelay {
		return fmt.Errorf("--delay muss mindestens %s betragen", admin.DefaultDelay)
	}
	if *requestBudget < 1 || *requestBudget > admin.MaxRequestBudget {
		return fmt.Errorf(
			"--request-budget muss für discover admin zwischen 1 und %d liegen",
			admin.MaxRequestBudget,
		)
	}
	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("Profil %q ist nicht konfiguriert; führe myyolo auth login aus", *profile)
		}
		return err
	}
	session, err := secretStore.LoadAdminSession(*profile)
	if err != nil && !secrets.IsNotFound(err) {
		return err
	}
	client, err := admin.New(
		&http.Client{},
		admin.WithDelay(*delay),
		admin.WithRequestBudget(*requestBudget),
	)
	if err != nil {
		return err
	}
	pages, nextSession, err := client.Discover(ctx, credentials, session)
	if err != nil {
		return err
	}
	if err := secretStore.SaveAdminSession(*profile, nextSession); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result := struct {
		Pages   int                       `json:"pages"`
		Records int                       `json:"records"`
		Imports []store.AdminImportResult `json:"imports"`
	}{Pages: len(pages)}
	for _, page := range pages {
		imported, importErr := db.ImportAdminPage(ctx, page)
		if importErr != nil {
			return importErr
		}
		result.Records += imported.Records
		result.Imports = append(result.Imports, imported)
	}
	return output.Write(stdout, result, "json")
}

func authCheck(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth check", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	source := flags.String("source", "all", "mysign, admin oder all prüfen")
	forceRelogin := flags.Bool(
		"force-relogin",
		false,
		"zwischengespeicherte Sitzungen ignorieren und einen autonomen Login-Ablauf prüfen",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *source != "mysign" && *source != "admin" && *source != "all" {
		return errors.New("--source muss mysign, admin oder all sein")
	}
	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("Profil %q ist nicht konfiguriert; führe myyolo auth login aus", *profile)
		}
		return err
	}
	result := struct {
		Profile       string `json:"profile"`
		MySign        string `json:"mysign,omitempty"`
		Admin         string `json:"admin,omitempty"`
		MySignMembers int    `json:"mysign_members,omitempty"`
		AdminTables   int    `json:"admin_tables,omitempty"`
		AdminRecords  int    `json:"admin_records,omitempty"`
		AdminSchema   string `json:"admin_schema_fingerprint,omitempty"`
		ForcedRelogin bool   `json:"forced_relogin"`
	}{Profile: *profile, ForcedRelogin: *forceRelogin}

	if *source == "mysign" || *source == "all" {
		var session secrets.Session
		if !*forceRelogin {
			session, err = secretStore.LoadMySignSession(*profile)
			if err != nil && !secrets.IsNotFound(err) {
				return err
			}
		}
		client, clientErr := transport.New(&http.Client{})
		if clientErr != nil {
			return clientErr
		}
		snapshot, nextSession, fetchErr := client.Fetch(ctx, credentials, session)
		if fetchErr != nil {
			return fetchErr
		}
		if saveErr := secretStore.SaveMySignSession(*profile, nextSession); saveErr != nil {
			return saveErr
		}
		result.MySign = "ok"
		result.MySignMembers = len(snapshot.Members)
	}
	if *source == "admin" || *source == "all" {
		var session secrets.AdminSession
		if !*forceRelogin {
			session, err = secretStore.LoadAdminSession(*profile)
			if err != nil && !secrets.IsNotFound(err) {
				return err
			}
		}
		client, clientErr := admin.New(&http.Client{})
		if clientErr != nil {
			return clientErr
		}
		page, nextSession, fetchErr := client.FetchAttendanceHome(ctx, credentials, session)
		if fetchErr != nil {
			return fetchErr
		}
		if saveErr := secretStore.SaveAdminSession(*profile, nextSession); saveErr != nil {
			return saveErr
		}
		result.Admin = "ok"
		result.AdminTables = len(page.Tables)
		for _, table := range page.Tables {
			result.AdminRecords += len(table.Rows)
		}
		result.AdminSchema = page.Metadata.Fingerprint
	}
	return output.Write(stdout, result, "json")
}

func initDB(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("db init", flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
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

func dbStatus(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("db status", flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	format := flags.String("format", "json", "table, json oder csv")
	if err := flags.Parse(args); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	status, err := db.Status(ctx)
	if err != nil {
		return err
	}
	return output.Write(stdout, status, *format)
}

func doctor(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	profile := flags.String("profile", "default", "Zugangsdaten-Profil")
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secretStore := secrets.NewKeyringStore()
	_, credentialErr := secretStore.LoadCredentials(*profile)
	_, mySignSessionErr := secretStore.LoadMySignSession(*profile)
	_, adminSessionErr := secretStore.LoadAdminSession(*profile)
	for _, err := range []error{credentialErr, mySignSessionErr, adminSessionErr} {
		if err != nil && !secrets.IsNotFound(err) {
			return err
		}
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	database, err := db.Status(ctx)
	if err != nil {
		return err
	}
	return output.Write(stdout, struct {
		Status              string               `json:"status"`
		Profile             string               `json:"profile"`
		Credentials         bool                 `json:"credentials_configured"`
		MySignSessionCached bool                 `json:"mysign_session_cached"`
		AdminSessionCached  bool                 `json:"admin_session_cached"`
		Database            store.DatabaseStatus `json:"database"`
	}{
		Status:              "ok",
		Profile:             *profile,
		Credentials:         credentialErr == nil,
		MySignSessionCached: mySignSessionErr == nil,
		AdminSessionCached:  adminSessionErr == nil,
		Database:            database,
	}, "json")
}

func importMySign(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("import mysign", flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	filePath := flags.String("file", "", "mySIGN-GetListData-JSON-Datei")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file ist erforderlich")
	}
	data, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("Eingabe lesen: %w", err)
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
	dbPath := flags.String("db", defaultDBPath(), "Pfad zur SQLite-Datenbank")
	format := flags.String("format", "table", "table, json oder csv")
	asOfValue := flags.String("as-of", "", "Auswertungszeitpunkt im RFC3339-Format (Standard: jetzt)")
	route := flags.String("route", "", "Admin-Datensätze auf eine exakte Route begrenzen")
	includePersonalData := flags.Bool(
		"include-personal-data",
		false,
		"Mitgliedsnamen und -kennungen in lokaler Ausgabe erlauben",
	)
	includeHealthData := flags.Bool(
		"include-health-data",
		false,
		"Reha-, Präventions- und Verordnungsdaten in lokaler Ausgabe erlauben",
	)
	includeFinancialData := flags.Bool(
		"include-financial-data",
		false,
		"Bank- und Abrechnungsdaten in lokaler Ausgabe erlauben",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	asOf := time.Now()
	if *asOfValue != "" {
		asOf, err = time.Parse(time.RFC3339, *asOfValue)
		if err != nil {
			return fmt.Errorf("--as-of muss RFC3339 sein: %w", err)
		}
	}

	var report any
	switch reportName {
	case "summary":
		report, err = db.SummaryAt(ctx, asOf)
	case "courses":
		report, err = db.CoursesAt(ctx, asOf)
	case "days":
		report, err = db.DaysAt(ctx, asOf)
	case "hours":
		report, err = db.HoursAt(ctx, asOf)
	case "sessions":
		report, err = db.SessionsAt(ctx, asOf)
	case "members":
		if !*includePersonalData {
			return errors.New("Mitgliederbericht erfordert --include-personal-data")
		}
		report, err = db.MembersAt(ctx, asOf)
	case "admin-capabilities":
		report, err = db.AdminCapabilities(ctx)
	case "admin-reha-hours":
		report, err = db.AdminRehaHours(ctx)
	case "admin-course-months":
		report, err = db.AdminCourseMonths(ctx)
	case "admin-missing-signatures":
		report, err = db.AdminMissingSignatures(ctx)
	case "admin-reha-attendance":
		if !*includePersonalData {
			return errors.New("Admin-Reha-Anwesenheitsbericht erfordert --include-personal-data")
		}
		report, err = db.AdminRehaAttendanceDetails(ctx)
	case "admin-missing-signature-members":
		if !*includePersonalData {
			return errors.New("Admin-Bericht zu fehlenden Unterschriften erfordert --include-personal-data")
		}
		report, err = db.AdminMissingSignatureDetails(ctx)
	case "admin-records":
		if !*includePersonalData {
			return errors.New("Admin-Datensatzbericht erfordert --include-personal-data")
		}
		scope := dataScopeFlags{
			personal:  includePersonalData,
			health:    includeHealthData,
			financial: includeFinancialData,
		}
		if *route == "" {
			if !*includeHealthData || !*includeFinancialData {
				return errors.New(
					"ungefilterte Admin-Datensätze erfordern --include-personal-data, " +
						"--include-health-data und --include-financial-data",
				)
			}
		} else if sensitivity, ok := readcatalog.SensitivityForStoredRoute(*route); ok {
			if err := scope.authorize(sensitivity); err != nil {
				return err
			}
		} else if !*includeHealthData || !*includeFinancialData {
			return errors.New("eine unklassifizierte Admin-Route erfordert alle data-scope-Flags")
		}
		report, err = db.AdminRecords(ctx, *route)
	default:
		return fmt.Errorf("unbekannter Bericht %q", reportName)
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
			return "", fmt.Errorf("Passwort lesen: %w", err)
		}
		if len(data) > 4096 {
			return "", errors.New("Passworteingabe ist zu lang")
		}
		password := strings.TrimRight(string(data), "\r\n")
		if password == "" {
			return "", errors.New("Passwort ist leer")
		}
		return password, nil
	}
	if configured := os.Getenv("MYYOLO_PASSWORD"); configured != "" {
		if len(configured) > 4096 {
			return "", errors.New("MYYOLO_PASSWORD ist zu lang")
		}
		return configured, nil
	}
	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("interaktive Passwortabfrage erfordert ein Terminal; nutze --password-stdin")
	}
	if _, err := fmt.Fprint(stdout, "Passwort: "); err != nil {
		return "", err
	}
	data, err := term.ReadPassword(int(file.Fd()))
	if _, printErr := fmt.Fprintln(stdout); err == nil {
		err = printErr
	}
	if err != nil {
		return "", fmt.Errorf("Passwort lesen: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("Passwort ist leer")
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
