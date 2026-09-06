package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

func printRehaInactivity(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report reha-inactivity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private Snapshot-SQLite-Datenbank")
	scopeFile := flags.String("scope-file", "", "Scope-Receipt-JSON zum Snapshot")
	asOfValue := flags.String("as-of", "", "verbindlicher Auswertungszeitpunkt im RFC3339-Format")
	location := flags.String("location", "", "exakter Standortschlüssel aus dem Scope-Receipt")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("REHA_INACTIVITY_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*scopeFile) == "" ||
		strings.TrimSpace(*asOfValue) == "" || strings.TrimSpace(*location) == "" {
		return errors.New("REHA_INACTIVITY_INVALID_ARGUMENTS: Pflichtangaben fehlen")
	}
	asOf, err := time.Parse(time.RFC3339, *asOfValue)
	if err != nil {
		return errors.New("REHA_INACTIVITY_INVALID_ARGUMENTS: Auswertungszeitpunkt ist ungültig")
	}
	if _, err := store.ComputeRehaInactivityThresholds(asOf); err != nil {
		return errors.New("REHA_INACTIVITY_INVALID_ARGUMENTS: Auswertungszeitpunkt ist ungültig")
	}
	receipt, err := store.ReadSnapshotReceipt(*scopeFile)
	if err != nil {
		return errors.New("REHA_INACTIVITY_SCOPE_FAILED: Scope-Receipt ist ungültig")
	}
	if receipt.Location != *location {
		return errors.New("REHA_INACTIVITY_SCOPE_MISMATCH: Standort stimmt nicht mit Scope-Receipt überein")
	}
	if err := store.ValidateReceiptForSnapshot(receipt, *dbPath, *location); err != nil {
		return errors.New("REHA_INACTIVITY_SCOPE_FAILED: Scope-Receipt passt nicht zum Report")
	}
	if err := store.VerifySnapshotHash(*dbPath, receipt.SnapshotSHA256); err != nil {
		return errors.New("REHA_INACTIVITY_SNAPSHOT_FAILED: Snapshot-Prüfung fehlgeschlagen")
	}
	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("REHA_INACTIVITY_SNAPSHOT_FAILED: Snapshot kann nicht gelesen werden")
	}
	defer db.Close()
	report, err := db.RehaInactivity(ctx, receipt.Source, receipt.Location, asOf, receipt.SnapshotSHA256)
	if err != nil {
		return errors.New("REHA_INACTIVITY_REPORT_FAILED: Capability-Report konnte nicht erstellt werden")
	}
	report.SnapshotAt = receipt.CompletedAt
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return errors.New("REHA_INACTIVITY_OUTPUT_FAILED: Ausgabe konnte nicht geschrieben werden")
	}
	return nil
}
