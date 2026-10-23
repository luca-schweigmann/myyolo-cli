package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

type prescriptionMetricOutput struct {
	Capability            string `json:"capability"`
	Metric                string `json:"metric"`
	Unit                  string `json:"unit"`
	Count                 int    `json:"count"`
	CountedRows           int    `json:"counted_rows"`
	SkippedRows           int    `json:"skipped_rows"`
	Coverage              string `json:"coverage"`
	ObservedAt            string `json:"observed_at"`
	SchemaFingerprint     string `json:"schema_fingerprint"`
	ContextFrom           string `json:"context_from,omitempty"`
	ContextTo             string `json:"context_to,omitempty"`
	SourceScopeStatus     string `json:"source_scope_status"`
	RequestContextBinding string `json:"request_context_binding"`
}

func printPrescriptionMetric(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report prescription-metric", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private SQLite database")
	capability := flags.String(
		"capability",
		"",
		"reha-prescriptions or reha-prescription-summary",
	)
	contextFile := flags.String(
		"context-file",
		"",
		"hash-bound request context receipt for a filtered summary",
	)
	format := flags.String("format", "table", "table, json or csv")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("PRESCRIPTION_INVALID_ARGUMENTS: invalid options")
	}
	if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*capability) == "" {
		return errors.New("PRESCRIPTION_INVALID_ARGUMENTS: required options are missing")
	}
	if *format != "table" && *format != "json" && *format != "csv" {
		return errors.New("PRESCRIPTION_INVALID_ARGUMENTS: unsupported output format")
	}
	switch *capability {
	case "reha-prescriptions":
		if *contextFile != "" {
			return errors.New("PRESCRIPTION_INVALID_ARGUMENTS: active observation does not accept a date context")
		}
	case "reha-prescription-summary":
		if strings.TrimSpace(*contextFile) == "" {
			return errors.New("PRESCRIPTION_CONTEXT_REQUIRED: filtered summary requires a context receipt")
		}
	default:
		return errors.New("PRESCRIPTION_UNSUPPORTED: capability is not supported by this report")
	}

	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("PRESCRIPTION_DATABASE_FAILED: database cannot be opened read-only")
	}
	defer db.Close()
	metric, err := db.PrescriptionMetric(ctx, *capability)
	if err != nil {
		return errors.New("PRESCRIPTION_REPORT_FAILED: aggregate cannot be created")
	}
	result := prescriptionMetricOutput{
		Capability:            metric.Capability,
		Metric:                metric.Metric,
		Unit:                  metric.Unit,
		Count:                 metric.Count,
		CountedRows:           metric.CountedRows,
		SkippedRows:           metric.SkippedRows,
		Coverage:              metric.Coverage,
		ObservedAt:            metric.ObservedAt,
		SchemaFingerprint:     metric.SchemaFingerprint,
		SourceScopeStatus:     "external_operator_scope_required",
		RequestContextBinding: "not_applicable_current_observation",
	}
	if *capability == "reha-prescription-summary" {
		receipt, err := store.ReadPrescriptionContextReceipt(*contextFile)
		if err != nil {
			return errors.New("PRESCRIPTION_CONTEXT_FAILED: context receipt is invalid")
		}
		if err := store.ValidatePrescriptionContext(*dbPath, receipt, metric); err != nil {
			return errors.New("PRESCRIPTION_CONTEXT_FAILED: context receipt does not match report")
		}
		result.ContextFrom = receipt.From
		result.ContextTo = receipt.To
		result.RequestContextBinding = receipt.BindingBasis
	}
	if err := output.Write(stdout, result, *format); err != nil {
		return errors.New("PRESCRIPTION_OUTPUT_FAILED: report cannot be written")
	}
	return nil
}

// printPrescriptionObservation writes the latest "Resttermine" collect as unit
// states per prescription. Member numbers leave this process only as
// HMAC(key, member number "|" end date); the key file stays with the caller.
func printPrescriptionObservation(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("report prescription-observation", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "private SQLite database")
	keyFile := flags.String("key-file", "", "absolute 0600 file with at least 32 key bytes")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*keyFile) == "" {
		return errors.New("PRESCRIPTION_INVALID_ARGUMENTS: --db and --key-file are required")
	}
	key, err := readObservationKey(*keyFile)
	if err != nil {
		return err
	}
	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return errors.New("PRESCRIPTION_DATABASE_FAILED: database cannot be opened read-only")
	}
	defer db.Close()
	observation, err := db.PrescriptionObservation(ctx, key)
	if err != nil {
		return fmt.Errorf("PRESCRIPTION_REPORT_FAILED: %v", err)
	}
	if err := output.Write(stdout, observation, "json"); err != nil {
		return errors.New("PRESCRIPTION_OUTPUT_FAILED: report cannot be written")
	}
	return nil
}

func readObservationKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if !filepath.IsAbs(path) || err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, errors.New("PRESCRIPTION_KEY_FAILED: key file must be an absolute regular 0600 file")
	}
	key, err := os.ReadFile(path)
	if err != nil || len(key) < 32 {
		return nil, errors.New("PRESCRIPTION_KEY_FAILED: key file must hold at least 32 bytes")
	}
	return key, nil
}
