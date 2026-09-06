package store

import "database/sql"

const (
	rehaReceiptVersion = 1
	rehaSnapshotMethod = "sqlite_online_backup_v1"
	rehaTimezone       = "Europe/Berlin"
	rehaMinSchema      = 5
	rehaMaxSchema      = 6
)

// RehaScopeInput is the small, pre-approved statement that gives a supported
// database its one permitted source and location. Neither schema v5 nor v6 has
// a location column, so this statement must never be inferred from course
// text, a profile name, or a file path.
type RehaScopeInput struct {
	Version    int               `json:"version"`
	Source     string            `json:"source"`
	SourceDB   string            `json:"source_db"`
	Location   string            `json:"location"`
	Provenance string            `json:"provenance"`
	Evidence   RehaScopeEvidence `json:"evidence"`
}

// RehaScopeEvidence prevents a caller from turning an arbitrary location
// string into an accepted scope. The evidence object is a pre-existing local
// verification record; this CLI only carries it into the receipt and never
// claims to verify the underlying business source itself.
type RehaScopeEvidence struct {
	Kind       string `json:"kind"`
	Reference  string `json:"reference"`
	SHA256     string `json:"sha256"`
	VerifiedAt string `json:"verified_at"`
	SourceDB   string `json:"source_db"`
	Source     string `json:"source"`
	Location   string `json:"location"`
}

// RehaEvidenceReceipt is the strict, separately stored receipt referenced by
// RehaScopeEvidence.Reference. Its bytes are hashed into the scope input; it
// carries only the independently supplied source/location statement and no
// member data.
type RehaEvidenceReceipt struct {
	Version    int    `json:"version"`
	SourceDB   string `json:"source_db"`
	Source     string `json:"source"`
	Location   string `json:"location"`
	VerifiedAt string `json:"verified_at"`
	Kind       string `json:"kind"`
}

// SnapshotReceipt binds the exact source scope to the bytes of a newly
// created SQLite snapshot. It is intentionally a separate sidecar instead of
// being inserted into the source database.
type SnapshotReceipt struct {
	Version          int               `json:"version"`
	Source           string            `json:"source"`
	Location         string            `json:"location"`
	Provenance       string            `json:"provenance"`
	Evidence         RehaScopeEvidence `json:"evidence"`
	SourceDB         string            `json:"source_db"`
	SnapshotDB       string            `json:"snapshot_db"`
	SourceSchema     int               `json:"source_schema_version"`
	SnapshotSHA256   string            `json:"snapshot_sha256"`
	ScopeInputSHA256 string            `json:"scope_input_sha256"`
	Method           string            `json:"method"`
	StartedAt        string            `json:"started_at"`
	CompletedAt      string            `json:"completed_at"`
}

// SnapshotResult is the aggregate-safe acknowledgement printed by db
// snapshot. Paths are local operator information; no database rows are
// included.
type SnapshotResult struct {
	Status         string `json:"status"`
	Source         string `json:"source"`
	Location       string `json:"location"`
	SourceDB       string `json:"source_db"`
	SnapshotDB     string `json:"snapshot_db"`
	Receipt        string `json:"receipt"`
	SchemaVersion  int    `json:"schema_version"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	Method         string `json:"method"`
}

// ReadOnlyStore never creates, migrates, chmods, or changes journal mode on
// its database. It is used only for the snapshot-backed aggregate report.
type ReadOnlyStore struct {
	db   *sql.DB
	path string
}

func supportedRehaSchema(schema int) bool {
	return schema >= rehaMinSchema && schema <= rehaMaxSchema
}

type RehaSessionRow struct {
	Source                             string `json:"source"`
	Location                           string `json:"location"`
	StableSessionID                    string `json:"stable_session_id"`
	Date                               string `json:"date"`
	Time                               string `json:"time"`
	Course                             string `json:"course,omitempty"`
	ParticipantCountCurrentObservation int    `json:"participant_count_current_observation"`
	AttendanceRows                     int    `json:"attendance_rows"`
	AttendedFlagTrue                   int    `json:"attended_flag_true"`
	SignedFlagTrue                     int    `json:"signed_flag_true"`
	CancelledFlagTrue                  int    `json:"cancelled_flag_true"`
	AttendedNotCancelled               int    `json:"attended_not_cancelled"`
	SignedAttendedNotCancelled         int    `json:"signed_attended_not_cancelled"`
	MissingSignatureCandidate          int    `json:"missing_signature_candidate"`
	NonAttendedNotCancelledCandidate   int    `json:"non_attended_not_cancelled_candidate"`
	PrescriptionLinkedRows             int    `json:"prescription_linked_rows"`
	PrescriptionUnlinkedRows           int    `json:"prescription_unlinked_rows"`
	ContradictoryFlags                 int    `json:"contradictory_flags"`
}

type RehaCoverage struct {
	Status          string   `json:"status"`
	SourceSessions  int      `json:"source_sessions"`
	SessionsInRange int      `json:"sessions_in_range"`
	InvalidDateRows int      `json:"invalid_date_rows"`
	InvalidEndRows  int      `json:"invalid_end_rows"`
	ObservedFrom    string   `json:"observed_from,omitempty"`
	ObservedTo      string   `json:"observed_to,omitempty"`
	Gaps            []string `json:"gaps"`
}

type RehaSessionReport struct {
	SnapshotAt                 string           `json:"snapshot_at"`
	ReportVersion              int              `json:"report_version"`
	GeneratedAt                string           `json:"generated_at"`
	ImportObservedAt           string           `json:"import_observed_at"`
	ObservationBasis           string           `json:"observation_basis"`
	ParticipantCountValidation string           `json:"participant_count_validation"`
	PopulationStatus           string           `json:"population_status"`
	RequestedFrom              string           `json:"requested_from"`
	RequestedTo                string           `json:"requested_to"`
	Timezone                   string           `json:"timezone"`
	AsOf                       string           `json:"as_of"`
	Source                     string           `json:"source"`
	Location                   string           `json:"location"`
	ScopeBasis                 string           `json:"scope_basis"`
	SnapshotSHA256             string           `json:"snapshot_sha256"`
	DataAsOf                   string           `json:"data_as_of,omitempty"`
	Coverage                   RehaCoverage     `json:"coverage"`
	Sessions                   []RehaSessionRow `json:"sessions"`
}
