package admin

import (
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"strings"
	"testing"
)

func syntheticRoster(rows int, aw, signed func(int) string) string {
	var b strings.Builder
	b.WriteString(`<input type="hidden" name="Kurs" value="321001"><input type="hidden" name="Datum" value="14.09.2026"><table><tr>`)
	for _, h := range courseSessionHeaders {
		fmt.Fprintf(&b, "<th>%s</th>", h)
	}
	b.WriteString("</tr>")
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&b, `<tr><td>%d</td><td></td><td><img src="green.png"></td><td>2</td><td>%d</td><td>Synthetic person</td><td></td><td>%s</td><td><button type="button"><a href="/ignored?id=123">%s</a></button><a href="/ignored?id=123"></a></td><td></td><td></td></tr>`, i, 1000+i, aw(i), signed(i))
	}
	b.WriteString("</table>")
	return b.String()
}
func parseSyntheticRoster(t *testing.T, s string) (Page, error) {
	t.Helper()
	base, _ := url.Parse("https://www.azh-myyolo.info")
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return ParsePage(base, "capability:course-session", doc)
}
func TestAdminCourseRosterFacts21Registered18SignedAttendance(t *testing.T) {
	source := syntheticRoster(21, func(i int) string {
		if i <= 18 {
			return `<img src="../images/anwesend_haken.png">`
		}
		return ""
	}, func(i int) string {
		if i <= 18 {
			return `<img src="../images/unterschrift_gruen.png">`
		}
		return `<img src="../images/Anmelden.png">`
	})
	page, err := parseSyntheticRoster(t, source)
	if err != nil {
		t.Fatal(err)
	}
	f := page.CourseSessionFacts
	if f == nil || f.Registered != 21 || f.AttendanceMarked != 18 || f.SignedAttendance != 18 || f.Capacity != nil || f.Cancelled != nil {
		t.Fatalf("facts=%#v", f)
	}
}
func TestAdminCourseRosterZeroAndUnsignedAreNotMissing(t *testing.T) {
	page, err := parseSyntheticRoster(t, syntheticRoster(0, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if page.CourseSessionFacts.Registered != 0 || page.CourseSessionFacts.SignedAttendance != 0 {
		t.Fatal("zero roster failed")
	}
	source := syntheticRoster(1, func(int) string { return `<img src="anwesend_haken.png">` }, func(int) string { return `<img src="unterschrift_rot.png">` })
	page, err = parseSyntheticRoster(t, source)
	if err != nil {
		t.Fatal(err)
	}
	if page.CourseSessionFacts.AttendanceMarked != 1 || page.CourseSessionFacts.SignedAttendance != 0 {
		t.Fatal("unsigned attendance counted")
	}
}
func TestAdminCourseRosterRejectsUnknownDuplicateAndDrift(t *testing.T) {
	valid := syntheticRoster(2, func(int) string { return `<img src="anwesend_haken.png">` }, func(int) string { return `<img src="unterschrift_gruen.png">` })
	for name, source := range map[string]string{
		"unknown attendance": strings.Replace(valid, "anwesend_haken.png", "red.png", 1),
		"unknown signature":  strings.Replace(valid, "unterschrift_gruen.png", "unknown.png", 1),
		"duplicate row":      strings.Replace(valid, "<td>2</td><td></td>", "<td>1</td><td></td>", 1),
		"duplicate member":   strings.Replace(valid, "<td>1002</td>", "<td>1001</td>", 1),
		"header drift":       strings.Replace(valid, "<th>AW</th>", "<th>Other</th>", 1),
		"duplicate table":    valid + valid,
		"missing signature":  strings.Replace(valid, `<img src="unterschrift_gruen.png">`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSyntheticRoster(t, source); err == nil {
				t.Fatal("drift/ambiguity accepted")
			}
		})
	}
}
