package store

import (
	"context"
	"errors"
	"time"
)

const rehaInactivityReportVersion = 1

// RehaInactivityBucket is deliberately count-less while the source cannot
// prove both the current assignment population and complete attendance
// history. A future source with those guarantees can fill Count safely.
type RehaInactivityBucket struct {
	Status string `json:"status"`
	Count  *int   `json:"count,omitempty"`
}

// RehaInactivityReport is a capability/status report for the aggregate-only
// Reha inactivity question. It never emits a person, member ID, or course.
// The current mySIGN snapshot cannot support candidate counts truthfully.
type RehaInactivityReport struct {
	ReportVersion           int                  `json:"report_version"`
	GeneratedAt             string               `json:"generated_at"`
	AsOf                    string               `json:"as_of"`
	Timezone                string               `json:"timezone"`
	Source                  string               `json:"source"`
	Location                string               `json:"location"`
	ScopeBasis              string               `json:"scope_basis"`
	SnapshotSHA256          string               `json:"snapshot_sha256"`
	SnapshotAt              string               `json:"snapshot_at,omitempty"`
	ImportObservedAt        string               `json:"import_observed_at"`
	DataAsOf                string               `json:"data_as_of"`
	CapabilityStatus        string               `json:"capability_status"`
	CoverageStatus          string               `json:"coverage_status"`
	HistoryStatus           string               `json:"history_status"`
	CurrentAssignmentStatus string               `json:"current_assignment"`
	MemberStatus            string               `json:"member_status"`
	PopulationStatus        string               `json:"population_status"`
	Over28Days              RehaInactivityBucket `json:"over_28_days"`
	Over3Months             RehaInactivityBucket `json:"over_3_months"`
	UnknownHistory          RehaInactivityBucket `json:"unknown_history"`
	Reasons                 []string             `json:"reasons"`
}

// RehaInactivity returns a truthful status for the inactivity capability.
// A single validated mySIGN snapshot has no completeness marker for its
// rolling attendance rows and no member-scoped current assignment/status.
// Therefore this method intentionally does not classify anyone or emit zero
// counts, even when the snapshot contains rows that look assignment-like.
func (store *ReadOnlyStore) RehaInactivity(
	ctx context.Context,
	source string,
	location string,
	asOf time.Time,
	snapshotSHA256 string,
) (RehaInactivityReport, error) {
	if store == nil || store.db == nil {
		return RehaInactivityReport{}, errors.New("read-only database is not open")
	}
	if source != "mysign" {
		return RehaInactivityReport{}, errors.New("unsupported Reha source")
	}
	if location == "" {
		return RehaInactivityReport{}, errors.New("Reha location is required")
	}
	if asOf.IsZero() {
		return RehaInactivityReport{}, errors.New("Reha as-of is required")
	}

	var observedAt, validation string
	if err := store.db.QueryRowContext(ctx, `
		SELECT observed_at, validation
		FROM reha_import_observation
		WHERE singleton=1
	`).Scan(&observedAt, &validation); err != nil || validation != "required_nonnegative_integer_and_collections_v1" {
		return RehaInactivityReport{}, errors.New("Reha report requires a fresh single validated import")
	}
	if _, err := parseDataTimestamp(observedAt); err != nil {
		return RehaInactivityReport{}, errors.New("invalid import observation timestamp")
	}
	dataAsOf, err := store.rehaDataAsOf(ctx, source)
	if err != nil {
		return RehaInactivityReport{}, err
	}

	return RehaInactivityReport{
		ReportVersion:           rehaInactivityReportVersion,
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339Nano),
		AsOf:                    asOf.UTC().Format(time.RFC3339Nano),
		Timezone:                rehaTimezone,
		Source:                  source,
		Location:                location,
		ScopeBasis:              "external_verified_scope",
		SnapshotSHA256:          snapshotSHA256,
		ImportObservedAt:        observedAt,
		DataAsOf:                dataAsOf,
		CapabilityStatus:        "unavailable",
		CoverageStatus:          "observed_rows_only",
		HistoryStatus:           "incomplete",
		CurrentAssignmentStatus: "unavailable",
		MemberStatus:            "unavailable",
		PopulationStatus:        "reha_membership_unproven",
		Over28Days:              RehaInactivityBucket{Status: "unavailable"},
		Over3Months:             RehaInactivityBucket{Status: "unavailable"},
		UnknownHistory:          RehaInactivityBucket{Status: "unavailable"},
		Reasons: []string{
			"single_snapshot_does_not_prove_complete_attendance_history",
			"current_assignment_is_not_available_as_member_scoped_state",
			"member_status_is_not_available",
		},
	}, nil
}

// RehaInactivityThresholds documents the business boundary for a future
// complete source. Both boundaries use calendar arithmetic in Europe/Berlin:
// the four-week boundary is strict (before 28 calendar days), while the
// Premium boundary uses calendar-month subtraction.
type RehaInactivityThresholds struct {
	Over28DaysBefore  time.Time
	Over3MonthsBefore time.Time
}

// ComputeRehaInactivityThresholds is kept separate from RehaInactivity so
// that no threshold is accidentally used to classify an incomplete snapshot.
func ComputeRehaInactivityThresholds(asOf time.Time) (RehaInactivityThresholds, error) {
	if asOf.IsZero() {
		return RehaInactivityThresholds{}, errors.New("Reha as-of is required")
	}
	location, err := berlinLocation()
	if err != nil {
		return RehaInactivityThresholds{}, err
	}
	local := asOf.In(location)
	strict28 := local.AddDate(0, 0, -28)
	calendar3 := local.AddDate(0, -3, 0)
	return RehaInactivityThresholds{
		Over28DaysBefore:  strict28,
		Over3MonthsBefore: calendar3,
	}, nil
}
