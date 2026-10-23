package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
)

const PrescriptionObservationVersion = 1

// Statuses of a prescription hash source inside an observation.
const (
	ObservationOK              = "ok"
	ObservationNotCollected    = "not_collected"
	ObservationSchemaUnmatched = "schema_unmatched"
)

// PrescriptionHashSource is one collected capability reduced to the hash set
// of its prescription numbers. It carries no names, member numbers or doctors.
type PrescriptionHashSource struct {
	Capability        string   `json:"capability"`
	Status            string   `json:"status"`
	ObservedAt        string   `json:"observed_at,omitempty"`
	SchemaFingerprint string   `json:"schema_fingerprint,omitempty"`
	CountedRows       int      `json:"counted_rows"`
	SkippedRows       int      `json:"skipped_rows"`
	Hashes            []string `json:"hashes"`
}

// PrescriptionObservation is the local state of the prescription lists in the
// database: the active set and, when collected, the archived set that serves as
// evidence that a prescription ended.
type PrescriptionObservation struct {
	Version    int                    `json:"version"`
	HashScheme string                 `json:"hash_scheme"`
	Active     PrescriptionHashSource `json:"active"`
	Archived   PrescriptionHashSource `json:"archived"`
}

func (store *ReadOnlyStore) PrescriptionObservation(ctx context.Context) (PrescriptionObservation, error) {
	active, err := store.prescriptionHashSource(ctx, "reha-prescriptions", admin.ActiveRehaPrescriptionIDs)
	if err != nil {
		return PrescriptionObservation{}, err
	}
	if active.Status != ObservationOK {
		return PrescriptionObservation{}, errors.New("active prescription list is missing or does not match its observed schema")
	}
	archived, err := store.prescriptionHashSource(ctx, "reha-prescriptions-archived", admin.ArchivedRehaPrescriptionIDs)
	if err != nil {
		return PrescriptionObservation{}, err
	}
	return PrescriptionObservation{
		Version:    PrescriptionObservationVersion,
		HashScheme: "sha256-myyolo-prescription-v1",
		Active:     active,
		Archived:   archived,
	}, nil
}

func (store *ReadOnlyStore) prescriptionHashSource(
	ctx context.Context,
	capability string,
	extract func(admin.Page) ([]string, int, error),
) (PrescriptionHashSource, error) {
	source := PrescriptionHashSource{Capability: capability, Hashes: []string{}}
	page, rowCounts, observedAt, fingerprint, err := store.capabilityPage(ctx, capability)
	if errors.Is(err, sql.ErrNoRows) {
		source.Status = ObservationNotCollected
		return source, nil
	}
	if err != nil {
		return source, err
	}
	source.ObservedAt, source.SchemaFingerprint = observedAt, fingerprint
	ids, skipped, err := extract(page)
	if err == nil {
		err = rowCountsMatch(page.Tables, rowCounts)
	}
	if err != nil {
		source.Status = ObservationSchemaUnmatched
		return source, nil
	}
	source.Status = ObservationOK
	source.CountedRows, source.SkippedRows = len(ids), skipped
	source.Hashes = admin.HashPrescriptionIDs(ids)
	return source, nil
}

// rowCountsMatch rejects a reconstruction that lost rows of a prescription
// table (identical rows collapse into one stored record). Both extractors read
// the table that has a "Nr." column.
func rowCountsMatch(tables []admin.Table, rowCounts []int) error {
	for index, table := range tables {
		for _, header := range table.Headers {
			if header == "Nr." && rowCounts[index] != len(table.Rows) {
				return errors.New("prescription capability rows cannot be reconstructed completely")
			}
		}
	}
	return nil
}
