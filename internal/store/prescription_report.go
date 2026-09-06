package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
)

const prescriptionContextVersion = 1

type PrescriptionMetric struct {
	admin.PrescriptionAggregate
	ObservedAt        string `json:"observed_at"`
	SchemaFingerprint string `json:"schema_fingerprint"`
}

// PrescriptionContextReceipt binds a filtered capability observation to the
// exact private database bytes used for an offline report.
type PrescriptionContextReceipt struct {
	Version        int    `json:"version"`
	Capability     string `json:"capability"`
	From           string `json:"from"`
	To             string `json:"to"`
	DatabaseSHA256 string `json:"database_sha256"`
	ObservedAt     string `json:"observed_at"`
	CapturedAt     string `json:"captured_at"`
	BindingBasis   string `json:"binding_basis"`
}

func (store *ReadOnlyStore) PrescriptionMetric(
	ctx context.Context,
	capability string,
) (PrescriptionMetric, error) {
	if store == nil || store.db == nil {
		return PrescriptionMetric{}, errors.New("read-only database is not open")
	}
	if capability != "reha-prescriptions" && capability != "reha-prescription-summary" {
		return PrescriptionMetric{}, fmt.Errorf("unsupported prescription metric capability %q", capability)
	}
	route := readcatalog.RouteKey(capability)

	var encodedHeaders, encodedRowCounts, observedAt, fingerprint string
	if err := store.db.QueryRowContext(ctx, `
		SELECT table_headers_json, table_rows_json, last_observed_at, schema_fingerprint
		FROM admin_capabilities WHERE route=?`, route,
	).Scan(&encodedHeaders, &encodedRowCounts, &observedAt, &fingerprint); err != nil {
		return PrescriptionMetric{}, fmt.Errorf("read prescription capability metadata: %w", err)
	}
	var headers [][]string
	if err := json.Unmarshal([]byte(encodedHeaders), &headers); err != nil {
		return PrescriptionMetric{}, errors.New("decode prescription capability headers")
	}
	var rowCounts []int
	if err := json.Unmarshal([]byte(encodedRowCounts), &rowCounts); err != nil || len(rowCounts) != len(headers) {
		return PrescriptionMetric{}, errors.New("decode prescription capability row counts")
	}
	page := admin.Page{Metadata: admin.PageMetadata{Route: route}}
	page.Tables = make([]admin.Table, len(headers))
	for index := range headers {
		page.Tables[index].Headers = headers[index]
		if len(headers[index]) == 0 {
			continue
		}
		rows, err := store.db.QueryContext(ctx, `
			SELECT values_json FROM admin_records
			WHERE route=? AND table_index=? AND last_observed_at=?
			ORDER BY record_hash`, route, index, observedAt)
		if err != nil {
			return PrescriptionMetric{}, fmt.Errorf("read prescription capability rows: %w", err)
		}
		for rows.Next() {
			var encoded string
			if err := rows.Scan(&encoded); err != nil {
				_ = rows.Close()
				return PrescriptionMetric{}, errors.New("scan prescription capability row")
			}
			row, err := restoreAdminRow(headers[index], encoded)
			if err != nil {
				_ = rows.Close()
				return PrescriptionMetric{}, err
			}
			page.Tables[index].Rows = append(page.Tables[index].Rows, row)
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if rowErr != nil {
			return PrescriptionMetric{}, errors.New("read prescription capability rows")
		}
		if closeErr != nil {
			return PrescriptionMetric{}, errors.New("close prescription capability rows")
		}
	}
	if err := validatePrescriptionTableReconstruction(capability, page.Tables, rowCounts); err != nil {
		return PrescriptionMetric{}, err
	}

	aggregate, err := admin.AggregatePrescriptionPage(page)
	if err != nil {
		return PrescriptionMetric{}, err
	}
	return PrescriptionMetric{
		PrescriptionAggregate: aggregate,
		ObservedAt:            observedAt,
		SchemaFingerprint:     fingerprint,
	}, nil
}

func validatePrescriptionTableReconstruction(
	capability string,
	tables []admin.Table,
	rowCounts []int,
) error {
	var expectedHeaders []string
	switch capability {
	case "reha-prescriptions":
		expectedHeaders = []string{
			"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", "",
		}
	case "reha-prescription-summary":
		expectedHeaders = []string{"", "Arzt", "Menge"}
	default:
		return fmt.Errorf("unsupported prescription metric capability %q", capability)
	}

	match := -1
	for index, table := range tables {
		if !equalPrescriptionHeaders(table.Headers, expectedHeaders) {
			continue
		}
		if match >= 0 {
			return errors.New("prescription capability contains more than one matching table")
		}
		match = index
	}
	if match < 0 {
		return errors.New("prescription capability expected table headers are missing")
	}
	if rowCounts[match] != len(tables[match].Rows) {
		return errors.New("prescription capability rows cannot be reconstructed completely")
	}
	return nil
}

func equalPrescriptionHeaders(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func restoreAdminRow(headers []string, encoded string) ([]string, error) {
	values := make(map[string]string)
	decoder := json.NewDecoder(strings.NewReader(encoded))
	if err := decoder.Decode(&values); err != nil {
		return nil, errors.New("decode prescription capability row")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("decode prescription capability row")
	}
	row := make([]string, 0, len(headers))
	used := make(map[string]int, len(headers))
	for index, header := range headers {
		key := fmt.Sprintf("column_%d", index+1)
		if header != "" {
			key = header
		}
		used[key]++
		if used[key] > 1 {
			key = fmt.Sprintf("%s_%d", key, used[key])
		}
		value, ok := values[key]
		if !ok {
			break
		}
		row = append(row, value)
	}
	if len(row) != len(values) {
		return nil, errors.New("prescription capability row does not match stored headers")
	}
	return row, nil
}

func ReadPrescriptionContextReceipt(path string) (PrescriptionContextReceipt, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return PrescriptionContextReceipt{}, errors.New("prescription context receipt path must be canonical absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return PrescriptionContextReceipt{}, fmt.Errorf("stat prescription context receipt: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return PrescriptionContextReceipt{}, errors.New("prescription context receipt must be a regular 0600 file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PrescriptionContextReceipt{}, fmt.Errorf("read prescription context receipt: %w", err)
	}
	var receipt PrescriptionContextReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return PrescriptionContextReceipt{}, errors.New("decode prescription context receipt")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return PrescriptionContextReceipt{}, errors.New("decode prescription context receipt")
	}
	if err := validatePrescriptionContextReceipt(receipt); err != nil {
		return PrescriptionContextReceipt{}, err
	}
	return receipt, nil
}

func ValidatePrescriptionContext(
	databasePath string,
	receipt PrescriptionContextReceipt,
	metric PrescriptionMetric,
) error {
	if metric.Capability != readcatalog.RouteKey(receipt.Capability) {
		return errors.New("prescription context capability does not match report")
	}
	if metric.ObservedAt != receipt.ObservedAt {
		return errors.New("prescription context observation does not match report")
	}
	actual, err := fileSHA256(databasePath)
	if err != nil {
		return errors.New("hash prescription report database")
	}
	if actual != receipt.DatabaseSHA256 {
		return errors.New("prescription context database hash does not match report")
	}
	return nil
}

func validatePrescriptionContextReceipt(receipt PrescriptionContextReceipt) error {
	if receipt.Version != prescriptionContextVersion {
		return errors.New("unsupported prescription context receipt version")
	}
	if receipt.Capability != "reha-prescription-summary" {
		return errors.New("unsupported prescription context capability")
	}
	if receipt.BindingBasis != "operator_captured_hash_bound_context_v1" {
		return errors.New("unsupported prescription context binding basis")
	}
	from, err := time.Parse("2006-01-02", receipt.From)
	if err != nil {
		return errors.New("invalid prescription context date range")
	}
	to, err := time.Parse("2006-01-02", receipt.To)
	if err != nil || from.After(to) {
		return errors.New("invalid prescription context date range")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return errors.New("prescription context date range exceeds source limit")
	}
	if !validLowerHex(receipt.DatabaseSHA256, 64) {
		return errors.New("invalid prescription context database hash")
	}
	observed, err := time.Parse(time.RFC3339Nano, receipt.ObservedAt)
	if err != nil {
		return errors.New("invalid prescription context observation time")
	}
	captured, err := time.Parse(time.RFC3339Nano, receipt.CapturedAt)
	if err != nil || captured.Before(observed) {
		return errors.New("invalid prescription context capture time")
	}
	return nil
}
