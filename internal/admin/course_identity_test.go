package admin

import (
	"golang.org/x/net/html"
	"net/url"
	"strings"
	"testing"
)

func TestPlannerPreservesOnlyCourseIdentityAtExactRow(t *testing.T) {
	base, _ := url.Parse("https://www.azh-myyolo.info")
	doc, _ := html.Parse(strings.NewReader(`<table><tr><th>Kurs</th></tr><tr><td>14.09.2026</td></tr><tr><td><a href="/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=123&amp;Datum=14.09.2026&amp;defaultMode=A">Synthetic course</a><a href="/Mitglieder/Mitglied_aendern.asp?ID=999">Private member</a></td></tr></table>`))
	page, err := ParsePage(base, "capability:course-planner-day", doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.CourseReferences) != 1 {
		t.Fatal("unexpected course identity count")
	}
	ref := page.CourseReferences[0]
	if ref.CourseID != "123" || ref.Date != "2026-09-14" || ref.Planner != "A" || ref.TableIndex != 0 || ref.RowIndex != 1 {
		t.Fatalf("identity not bound to exact row: %+v", ref)
	}
	for _, href := range []string{
		"https://untrusted.invalid/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=123&Datum=14.09.2026&defaultMode=A",
		"/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=123&Datum=31.02.2026&defaultMode=A",
		"/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=123&Datum=14.09.2026&defaultMode=unknown",
	} {
		if _, ok := courseReference(base, href); ok {
			t.Fatal("unqualified identity accepted")
		}
	}
}

func TestDailyPlannerCourseReferencePreservesAbsentPlanner(t *testing.T) {
	base, _ := url.Parse("https://www.azh-myyolo.info")
	doc, _ := html.Parse(strings.NewReader(`<table><tr><th>Kurs</th></tr><tr><td><a href="/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp?Kurs=321&amp;Datum=16.09.2026">Synthetic</a></td></tr></table>`))
	page, err := ParsePage(base, "capability:course-planner-day", doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.CourseReferences) != 1 {
		t.Fatalf("missing exact daily course reference: %#v", page.CourseReferences)
	}
	ref := page.CourseReferences[0]
	if ref.CourseID != "321" || ref.Date != "2026-09-16" || ref.Planner != "" {
		t.Fatalf("fabricated/missing identity: %#v", ref)
	}
}

func TestCourseAttendanceIndicatorsAreScopedAndPrivate(t *testing.T) {
	base, _ := url.Parse("https://www.azh-myyolo.info")
	source := syntheticRoster(1, func(int) string { return `<img src="anwesend_haken.png">` }, func(int) string { return `<img src="unterschrift_gruen.png">` })
	doc, _ := html.Parse(strings.NewReader(source))
	page, err := ParsePage(base, "capability:course-session", doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.CourseAttendanceIndicators) != 1 {
		t.Fatalf("indicators=%#v", page.CourseAttendanceIndicators)
	}
	item := page.CourseAttendanceIndicators[0]
	if item.RowIndex != 0 || len(item.ImageAssets) != 1 || item.ImageAssets[0] != "anwesend_haken.png" || len(item.Checkboxes) != 0 {
		t.Fatalf("unsafe or wrong evidence: %#v", item)
	}
	doc, _ = html.Parse(strings.NewReader(source))
	page, err = ParsePage(base, "capability:course-day-list", doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.CourseAttendanceIndicators) != 0 {
		t.Fatal("indicator scope leaked")
	}
}
