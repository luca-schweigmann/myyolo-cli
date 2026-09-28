package admin

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type CoursePlannerFacts struct {
	From         string             `json:"from"`
	To           string             `json:"to"`
	Completeness string             `json:"completeness"`
	Rows         []CoursePlannerRow `json:"rows"`
}
type CoursePlannerRow struct {
	CourseID    string `json:"course_id"`
	Planner     string `json:"planner,omitempty"`
	Date        string `json:"date"`
	CourseLabel string `json:"course_label"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Capacity    int    `json:"capacity"`
	Linked      int    `json:"linked"`
	Registered  int    `json:"registered"`
	Cancelled   int    `json:"cancelled"`
	Free        int    `json:"free"`
}

func parseCoursePlanner(page Page) (*CoursePlannerFacts, error) {
	out := &CoursePlannerFacts{Completeness: "validated_week_inventory", Rows: []CoursePlannerRow{}}
	matches := 0
	for ti, t := range page.Tables {
		candidate := false
		for _, h := range t.Headers {
			if h == "belegt" {
				candidate = true
			}
		}
		if !candidate {
			continue
		}
		matches++
		if !reflect.DeepEqual(t.Headers, []string{"", "Kurs", "Raum", "Kursleitung", "Uhrzeit", "", "Plätze", "verkettet", "belegt", "storniert", "frei", ""}) {
			return nil, errors.New("planner header drift")
		}
		day := ""
		days := 0
		seen := map[string]bool{}
		for ri, r := range t.Rows {
			if len(r) == 2 {
				d, e := rangeDate(r[0])
				if e != nil {
					return nil, e
				}
				date, _ := time.Parse("2006-01-02", d)
				if days == 0 {
					if date.Weekday() != time.Monday {
						return nil, errors.New("planner week must start Monday")
					}
					out.From = d
				} else {
					previous, _ := time.Parse("2006-01-02", day)
					if date != previous.AddDate(0, 0, 1) {
						return nil, errors.New("planner day inventory gap")
					}
				}
				days++
				day = d
				out.To = d
				continue
			}
			if len(r) != 11 || day == "" {
				return nil, errors.New("planner row drift")
			}
			var reference *CourseReference
			for _, ref := range page.CourseReferences {
				if ref.TableIndex == ti && ref.RowIndex == ri {
					if ref.Date != day {
						return nil, errors.New("planner reference date conflict")
					}
					if reference != nil && (reference.CourseID != ref.CourseID || reference.Planner != ref.Planner) {
						return nil, errors.New("planner reference identity conflict")
					}
					copy := ref
					reference = &copy
				}
			}
			if reference == nil {
				return nil, errors.New("planner native course reference missing")
			}
			key := reference.CourseID + "/" + day + "/" + reference.Planner
			if seen[key] {
				return nil, errors.New("duplicate planner session")
			}
			seen[key] = true
			ts := strings.Split(r[3], " - ")
			if len(ts) != 2 {
				return nil, errors.New("planner time drift")
			}
			for _, v := range ts {
				if _, e := time.Parse("15:04", v); e != nil {
					return nil, errors.New("invalid planner time")
				}
			}
			values := make([]int, 5)
			for i := range values {
				v, e := strconv.Atoi(r[5+i])
				if e != nil || (i < 4 && v < 0) {
					return nil, errors.New("invalid planner count")
				}
				values[i] = v
			}
			if values[0]-values[2] != values[4] {
				return nil, errors.New("planner capacity arithmetic conflict")
			}
			if r[0] == "" {
				return nil, errors.New("planner label missing")
			}
			out.Rows = append(out.Rows, CoursePlannerRow{CourseID: reference.CourseID, Planner: reference.Planner, Date: day, CourseLabel: r[0], StartTime: ts[0], EndTime: ts[1], Capacity: values[0], Linked: values[1], Registered: values[2], Cancelled: values[3], Free: values[4]})
		}
		if days != 7 {
			return nil, errors.New("planner week incomplete")
		}
	}
	if matches != 1 {
		return nil, errors.New("planner inventory table missing or ambiguous")
	}
	return out, nil
}
