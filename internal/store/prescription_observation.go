package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
)

// RemainingUnitsRequest is the only request whose statistic lists every active
// prescription: the threshold 999 is above any unit count.
const RemainingUnitsRequest = "Menge=999"

// PrescriptionObservation is the latest "Resttermine" collect, keyed with the
// caller's HMAC key.
type PrescriptionObservation struct {
	Version    int    `json:"version"`
	Capability string `json:"capability"`
	ObservedAt string `json:"observed_at"`
	KeyID      string `json:"key_id"`
	admin.PrescriptionObservation
}

func (store *ReadOnlyStore) PrescriptionObservation(ctx context.Context, key []byte) (PrescriptionObservation, error) {
	if store == nil || store.db == nil {
		return PrescriptionObservation{}, errors.New("read-only database is not open")
	}
	var body, observedAt, encoded string
	if err := store.db.QueryRowContext(ctx, `
		SELECT request_body, observed_at, page_json FROM admin_observations
		WHERE route=? ORDER BY run_id DESC LIMIT 1`,
		readcatalog.RouteKey("prescription-remaining-units"),
	).Scan(&body, &observedAt, &encoded); err != nil {
		return PrescriptionObservation{}, fmt.Errorf("read remaining units observation: %w", err)
	}
	if body != RemainingUnitsRequest {
		return PrescriptionObservation{}, errors.New("remaining units were not collected with threshold 999")
	}
	var page admin.Page
	if err := json.Unmarshal([]byte(encoded), &page); err != nil {
		return PrescriptionObservation{}, errors.New("decode remaining units observation")
	}
	observation, err := admin.ObservePrescriptions(page, key)
	if err != nil {
		return PrescriptionObservation{}, err
	}
	return PrescriptionObservation{
		Version:                 1,
		Capability:              "prescription-remaining-units",
		ObservedAt:              observedAt,
		KeyID:                   admin.KeyID(key),
		PrescriptionObservation: observation,
	}, nil
}
