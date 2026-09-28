package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
)

func TestAdminObservationsPreserveRunsFiltersDuplicatesAndABA(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i, value := range []string{"A", "B", "A"} {
		page := admin.Page{Metadata: admin.PageMetadata{Route: "capability:course-day-list", Fingerprint: strings.Repeat("a", 64)},
			Tables:      []admin.Table{{Headers: []string{"Kurs"}, Rows: [][]string{{value}, {value}}}},
			RequestPath: "/Kursplaner/Listen/Kursplaner_Liste_Tag_anzeigen.asp", RequestBody: "von=2026-09-11"}
		if _, err := db.ImportAdminPageAt(ctx, page, time.Date(2026, 9, 12, i, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.db.Query(`SELECT request_body, page_json FROM admin_observations ORDER BY run_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var body, raw string
		if err := rows.Scan(&body, &raw); err != nil {
			t.Fatal(err)
		}
		if body != "von=2026-09-11" {
			t.Fatal("lost request scope")
		}
		var page admin.Page
		if err := json.Unmarshal([]byte(raw), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Tables[0].Rows) != 2 {
			t.Fatal("duplicate row was lost")
		}
		got = append(got, page.Tables[0].Rows[0][0])
	}
	if strings.Join(got, ",") != "A,B,A" {
		t.Fatalf("lost observation history: %v", got)
	}
}
