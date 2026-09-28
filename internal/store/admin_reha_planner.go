package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"net/url"
	"time"
)

type plannerObservation struct {
	Row   admin.CoursePlannerRow
	Stamp string
}

func plannerKey(course, date, planner string) string { return course + "/" + date + "/" + planner }

// Only exact native course/date/planner relationships are retained. No matching
// by labels, clock time, capacity or coincident counts is allowed.
func (s *ReadOnlyStore) adminPlannerInventory(ctx context.Context, from, to, asOf time.Time) (map[string]plannerObservation, *int, error) {
	result := map[string]plannerObservation{}
	fail := func() (map[string]plannerObservation, *int, error) {
		return nil, nil, errors.New("invalid native planner inventory proof")
	}
	rows, e := s.db.QueryContext(ctx, `SELECT request_path,observed_at,page_json FROM admin_observations WHERE route='capability:course-planner-week' ORDER BY run_id`)
	if e != nil {
		return fail()
	}
	defer rows.Close()
	type observation struct {
		Facts admin.CoursePlannerFacts
		Stamp time.Time
	}
	weeks := map[string]observation{}
	capability, _ := readcatalog.Lookup("course-planner-week")
	for rows.Next() {
		var path, stamp, raw string
		if rows.Scan(&path, &stamp, &raw) != nil {
			return fail()
		}
		observed, e := parseDataTimestamp(stamp)
		if e != nil {
			return fail()
		}
		if observed.After(asOf) {
			continue
		}
		var page admin.Page
		if json.Unmarshal([]byte(raw), &page) != nil || page.CoursePlannerFacts == nil {
			return fail()
		}
		f := page.CoursePlannerFacts
		if f.Completeness != "validated_week_inventory" {
			return fail()
		}
		u, e := url.ParseRequestURI(path)
		if e != nil || u.IsAbs() || u.Path != capability.Path {
			return fail()
		}
		for k, v := range u.Query() {
			if (k != "Datum" && k != "Raum") || len(v) != 1 {
				return fail()
			}
		}
		if u.Query().Get("Raum") != "" {
			return fail()
		}
		start, e := time.ParseInLocation("2006-01-02", f.From, from.Location())
		if e != nil || start.Weekday() != time.Monday || start.AddDate(0, 0, 6).Format("2006-01-02") != f.To {
			return fail()
		}
		sourceDate := observed.In(from.Location())
		if v := u.Query().Get("Datum"); v != "" {
			sourceDate, e = time.ParseInLocation("02.01.2006", v, from.Location())
			if e != nil {
				return fail()
			}
		}
		day := sourceDate.Format("2006-01-02")
		if day < f.From || day > f.To {
			return fail()
		}
		if f.To < from.Format("2006-01-02") || f.From > to.Format("2006-01-02") {
			continue
		}
		old, ok := weeks[f.From]
		if !ok || observed.After(old.Stamp) {
			weeks[f.From] = observation{*f, observed}
		}
	}
	if rows.Err() != nil {
		return fail()
	}
	covered := map[string]bool{}
	for _, w := range weeks {
		start, _ := time.ParseInLocation("2006-01-02", w.Facts.From, from.Location())
		for i := 0; i < 7; i++ {
			covered[start.AddDate(0, 0, i).Format("2006-01-02")] = true
		}
		for _, r := range w.Facts.Rows {
			if r.Date < w.Facts.From || r.Date > w.Facts.To || r.Registered < 0 || r.Capacity < 0 || r.Capacity-r.Registered != r.Free || r.Cancelled < 0 {
				return fail()
			}
			if r.Date < from.Format("2006-01-02") || r.Date > to.Format("2006-01-02") {
				continue
			}
			key := plannerKey(adminRehaPseudonym("course", r.CourseID), r.Date, r.Planner)
			if _, ok := result[key]; ok {
				return fail()
			}
			result[key] = plannerObservation{r, w.Stamp.UTC().Format(time.RFC3339Nano)}
		}
	}
	complete := true
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !covered[d.Format("2006-01-02")] {
			complete = false
		}
	}
	var expected *int
	if complete {
		n := len(result)
		expected = &n
	}
	return result, expected, nil
}
