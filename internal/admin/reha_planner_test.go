package admin

import (
	"testing"
	"time"
)

func syntheticPlanner() Page {
	p := Page{Tables: []Table{{Headers: []string{"", "Kurs", "Raum", "Kursleitung", "Uhrzeit", "", "Plätze", "verkettet", "belegt", "storniert", "frei", ""}}}}
	day, _ := time.Parse("2006-01-02", "2026-09-14")
	for i := 0; i < 7; i++ {
		p.Tables[0].Rows = append(p.Tables[0].Rows, []string{day.AddDate(0, 0, i).Format("02.01.2006"), "day"})
		if i == 1 {
			p.Tables[0].Rows = append(p.Tables[0].Rows, []string{"Reha Synthetic", "Raum", "private coach", "14:05 - 14:50", "", "20", "0", "0", "0", "20", ""})
			p.CourseReferences = append(p.CourseReferences, CourseReference{CourseID: "12345", Date: "2026-09-15", Planner: "A", TableIndex: 0, RowIndex: 2})
		}
	}
	return p
}
func TestPlannerInventoryZeroAndFailClosed(t *testing.T) {
	p := syntheticPlanner()
	f, e := parseCoursePlanner(p)
	if e != nil || len(f.Rows) != 1 || f.Rows[0].Registered != 0 || f.Rows[0].Capacity != 20 || f.To != "2026-09-20" {
		t.Fatalf("facts=%#v err=%v", f, e)
	}
	p.CourseReferences = append(p.CourseReferences, p.CourseReferences[0])
	if _, e := parseCoursePlanner(p); e != nil {
		t.Fatal("duplicate link must deduplicate")
	}
	p.Tables[0].Rows[2][9] = "19"
	if _, e := parseCoursePlanner(p); e == nil {
		t.Fatal("accepted arithmetic drift")
	}
	p = syntheticPlanner()
	p.Tables[0].Rows = p.Tables[0].Rows[:len(p.Tables[0].Rows)-1]
	if _, e := parseCoursePlanner(p); e == nil {
		t.Fatal("accepted missing day")
	}
}
