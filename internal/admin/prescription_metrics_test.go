package admin

import (
	"strconv"
	"strings"
	"testing"
)

func TestAggregateActiveRehaPrescriptions(t *testing.T) {
	page := Page{
		Metadata: PageMetadata{Route: activeRehaPrescriptionsRoute},
		Tables: []Table{{
			Headers: []string{"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", ""},
			Rows: [][]string{
				{"vo-1", "member-1", "Muster", "Eins", "50", "4", "46", "yes", ""},
				{"vo-2", "member-1", "Muster", "Eins", "50", "2", "48", "yes", ""},
				{"Seitenende"},
			},
		}},
	}

	got, err := AggregatePrescriptionPage(page)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || got.CountedRows != 2 || got.SkippedRows != 1 ||
		got.Coverage != "schema_matched_rows_only" {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestAggregateActiveRehaPrescriptionsRejectsDuplicateID(t *testing.T) {
	headers := []string{"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", ""}
	page := Page{
		Metadata: PageMetadata{Route: activeRehaPrescriptionsRoute},
		Tables: []Table{{
			Headers: headers,
			Rows: [][]string{
				{"vo-1", "member-1", "", "", "50", "4", "46", "", ""},
				{"vo-1", "member-2", "", "", "50", "3", "47", "", ""},
			},
		}},
	}

	_, err := AggregatePrescriptionPage(page)
	if err == nil || !strings.Contains(err.Error(), "duplicate prescription identifier") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestAggregateRehaPrescriptionSummary(t *testing.T) {
	page := Page{
		Metadata: PageMetadata{Route: rehaPrescriptionSummaryRoute},
		Tables: []Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows: [][]string{
				{"", "Praxis A", "2"},
				{"", "Praxis B", "3"},
				{"Gesamt", ""},
			},
		}},
	}

	got, err := AggregatePrescriptionPage(page)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 5 || got.CountedRows != 2 || got.SkippedRows != 1 {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestAggregateRehaPrescriptionSummaryRejectsMalformedMenge(t *testing.T) {
	page := Page{
		Metadata: PageMetadata{Route: rehaPrescriptionSummaryRoute},
		Tables: []Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows:    [][]string{{"", "Praxis A", "2 Fälle"}},
		}},
	}

	_, err := AggregatePrescriptionPage(page)
	if err == nil || !strings.Contains(err.Error(), "Menge is not a nonnegative integer") {
		t.Fatalf("expected strict integer rejection, got %v", err)
	}
}

func TestAggregateRehaPrescriptionSummaryRejectsTotalOverflow(t *testing.T) {
	page := Page{
		Metadata: PageMetadata{Route: rehaPrescriptionSummaryRoute},
		Tables: []Table{{
			Headers: []string{"", "Arzt", "Menge"},
			Rows: [][]string{
				{"", "Praxis A", strconv.Itoa(int(^uint(0) >> 1))},
				{"", "Praxis B", "1"},
			},
		}},
	}

	_, err := AggregatePrescriptionPage(page)
	if err == nil || !strings.Contains(err.Error(), "Menge total is outside") {
		t.Fatalf("expected total overflow rejection, got %v", err)
	}
}

func TestAggregatePrescriptionPageRejectsSchemaDrift(t *testing.T) {
	page := Page{
		Metadata: PageMetadata{Route: rehaPrescriptionSummaryRoute},
		Tables:   []Table{{Headers: []string{"Arzt", "Menge"}}},
	}

	_, err := AggregatePrescriptionPage(page)
	if err == nil || !strings.Contains(err.Error(), "expected table headers are missing") {
		t.Fatalf("expected schema rejection, got %v", err)
	}
}
