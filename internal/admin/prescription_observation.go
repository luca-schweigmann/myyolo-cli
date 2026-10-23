package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// PrescriptionIDHash is the only form in which a prescription number leaves
// the admin parser: a domain-separated SHA-256 of the trimmed "Nr." value.
func PrescriptionIDHash(id string) string {
	sum := sha256.Sum256([]byte("myyolo-prescription-v1\x00" + strings.TrimSpace(id)))
	return hex.EncodeToString(sum[:])
}

// ActiveRehaPrescriptionIDs returns the distinct "Nr." values of the observed
// active-prescription table. The header must match exactly; duplicates are an
// error, and rows without "Nr." or "M-Nr.:" are counted as skipped.
func ActiveRehaPrescriptionIDs(page Page) ([]string, int, error) {
	table, err := exactHeaderTable(page.Tables, []string{
		"Nr.", "M-Nr.:", "Name", "Vorname", "Anzahl", "Besuche", "Termine", "AG", "",
	})
	if err != nil {
		return nil, 0, fmt.Errorf("active Reha prescriptions: %w", err)
	}
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(table.Rows))
	skipped := 0
	for _, row := range table.Rows {
		if len(row) != len(table.Headers) || strings.TrimSpace(row[0]) == "" || strings.TrimSpace(row[1]) == "" {
			skipped++
			continue
		}
		id := strings.TrimSpace(row[0])
		if _, duplicate := seen[id]; duplicate {
			return nil, 0, errors.New("duplicate prescription identifier")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, skipped, nil
}

// ArchivedRehaPrescriptionIDs reads the "Nr." column of the archived-
// prescription export. Its full header row is not yet observed, so the parser
// accepts exactly one table with exactly one "Nr." column and fails closed on
// anything else.
func ArchivedRehaPrescriptionIDs(page Page) ([]string, int, error) {
	column, match := -1, -1
	for index, table := range page.Tables {
		found := -1
		for position, header := range table.Headers {
			if header != "Nr." {
				continue
			}
			if found >= 0 {
				return nil, 0, errors.New("archived prescriptions: more than one Nr. column")
			}
			found = position
		}
		if found < 0 {
			continue
		}
		if match >= 0 {
			return nil, 0, errors.New("archived prescriptions: more than one table with a Nr. column")
		}
		column, match = found, index
	}
	if match < 0 {
		return nil, 0, errors.New("archived prescriptions: Nr. column is missing")
	}
	table := page.Tables[match]
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(table.Rows))
	skipped := 0
	for _, row := range table.Rows {
		if len(row) != len(table.Headers) || strings.TrimSpace(row[column]) == "" {
			skipped++
			continue
		}
		id := strings.TrimSpace(row[column])
		if _, duplicate := seen[id]; duplicate {
			return nil, 0, errors.New("archived prescriptions: duplicate identifier")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, skipped, nil
}

// HashPrescriptionIDs returns the sorted hash set of the given identifiers.
func HashPrescriptionIDs(ids []string) []string {
	hashes := make([]string, len(ids))
	for index, id := range ids {
		hashes[index] = PrescriptionIDHash(id)
	}
	sort.Strings(hashes)
	return hashes
}
