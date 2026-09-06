package admin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	activeRehaPrescriptionsRoute = "capability:reha-prescriptions"
	rehaPrescriptionSummaryRoute = "capability:reha-prescription-summary"
)

// PrescriptionAggregate is intentionally limited to aggregate-safe counts.
// The request context (location and date range) remains the caller's evidence.
type PrescriptionAggregate struct {
	Capability  string `json:"capability"`
	Metric      string `json:"metric"`
	Unit        string `json:"unit"`
	Count       int    `json:"count"`
	CountedRows int    `json:"counted_rows"`
	SkippedRows int    `json:"skipped_rows"`
	Coverage    string `json:"coverage"`
}

// AggregatePrescriptionPage extracts only the two prescription aggregates
// whose response schemas have been observed. It rejects schema drift and
// malformed numeric cells instead of turning unknown input into zero.
func AggregatePrescriptionPage(page Page) (PrescriptionAggregate, error) {
	switch page.Metadata.Route {
	case activeRehaPrescriptionsRoute:
		return aggregateActiveRehaPrescriptions(page)
	case rehaPrescriptionSummaryRoute:
		return aggregateRehaPrescriptionSummary(page)
	default:
		return PrescriptionAggregate{}, fmt.Errorf(
			"unsupported prescription aggregate route %q",
			page.Metadata.Route,
		)
	}
}

func aggregateActiveRehaPrescriptions(page Page) (PrescriptionAggregate, error) {
	table, err := exactHeaderTable(page.Tables, []string{
		"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", "",
	})
	if err != nil {
		return PrescriptionAggregate{}, fmt.Errorf("active Reha prescriptions: %w", err)
	}

	seen := make(map[string]struct{})
	counted := 0
	skipped := 0
	for _, row := range table.Rows {
		if len(row) != len(table.Headers) || strings.TrimSpace(row[0]) == "" || strings.TrimSpace(row[1]) == "" {
			skipped++
			continue
		}
		prescriptionID := strings.TrimSpace(row[0])
		if _, duplicate := seen[prescriptionID]; duplicate {
			return PrescriptionAggregate{}, errors.New("duplicate prescription identifier")
		}
		seen[prescriptionID] = struct{}{}
		counted++
	}

	return PrescriptionAggregate{
		Capability:  activeRehaPrescriptionsRoute,
		Metric:      "active_reha_prescriptions",
		Unit:        "prescription_rows",
		Count:       counted,
		CountedRows: counted,
		SkippedRows: skipped,
		Coverage:    "schema_matched_rows_only",
	}, nil
}

func aggregateRehaPrescriptionSummary(page Page) (PrescriptionAggregate, error) {
	table, err := exactHeaderTable(page.Tables, []string{"", "Arzt", "Menge"})
	if err != nil {
		return PrescriptionAggregate{}, fmt.Errorf("Reha prescription summary: %w", err)
	}

	total := 0
	counted := 0
	skipped := 0
	for _, row := range table.Rows {
		if len(row) != len(table.Headers) || strings.TrimSpace(row[1]) == "" {
			skipped++
			continue
		}
		value := strings.TrimSpace(row[2])
		if value == "" || strings.Trim(value, "0123456789") != "" {
			return PrescriptionAggregate{}, errors.New("Menge is not a nonnegative integer")
		}
		amount, err := strconv.Atoi(value)
		if err != nil {
			return PrescriptionAggregate{}, errors.New("Menge is outside the supported integer range")
		}
		maxInt := int(^uint(0) >> 1)
		if amount > maxInt-total {
			return PrescriptionAggregate{}, errors.New("Menge total is outside the supported integer range")
		}
		total += amount
		counted++
	}

	return PrescriptionAggregate{
		Capability:  rehaPrescriptionSummaryRoute,
		Metric:      "reha_prescription_summary_total",
		Unit:        "prescriptions",
		Count:       total,
		CountedRows: counted,
		SkippedRows: skipped,
		Coverage:    "schema_matched_rows_only",
	}, nil
}

func exactHeaderTable(tables []Table, expected []string) (Table, error) {
	match := -1
	for index, table := range tables {
		if !equalStrings(table.Headers, expected) {
			continue
		}
		if match >= 0 {
			return Table{}, errors.New("response contains more than one matching table")
		}
		match = index
	}
	if match < 0 {
		return Table{}, errors.New("expected table headers are missing")
	}
	return tables[match], nil
}

func equalStrings(left, right []string) bool {
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
