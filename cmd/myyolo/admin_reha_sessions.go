package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
	"io"
	"time"
)

func printAdminRehaSessions(ctx context.Context, args []string, stdout io.Writer) error {
	return printAdminReha(ctx, args, stdout, false)
}
func printAdminRehaRanges(ctx context.Context, args []string, stdout io.Writer) error {
	return printAdminReha(ctx, args, stdout, true)
}
func printAdminReha(ctx context.Context, args []string, stdout io.Writer, ranges bool) error {
	flags := flag.NewFlagSet("report admin-reha-sessions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private Admin-Archivdatenbank")
	fromValue := flags.String("from", "", "erster Tag inklusive")
	toValue := flags.String("to", "", "letzter Tag inklusive")
	asOfValue := flags.String("as-of", "", "RFC3339-Auswertungszeitpunkt")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *dbPath == "" {
		return errors.New("ADMIN_REHA_INVALID_ARGUMENTS")
	}
	from, err := parseBerlinDate(*fromValue)
	if err != nil {
		return errors.New("ADMIN_REHA_INVALID_ARGUMENTS")
	}
	to, err := parseBerlinDate(*toValue)
	if err != nil || from.After(to) {
		return errors.New("ADMIN_REHA_INVALID_ARGUMENTS")
	}
	asOf := time.Now().UTC()
	if *asOfValue != "" {
		asOf, err = time.Parse(time.RFC3339, *asOfValue)
		if err != nil {
			return errors.New("ADMIN_REHA_INVALID_ARGUMENTS")
		}
	}
	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("ADMIN_REHA_ARCHIVE_FAILED")
	}
	defer db.Close()
	var report any
	if ranges {
		report, err = db.AdminRehaRanges(ctx, from, to, asOf)
	} else {
		report, err = db.AdminRehaSessions(ctx, from, to, asOf)
	}
	if err != nil {
		return errors.New("ADMIN_REHA_REPORT_FAILED: source-native Daten oder Feldbelege fehlen")
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
