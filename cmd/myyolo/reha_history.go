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

func printRehaHistory(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report reha-history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private Snapshot-SQLite-Datenbank")
	scopeFile := flags.String("scope-file", "", "Scope-Receipt-JSON zum Snapshot")
	fromValue := flags.String("from", "", "erster Kalendertag (YYYY-MM-DD), inklusive")
	toValue := flags.String("to", "", "letzter Kalendertag (YYYY-MM-DD), inklusive")
	location := flags.String("location", "", "exakter Standortschlüssel aus dem Scope-Receipt")
	schema := flags.String("schema-version", "v1", "Ausgabevertrag: v1 oder v2 (Kurslabel und Eintragungen)")
	asOfValue := flags.String("as-of", "", "verbindlicher Auswertungszeitpunkt im RFC3339-Format (Standard: jetzt)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: ungültige Optionen")
	}
	if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*scopeFile) == "" ||
		strings.TrimSpace(*fromValue) == "" || strings.TrimSpace(*toValue) == "" ||
		strings.TrimSpace(*location) == "" {
		return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: Pflichtangaben fehlen")
	}
	if *schema != "v1" && *schema != "v2" {
		return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: Schema muss v1 oder v2 sein")
	}
	from, err := parseBerlinDate(*fromValue)
	if err != nil {
		return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: Datumsbereich ist ungültig")
	}
	to, err := parseBerlinDate(*toValue)
	if err != nil || from.After(to) {
		return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: Datumsbereich ist ungültig")
	}
	asOf := time.Now().UTC()
	if *asOfValue != "" {
		asOf, err = time.Parse(time.RFC3339, *asOfValue)
		if err != nil {
			return errors.New("REHA_HISTORY_INVALID_ARGUMENTS: Auswertungszeitpunkt ist ungültig")
		}
	}

	receipt, err := store.ReadSnapshotReceipt(*scopeFile)
	if err != nil {
		return errors.New("REHA_HISTORY_SCOPE_FAILED: Scope-Receipt ist ungültig")
	}
	if receipt.Location != *location {
		return errors.New("REHA_HISTORY_SCOPE_MISMATCH: Standort stimmt nicht mit Scope-Receipt überein")
	}
	if err := store.ValidateReceiptForSnapshot(receipt, *dbPath, *location); err != nil {
		return errors.New("REHA_HISTORY_SCOPE_FAILED: Scope-Receipt passt nicht zum Report")
	}
	snapshotAt, err := time.Parse(time.RFC3339Nano, receipt.CompletedAt)
	if err != nil || asOf.Before(snapshotAt) {
		return errors.New("REHA_HISTORY_SCOPE_FAILED: as-of liegt vor dem Abschlusszeitpunkt des Snapshots")
	}
	if err := store.VerifySnapshotHash(*dbPath, receipt.SnapshotSHA256); err != nil {
		return errors.New("REHA_HISTORY_SNAPSHOT_FAILED: Snapshot-Prüfung fehlgeschlagen")
	}
	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("REHA_HISTORY_SNAPSHOT_FAILED: Snapshot kann nicht gelesen werden")
	}
	defer db.Close()
	report, err := db.RehaHistoryWithSchema(
		ctx,
		receipt.Source,
		receipt.Location,
		from,
		to,
		asOf,
		receipt.SnapshotSHA256,
		"reha-history-observations."+*schema,
	)
	if err != nil {
		return errors.New("REHA_HISTORY_REPORT_FAILED: Reha-History-Report konnte nicht erstellt werden")
	}
	report.SnapshotAt = receipt.CompletedAt
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return errors.New("REHA_HISTORY_OUTPUT_FAILED: Ausgabe konnte nicht geschrieben werden")
	}
	return nil
}
