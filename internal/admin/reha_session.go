package admin

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CourseSessionFacts contains no person identifiers. Counts describe exactly
// the observed native Admin roster, not a mySIGN join or calendar completeness.
type CourseSessionFacts struct {
	Registered         int    `json:"registered"`
	AttendanceMarked   int    `json:"attendance_marked"`
	SignedAttendance   int    `json:"signed_attendance"`
	Cancelled          *int   `json:"cancelled"`
	Capacity           *int   `json:"capacity"`
	Completeness       string `json:"completeness"`
	CancellationStatus string `json:"cancellation_status"`
}

var courseSessionHeaders = []string{"", "Zeitraum", "Raum", "TN", "MNr.", "Teilnehmer", "Einheiten", "AW", "Terminplanung", "Bemerkung", ""}

func parseCourseSessionFacts(tables []*html.Node) (*CourseSessionFacts, error) {
	var selected *html.Node
	for _, table := range tables {
		parsed, err := parseTable(table)
		if err != nil {
			return nil, err
		}
		if sameHeaders(parsed.Headers, courseSessionHeaders) {
			if selected != nil {
				return nil, errors.New("ambiguous Admin course roster tables")
			}
			selected = table
		}
	}
	if selected == nil {
		return nil, errors.New("unsupported Admin course roster schema")
	}
	facts := &CourseSessionFacts{Completeness: "validated_observed_roster", CancellationStatus: "unknown_not_provided"}
	seenMembers := map[string]bool{}
	var parseErr error
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if parseErr != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "tr" && nearestAncestor(n.Parent, "table") == selected {
			var cells []*html.Node
			header := false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, c)
					header = header || c.Data == "th"
				}
			}
			if len(cells) == 0 || header {
				return
			}
			texts := make([]string, len(cells))
			nonempty := false
			for i, c := range cells {
				texts[i] = strings.TrimSpace(nodeText(c))
				nonempty = nonempty || texts[i] != ""
			}
			if !nonempty {
				return
			}
			// The observed source has one ten-cell course/time introduction row.
			if len(cells) == 10 && texts[0] == "" && strings.Contains(texts[1], " - ") {
				return
			}
			if len(cells) != len(courseSessionHeaders) {
				parseErr = errors.New("unsupported Admin course roster row shape")
				return
			}
			number, err := strconv.Atoi(texts[0])
			if err != nil || number != facts.Registered+1 {
				parseErr = errors.New("noncontiguous or duplicate Admin course roster row")
				return
			}
			if texts[4] == "" || seenMembers[texts[4]] {
				parseErr = errors.New("missing or duplicate Admin roster member reference")
				return
			}
			seenMembers[texts[4]] = true
			attended, err := parseAttendanceCell(cells[7])
			if err != nil {
				parseErr = fmt.Errorf("Admin attendance row %d: %w", number, err)
				return
			}
			signed, err := parseSignatureCell(cells[8])
			if err != nil {
				parseErr = fmt.Errorf("Admin signature row %d: %w", number, err)
				return
			}
			facts.Registered++
			if attended {
				facts.AttendanceMarked++
				if signed {
					facts.SignedAttendance++
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(selected)
	if parseErr != nil {
		return nil, parseErr
	}
	return facts, nil
}

func sameHeaders(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != b[i] {
			return false
		}
	}
	return true
}

func statusCellAssets(cell *html.Node) ([]string, error) {
	var assets []string
	var parseErr error
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				u, err := url.Parse(attr(n, "src"))
				if err != nil || u.RawQuery != "" {
					parseErr = errors.New("unsupported Admin status image reference")
					return
				}
				assets = append(assets, path.Base(u.Path))
			case "button":
				if !strings.EqualFold(attr(n, "type"), "button") {
					parseErr = errors.New("unsupported Admin status button")
					return
				}
			case "input", "select", "textarea":
				parseErr = errors.New("unsupported Admin status control")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)
	return assets, parseErr
}
func parseAttendanceCell(cell *html.Node) (bool, error) {
	if strings.TrimSpace(nodeText(cell)) != "" {
		return false, errors.New("unsupported Admin attendance text")
	}
	assets, err := statusCellAssets(cell)
	if err != nil {
		return false, err
	}
	count := 0
	for _, asset := range assets {
		switch asset {
		case "anwesend_haken.png":
			count++
		case "spacer.gif":
		default:
			return false, errors.New("unknown Admin attendance marker")
		}
	}
	if count > 1 {
		return false, errors.New("duplicate Admin attendance marker")
	}
	return count == 1, nil
}
func parseSignatureCell(cell *html.Node) (bool, error) {
	if strings.TrimSpace(nodeText(cell)) != "" {
		return false, errors.New("unsupported Admin signature text")
	}
	assets, err := statusCellAssets(cell)
	if err != nil {
		return false, err
	}
	markers := 0
	signed := false
	for _, asset := range assets {
		switch asset {
		case "unterschrift_gruen.png":
			markers++
			signed = true
		case "unterschrift_rot.png", "Anmelden.png":
			markers++
		case "storno.png", "serienbuchung.png", "termin_loeschen.png", "insert_link_black_24dp.svg", "spacer.gif":
		default:
			return false, errors.New("unknown Admin signature marker")
		}
	}
	if markers != 1 {
		return false, errors.New("missing or duplicate Admin signature status")
	}
	return signed, nil
}

// Private response identity: the detail request uses the recurring course ID,
// while hidden Kurs and the source action's KursBuchungID identify this booking.
type CourseSessionIdentity struct {
	BookingID string `json:"booking_id"`
	Date      string `json:"date"`
}

var nativeBookingIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)

func parseCourseSessionIdentity(document *html.Node) (*CourseSessionIdentity, error) {
	values := map[string][]string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" && strings.EqualFold(attr(n, "type"), "hidden") {
			name := attr(n, "name")
			if name == "Kurs" || name == "Datum" {
				values[name] = append(values[name], attr(n, "value"))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(document)
	if len(values["Kurs"]) != 1 || len(values["Datum"]) != 1 || !nativeBookingIDPattern.MatchString(values["Kurs"][0]) {
		return nil, errors.New("missing or ambiguous native Admin booking identity")
	}
	day, err := time.Parse("02.01.2006", values["Datum"][0])
	if err != nil {
		return nil, errors.New("invalid native Admin booking date")
	}
	return &CourseSessionIdentity{BookingID: values["Kurs"][0], Date: day.Format("2006-01-02")}, nil
}
