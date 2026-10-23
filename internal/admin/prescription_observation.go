package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrPrescriptionKeyCollision: two active Reha prescriptions share member
// number and end date. They cannot be told apart from night to night, and
// merging them would undercount the stock, so the observation fails instead.
var ErrPrescriptionKeyCollision = errors.New("two active prescriptions share member number and end date")

// remainingUnitsHeaders is the observed header row of the statistic
// "Resttermine" (capability prescription-remaining-units).
var remainingUnitsHeaders = []string{
	"", "Nr.:", "Name", "Vorname", "Art", "Termine", "wahrgenommen", "Rest", "Termine", "Ende", "",
}

// operatorLocation matches the postal code and town that close the operator
// line in the page head ("... 70839 Gerlingen").
var operatorLocation = regexp.MustCompile(`(?:^|\s)(\d{5} \S+)$`)

// PrescriptionState is one active prescription reduced to its unit counts.
// Key is HMAC-SHA256(key, member number "|" end date); names, member numbers,
// doctors and the second "Termine" column never leave the parser.
type PrescriptionState struct {
	Key          string `json:"vo_key"`
	Termine      int    `json:"termine"`
	Wahrgenommen int    `json:"wahrgenommen"`
	Rest         int    `json:"rest"`
	Ende         string `json:"ende"`
}

// PrescriptionObservation is the "Resttermine" statistic of one collect run.
type PrescriptionObservation struct {
	Location     string              `json:"location"`
	Rows         []PrescriptionState `json:"rows"`
	OtherArtRows int                 `json:"other_art_rows"`
}

// ObservePrescriptions reads the "Resttermine" statistic. It fails closed on
// header drift, lost rows, malformed cells, duplicate keys and an operator
// location that is missing or not unique.
func ObservePrescriptions(page Page, key []byte) (PrescriptionObservation, error) {
	table, err := exactHeaderTable(page.Tables, remainingUnitsHeaders)
	if err != nil {
		return PrescriptionObservation{}, fmt.Errorf("remaining units: %w", err)
	}
	match := -1
	for index := range page.Tables {
		if equalStrings(page.Tables[index].Headers, remainingUnitsHeaders) {
			match = index
		}
	}
	if match >= len(page.Metadata.TableRows) || page.Metadata.TableRows[match] != len(table.Rows) {
		return PrescriptionObservation{}, errors.New("remaining units: rows cannot be reconstructed completely")
	}
	location, err := pageLocation(page.Tables, match)
	if err != nil {
		return PrescriptionObservation{}, err
	}

	observation := PrescriptionObservation{Location: location, Rows: []PrescriptionState{}}
	seen := make(map[string]struct{}, len(table.Rows))
	for _, row := range table.Rows {
		if len(row) != len(remainingUnitsHeaders) {
			return PrescriptionObservation{}, errors.New("remaining units: row width does not match header")
		}
		if strings.TrimSpace(row[4]) != "Reha" {
			observation.OtherArtRows++
			continue
		}
		member := strings.TrimSpace(row[1])
		if member == "" || strings.Trim(member, "0123456789") != "" {
			return PrescriptionObservation{}, errors.New("remaining units: member number is not numeric")
		}
		termine, errT := count(row[5])
		wahrgenommen, errW := count(row[6])
		rest, errR := count(row[7])
		if errT != nil || errW != nil || errR != nil || rest != termine-wahrgenommen {
			return PrescriptionObservation{}, errors.New("remaining units: unit counts are inconsistent")
		}
		end, err := time.Parse("02.01.2006", strings.TrimSpace(row[9]))
		if err != nil {
			return PrescriptionObservation{}, errors.New("remaining units: end date is invalid")
		}
		ende := end.Format("2006-01-02")
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(member + "|" + ende))
		state := PrescriptionState{
			Key:          hex.EncodeToString(mac.Sum(nil)),
			Termine:      termine,
			Wahrgenommen: wahrgenommen,
			Rest:         rest,
			Ende:         ende,
		}
		if _, duplicate := seen[state.Key]; duplicate {
			return PrescriptionObservation{}, ErrPrescriptionKeyCollision
		}
		seen[state.Key] = struct{}{}
		observation.Rows = append(observation.Rows, state)
	}
	sort.Slice(observation.Rows, func(i, j int) bool { return observation.Rows[i].Key < observation.Rows[j].Key })
	return observation, nil
}

// KeyID identifies an HMAC key without revealing it, so a reader can tell
// observations made with another key apart.
func KeyID(key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("myyolo-prescription-key-id"))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

func pageLocation(tables []Table, data int) (string, error) {
	found := map[string]struct{}{}
	for index, table := range tables {
		if index == data {
			continue
		}
		for _, row := range table.Rows {
			for _, cell := range row {
				if match := operatorLocation.FindStringSubmatch(strings.TrimSpace(cell)); match != nil {
					found[match[1]] = struct{}{}
				}
			}
		}
	}
	if len(found) != 1 {
		return "", errors.New("remaining units: operator location is missing or not unique")
	}
	for location := range found {
		return location, nil
	}
	return "", nil
}

func count(cell string) (int, error) {
	value := strings.TrimSpace(cell)
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, errors.New("not a nonnegative integer")
	}
	return strconv.Atoi(value)
}
