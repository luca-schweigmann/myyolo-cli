package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
)

func TestRehaImportGateRejectsSecondObservation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	prepareRehaSource(t, path)
	query := func(wantError bool) {
		db, err := OpenReadOnly(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		report, err := db.RehaSessions(ctx, "mysign", "point-gerlingen", mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-08-31"), time.Now(), strings.Repeat("a", 64))
		if (err != nil) != wantError {
			t.Fatalf("report error: %v", err)
		}
		if !wantError {
			if report.ReportVersion != 3 || report.ImportObservedAt == "" || report.GeneratedAt == "" || report.DataAsOf != report.ImportObservedAt {
				t.Fatal("missing observation contract")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "no_shows") || strings.Contains(string(encoded), `"pending"`) {
				t.Fatal("definitive no-show or ambiguous pending status exposed")
			}
		}
	}
	query(false)
	raw, err := os.ReadFile(filepath.Join("..", "mysign", "testdata", "get_list_data.synthetic.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mysign.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ImportMySign(ctx, snapshot, "second"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	query(true)
}

func TestRehaLegacyDatabaseCannotClaimWireValidation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	_, err = ro.RehaSessions(ctx, "mysign", "point-gerlingen", mustBerlinDate(t, "2026-07-01"), mustBerlinDate(t, "2026-08-31"), time.Now(), "")
	if err == nil || !strings.Contains(err.Error(), "fresh single validated import") {
		t.Fatalf("old DB accepted: %v", err)
	}
}

func TestSessionEndBerlinDST(t *testing.T) {
	for _, tc := range []struct{ date, clock, want string }{
		{"2026-07-28T00:00:00Z", "10:00 - 10:45", "2026-07-28T08:45:00Z"},
		{"2026-03-29T00:00:00+01:00", "01:00 - 03:30", "2026-03-29T01:30:00Z"},
		{"2026-03-29T00:00:00+01:00", "01:00 - 02:30", ""},
		{"2026-10-25T00:00:00+02:00", "01:00 - 02:30", ""},
		{"2026-10-25T00:00:00+02:00", "01:00 - 03:30", "2026-10-25T02:30:00Z"},
		{"2026-03-28T23:00:00+01:00", "23:00 - 03:30", "2026-03-29T01:30:00Z"},
		{"2026-03-28T23:00:00+01:00", "23:00 - 02:30", ""},
		{"2026-03-29T23:00:00+02:00", "23:00 - 02:30", "2026-03-30T00:30:00Z"},
	} {
		if got := sessionEndUTC(mysign.CourseSession{DateISO: tc.date, TimeFormatted: tc.clock}); got != tc.want {
			t.Errorf("%s %s: got %s want %s", tc.date, tc.clock, got, tc.want)
		}
	}
}

func TestRehaFlagIntersections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flags.sqlite")
	prepareConflictSource(t, path)
	for attended := 0; attended <= 1; attended++ {
		for signed := 0; signed <= 1; signed++ {
			for cancelled := 0; cancelled <= 1; cancelled++ {
				writer, err := Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.db.Exec(`UPDATE attendance SET attended=?,signed=?,cancelled=? WHERE myyolo_id='attendance-conflict'`, attended, signed, cancelled); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				ro, err := OpenReadOnly(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				report, err := ro.RehaSessions(ctx, "mysign", "point-gerlingen", mustBerlinDate(t, "2026-07-28"), mustBerlinDate(t, "2026-07-28"), time.Now(), "")
				ro.Close()
				if err != nil {
					t.Fatal(err)
				}
				row := report.Sessions[0]
				wantAttended := attended * (1 - cancelled)
				// prepareConflictSource also contains one fixed non-attended,
				// non-cancelled row beside the row varied by this matrix.
				wantNonAttended := 1 + (1-attended)*(1-cancelled)
				if row.AttendedFlagTrue != attended || row.SignedFlagTrue != signed || row.CancelledFlagTrue != cancelled ||
					row.AttendanceRows != 2 ||
					row.AttendedNotCancelled != wantAttended || row.SignedAttendedNotCancelled != wantAttended*signed ||
					row.MissingSignatureCandidate != wantAttended*(1-signed) || row.NonAttendedNotCancelledCandidate != wantNonAttended {
					t.Fatalf("flags %d/%d/%d yielded %#v", attended, signed, cancelled, row)
				}
			}
		}
	}
}
