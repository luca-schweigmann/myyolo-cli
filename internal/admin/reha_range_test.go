package admin

import (
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"strings"
	"testing"
)

func rangeFixture(rows string) string {
	return `<input name="von" value="01.07.2026"><input name="bis" value="31.07.2026"><table><tr><th></th><th>Kursbezeichnung</th><th>Raum</th><th>Anzahl</th><th>Zeit</th><th>Uhrzeit</th><th></th></tr>` + rows + `</table>`
}
func rangeRow(ord, id, count int) string {
	return fmt.Sprintf(`<tr><td>%d</td><td>Reha Synthetic</td><td>A</td><td>%d</td><td>01.07.2026</td><td>11:35 : 12:20</td><td><a href="/Kursplaner_Auswertungen/Kurs_Teilnehemer_Mitglieder_Anzeigen.asp?KursID=%d&amp;Datum=01.07.2026&amp;von=01.07.2026&amp;bis=31.07.2026">ansehen</a></td></tr>`, ord, count, id)
}
func parseRangeTest(source string) (Page, error) {
	u, _ := url.Parse("https://example.test")
	doc, _ := html.Parse(strings.NewReader(source))
	return ParsePage(u, "capability:course-attended", doc)
}
func TestCourseRangeTypedZeroAndExactIdentity(t *testing.T) {
	p, e := parseRangeTest(rangeFixture(rangeRow(1, 700001, 0) + rangeRow(2, 700002, 18)))
	if e != nil {
		t.Fatal(e)
	}
	f := p.CourseRangeFacts
	if f == nil || len(f.Rows) != 2 || f.Rows[0].Count != 0 || f.Rows[1].Count != 18 || f.Rows[0].BookingID == f.Rows[1].BookingID {
		t.Fatalf("invalid facts %#v", f)
	}
}
func TestCourseRangeFailClosed(t *testing.T) {
	valid := rangeFixture(rangeRow(1, 700001, 18))
	for name, source := range map[string]string{"missing-scope": strings.Replace(valid, `name="von"`, `name="other"`, 1), "drift": strings.Replace(valid, "Anzahl", "Anzahl neu", 1), "duplicate-row": rangeFixture(rangeRow(1, 700001, 18) + rangeRow(2, 700001, 18)), "gap": rangeFixture(rangeRow(2, 700001, 18)), "negative": strings.Replace(valid, "<td>18</td>", "<td>-1</td>", 1), "scope-conflict": strings.Replace(valid, "Datum=01.07.2026", "Datum=02.07.2026", 1), "pagination": valid + `<a href="?page=2">next</a>`, "unknown-empty": rangeFixture("")} {
		t.Run(name, func(t *testing.T) {
			if _, e := parseRangeTest(source); e == nil {
				t.Fatal("accepted drift")
			}
		})
	}
}
func TestCourseRangeDuplicateAnchorsAndExplicitEmpty(t *testing.T) {
	row := rangeRow(1, 700001, 18)
	start := strings.Index(row, "<a ")
	end := strings.Index(row, "</a>") + 4
	duplicate := strings.Replace(row, "</td></tr>", row[start:end]+"</td></tr>", 1)
	p, e := parseRangeTest(rangeFixture(duplicate))
	if e != nil || len(p.CourseRangeFacts.Rows) != 1 {
		t.Fatalf("duplicate anchor: %v", e)
	}
	p, e = parseRangeTest(`<input name="von" value="01.07.2026"><input name="bis" value="31.07.2026"><table><tr><td>keine Daten vorhanden</td></tr></table>`)
	if e != nil || len(p.CourseRangeFacts.Rows) != 0 {
		t.Fatalf("empty: %v", e)
	}
}

func TestCourseRangeRejectsOverlongNativeID(t *testing.T) {
	source := strings.Replace(rangeFixture(rangeRow(1, 700001, 18)), "KursID=700001", "KursID=1234567890123", 1)
	if _, e := parseRangeTest(source); e == nil {
		t.Fatal("accepted overlong native ID")
	}
}
