package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func snapshotDB(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("db snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceDB := flags.String("source-db", "", "bestehende Quell-SQLite-Datenbank")
	outputDB := flags.String("output-db", "", "neue private Snapshot-SQLite-Datenbank")
	scopeInput := flags.String("scope-input", "", "vorab geprüfte Source-/Standort-Scope-JSON")
	receipt := flags.String("receipt", "", "neue private Scope-Receipt-JSON")
	if err := flags.Parse(args); err != nil {
		return errors.New("REHA_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if flags.NArg() != 0 {
		return errors.New("REHA_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if strings.TrimSpace(*sourceDB) == "" || strings.TrimSpace(*outputDB) == "" ||
		strings.TrimSpace(*scopeInput) == "" || strings.TrimSpace(*receipt) == "" {
		return errors.New("REHA_INVALID_ARGUMENTS: Pflichtangaben fehlen")
	}
	result, err := store.CreateSnapshot(ctx, *sourceDB, *outputDB, *scopeInput, *receipt)
	if err != nil {
		return errors.New("REHA_SNAPSHOT_FAILED: Snapshot konnte nicht erstellt werden")
	}
	if err := output.Write(stdout, result, "json"); err != nil {
		return errors.New("REHA_OUTPUT_FAILED: Ausgabe konnte nicht geschrieben werden")
	}
	return nil
}

func printRehaSessions(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report reha-sessions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private Snapshot-SQLite-Datenbank")
	scopeFile := flags.String("scope-file", "", "Scope-Receipt-JSON zum Snapshot")
	fromValue := flags.String("from", "", "erster Kalendertag (YYYY-MM-DD), inklusive")
	toValue := flags.String("to", "", "letzter Kalendertag (YYYY-MM-DD), inklusive")
	location := flags.String("location", "", "exakter Standortschlüssel aus dem Scope-Receipt")
	asOfValue := flags.String("as-of", "", "Auswertungszeitpunkt im RFC3339-Format (Standard: jetzt)")
	format := flags.String("format", "table", "table, json oder csv")
	if err := flags.Parse(args); err != nil {
		return errors.New("REHA_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if flags.NArg() != 0 {
		return errors.New("REHA_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*scopeFile) == "" ||
		strings.TrimSpace(*fromValue) == "" || strings.TrimSpace(*toValue) == "" ||
		strings.TrimSpace(*location) == "" {
		return errors.New("REHA_INVALID_ARGUMENTS: Pflichtangaben fehlen")
	}
	if *format != "table" && *format != "json" && *format != "csv" {
		return errors.New("REHA_INVALID_ARGUMENTS: Ausgabeformat ist nicht unterstützt")
	}
	from, err := parseBerlinDate(*fromValue)
	if err != nil {
		return errors.New("REHA_INVALID_ARGUMENTS: Datumsbereich ist ungültig")
	}
	to, err := parseBerlinDate(*toValue)
	if err != nil {
		return errors.New("REHA_INVALID_ARGUMENTS: Datumsbereich ist ungültig")
	}
	if from.After(to) {
		return errors.New("REHA_INVALID_ARGUMENTS: Datumsbereich ist ungültig")
	}
	asOf := time.Now().UTC()
	if *asOfValue != "" {
		asOf, err = time.Parse(time.RFC3339, *asOfValue)
		if err != nil {
			return errors.New("REHA_INVALID_ARGUMENTS: Auswertungszeitpunkt ist ungültig")
		}
	}
	receipt, err := store.ReadSnapshotReceipt(*scopeFile)
	if err != nil {
		return errors.New("REHA_SCOPE_FAILED: Scope-Receipt ist ungültig")
	}
	if receipt.Location != *location {
		return errors.New("REHA_SCOPE_MISMATCH: Standort stimmt nicht mit Scope-Receipt überein")
	}
	if err := store.ValidateReceiptForSnapshot(receipt, *dbPath, *location); err != nil {
		return errors.New("REHA_SCOPE_FAILED: Scope-Receipt passt nicht zum Report")
	}
	if err := store.VerifySnapshotHash(*dbPath, receipt.SnapshotSHA256); err != nil {
		return errors.New("REHA_SNAPSHOT_FAILED: Snapshot-Prüfung fehlgeschlagen")
	}
	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("REHA_SNAPSHOT_FAILED: Snapshot kann nicht gelesen werden")
	}
	defer db.Close()
	report, err := db.RehaSessions(
		ctx,
		receipt.Source,
		receipt.Location,
		from,
		to,
		asOf,
		receipt.SnapshotSHA256,
	)
	if err != nil {
		return errors.New("REHA_REPORT_FAILED: Reha-Report konnte nicht erstellt werden")
	}
	report.SnapshotAt = receipt.CompletedAt
	if err := writeRehaSessionReport(stdout, report, *format); err != nil {
		return errors.New("REHA_OUTPUT_FAILED: Ausgabe konnte nicht geschrieben werden")
	}
	return nil
}

func parseBerlinDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location), nil
}

func writeRehaSessionReport(writer io.Writer, report store.RehaSessionReport, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "csv":
		return writeRehaCSV(writer, report)
	case "table":
		return writeRehaTable(writer, report)
	default:
		return errors.New("REHA_INVALID_ARGUMENTS: Ausgabeformat ist nicht unterstützt")
	}
}

func writeRehaTable(writer io.Writer, report store.RehaSessionReport) error {
	metadata := []struct {
		key   string
		value string
	}{
		{"requested_from", report.RequestedFrom},
		{"requested_to", report.RequestedTo},
		{"timezone", report.Timezone},
		{"as_of", report.AsOf},
		{"source", report.Source},
		{"location", report.Location},
		{"scope_basis", report.ScopeBasis},
		{"snapshot_sha256", report.SnapshotSHA256},
		{"data_as_of", report.DataAsOf},
		{"snapshot_at", report.SnapshotAt},
		{"report_version", fmt.Sprint(report.ReportVersion)},
		{"generated_at", report.GeneratedAt},
		{"import_observed_at", report.ImportObservedAt},
		{"observation_basis", report.ObservationBasis},
		{"participant_count_validation", report.ParticipantCountValidation},
		{"population_status", report.PopulationStatus},
		{"coverage_status", report.Coverage.Status},
		{"source_sessions", fmt.Sprint(report.Coverage.SourceSessions)},
		{"sessions_in_range", fmt.Sprint(report.Coverage.SessionsInRange)},
		{"invalid_date_rows", fmt.Sprint(report.Coverage.InvalidDateRows)},
		{"invalid_end_rows", fmt.Sprint(report.Coverage.InvalidEndRows)},
		{"observed_from", report.Coverage.ObservedFrom},
		{"observed_to", report.Coverage.ObservedTo},
		{"gaps", strings.Join(report.Coverage.Gaps, ";")},
	}
	for _, item := range metadata {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", tableCell(item.key), tableCell(item.value)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	headers := rehaRowHeaders()
	if _, err := fmt.Fprintln(table, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range report.Sessions {
		values := rehaRowValues(row)
		for index := range values {
			values[index] = tableCell(values[index])
		}
		if _, err := fmt.Fprintln(table, strings.Join(values, "\t")); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeRehaCSV(writer io.Writer, report store.RehaSessionReport) error {
	csvWriter := csv.NewWriter(writer)
	rowHeaders := rehaRowHeaders()
	metadataHeaders := []string{
		"report_requested_from",
		"report_requested_to",
		"report_timezone",
		"report_as_of",
		"report_source",
		"report_location",
		"report_scope_basis",
		"report_snapshot_sha256",
		"report_data_as_of",
		"coverage_status",
		"coverage_source_sessions",
		"coverage_sessions_in_range",
		"coverage_invalid_date_rows",
		"coverage_invalid_end_rows",
		"coverage_observed_from",
		"coverage_observed_to",
		"coverage_gaps",
		"report_version", "generated_at", "import_observed_at", "observation_basis", "participant_count_validation", "population_status", "snapshot_at",
	}
	if err := csvWriter.Write(append(append([]string{}, rowHeaders...), metadataHeaders...)); err != nil {
		return err
	}
	metadata := []string{
		report.RequestedFrom,
		report.RequestedTo,
		report.Timezone,
		report.AsOf,
		report.Source,
		report.Location,
		report.ScopeBasis,
		report.SnapshotSHA256,
		report.DataAsOf,
		report.Coverage.Status,
		fmt.Sprint(report.Coverage.SourceSessions),
		fmt.Sprint(report.Coverage.SessionsInRange),
		fmt.Sprint(report.Coverage.InvalidDateRows),
		fmt.Sprint(report.Coverage.InvalidEndRows),
		report.Coverage.ObservedFrom,
		report.Coverage.ObservedTo,
		strings.Join(report.Coverage.Gaps, ";"),
		fmt.Sprint(report.ReportVersion), report.GeneratedAt, report.ImportObservedAt, report.ObservationBasis, report.ParticipantCountValidation, report.PopulationStatus, report.SnapshotAt,
	}
	for index := range metadata {
		metadata[index] = csvCell(metadata[index])
	}
	if len(report.Sessions) == 0 {
		if err := csvWriter.Write(append(make([]string, len(rowHeaders)), metadata...)); err != nil {
			return err
		}
	} else {
		for _, row := range report.Sessions {
			values := append(rehaRowValues(row), metadata...)
			for index := range values {
				values[index] = csvCell(values[index])
			}
			if err := csvWriter.Write(values); err != nil {
				return err
			}
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func rehaRowHeaders() []string {
	return []string{
		"source",
		"location",
		"stable_session_id",
		"date",
		"time",
		"course",
		"participant_count_current_observation",
		"attendance_rows",
		"attended_flag_true",
		"signed_flag_true",
		"cancelled_flag_true",
		"attended_not_cancelled",
		"signed_attended_not_cancelled",
		"missing_signature_candidate",
		"non_attended_not_cancelled_candidate",
		"prescription_linked_rows",
		"prescription_unlinked_rows",
		"contradictory_flags",
	}
}

func rehaRowValues(row store.RehaSessionRow) []string {
	return []string{
		row.Source,
		row.Location,
		row.StableSessionID,
		row.Date,
		row.Time,
		row.Course,
		fmt.Sprint(row.ParticipantCountCurrentObservation),
		fmt.Sprint(row.AttendanceRows),
		fmt.Sprint(row.AttendedFlagTrue),
		fmt.Sprint(row.SignedFlagTrue),
		fmt.Sprint(row.CancelledFlagTrue),
		fmt.Sprint(row.AttendedNotCancelled),
		fmt.Sprint(row.SignedAttendedNotCancelled),
		fmt.Sprint(row.MissingSignatureCandidate),
		fmt.Sprint(row.NonAttendedNotCancelledCandidate),
		fmt.Sprint(row.PrescriptionLinkedRows),
		fmt.Sprint(row.PrescriptionUnlinkedRows),
		fmt.Sprint(row.ContradictoryFlags),
	}
}

func tableCell(value string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(value)
}

func csvCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	default:
		return value
	}
}
