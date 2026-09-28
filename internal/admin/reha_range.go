package admin

import (
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CourseRangeFacts is private source evidence. BookingID is never a recurring
// course ID. Normal management exports pseudonymize it and omit member data.
type CourseRangeFacts struct {
	Kind         string           `json:"kind"`
	From         string           `json:"from"`
	To           string           `json:"to"`
	Completeness string           `json:"completeness"`
	Rows         []CourseRangeRow `json:"rows"`
}
type CourseRangeRow struct {
	BookingID   string `json:"booking_id"`
	CourseLabel string `json:"course_label"`
	Date        string `json:"date"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Count       int    `json:"count"`
}

var rangeTime = regexp.MustCompile(`^([0-2][0-9]:[0-5][0-9]) : ([0-2][0-9]:[0-5][0-9])$`)
var rangeID = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)

const rangeMemberPath = "/Kursplaner_Auswertungen/Kurs_Teilnehemer_Mitglieder_Anzeigen.asp"

func rangeDate(value string) (string, error) {
	d, e := time.Parse("02.01.2006", value)
	if e != nil {
		return "", errors.New("invalid range date")
	}
	return d.Format("2006-01-02"), nil
}
func parseCourseRange(base *url.URL, route string, doc *html.Node, tables []*html.Node) (*CourseRangeFacts, error) {
	kind := strings.TrimPrefix(route, "capability:course-")
	countHeader, ok := map[string]string{"attended": "Anzahl", "not-attended": "Anzahl nAW", "cancelled": "Anzahl Storno"}[kind]
	if !ok {
		return nil, nil
	}
	result := &CourseRangeFacts{Kind: kind, Completeness: "validated_complete_range", Rows: []CourseRangeRow{}}
	fields := map[string][]string{}
	var validationErr error
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			name := strings.ToLower(attr(n, "name"))
			if n.Data == "input" && (name == "von" || name == "bis") {
				fields[name] = append(fields[name], attr(n, "value"))
			}
			if n.Data == "a" {
				u, e := url.Parse(attr(n, "href"))
				if e == nil {
					for k := range u.Query() {
						switch strings.ToLower(k) {
						case "page", "seite", "offset", "cursor", "start", "next":
							validationErr = errors.New("range pagination is not supported")
						}
					}
				}
				switch strings.ToLower(attr(n, "rel")) {
				case "next", "prev":
					validationErr = errors.New("range pagination is not supported")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if validationErr != nil {
		return nil, validationErr
	}
	if len(fields["von"]) != 1 || len(fields["bis"]) != 1 {
		return nil, errors.New("range scope echo missing or ambiguous")
	}
	var err error
	result.From, err = rangeDate(fields["von"][0])
	if err != nil {
		return nil, err
	}
	result.To, err = rangeDate(fields["bis"][0])
	if err != nil || result.From > result.To {
		return nil, errors.New("invalid range scope echo")
	}
	matches, empty := 0, 0
	seen := map[string]bool{}
	for _, table := range tables {
		parsed, e := parseTable(table)
		if e != nil {
			return nil, e
		}
		if nodeText(table) == "keine Daten vorhanden" {
			empty++
			continue
		}
		candidate := false
		for _, h := range parsed.Headers {
			if h == "Kursbezeichnung" {
				candidate = true
			}
		}
		if !candidate {
			continue
		}
		matches++
		if !reflect.DeepEqual(parsed.Headers, []string{"", "Kursbezeichnung", "Raum", countHeader, "Zeit", "Uhrzeit", ""}) {
			return nil, errors.New("range header drift")
		}
		ordinal := 0
		var rows func(*html.Node) error
		rows = func(n *html.Node) error {
			if n.Type == html.ElementNode && n.Data == "tr" && nearestAncestor(n.Parent, "table") == table {
				var cells []*html.Node
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
						cells = append(cells, c)
					}
				}
				if len(cells) > 0 && cells[0].Data == "th" {
					return nil
				}
				if len(cells) == 0 {
					return nil
				}
				if len(cells) != 7 {
					return errors.New("range row shape drift")
				}
				ordinal++
				if nodeText(cells[0]) != strconv.Itoa(ordinal) {
					return errors.New("range row inventory incomplete or duplicated")
				}
				count, e := strconv.Atoi(nodeText(cells[3]))
				if e != nil || count < 0 {
					return errors.New("invalid range count")
				}
				date, e := rangeDate(nodeText(cells[4]))
				if e != nil || date < result.From || date > result.To {
					return errors.New("range row outside source scope")
				}
				times := rangeTime.FindStringSubmatch(nodeText(cells[5]))
				if len(times) != 3 {
					return errors.New("range time drift")
				}
				for _, v := range times[1:] {
					if _, e := time.Parse("15:04", v); e != nil {
						return errors.New("invalid range time")
					}
				}
				label := nodeText(cells[1])
				if label == "" {
					return errors.New("missing range course label")
				}
				booking := ""
				anchors := 0
				var links func(*html.Node) error
				links = func(a *html.Node) error {
					if a.Type == html.ElementNode && a.Data == "a" {
						u, e := url.Parse(attr(a, "href"))
						if e != nil {
							return errors.New("invalid range reference")
						}
						u = base.ResolveReference(u)
						if u.Path == rangeMemberPath {
							anchors++
							if u.Scheme != base.Scheme || u.Host != base.Host {
								return errors.New("foreign range reference")
							}
							q := u.Query()
							if len(q) != 4 {
								return errors.New("range reference drift")
							}
							for _, k := range []string{"KursID", "Datum", "von", "bis"} {
								if len(q[k]) != 1 {
									return errors.New("ambiguous range reference")
								}
							}
							d, e := rangeDate(q.Get("Datum"))
							if e != nil || d != date || q.Get("von") != fields["von"][0] || q.Get("bis") != fields["bis"][0] || !rangeID.MatchString(q.Get("KursID")) {
								return errors.New("range identity scope conflict")
							}
							if booking != "" && booking != q.Get("KursID") {
								return errors.New("conflicting duplicate anchors")
							}
							booking = q.Get("KursID")
						}
					}
					for c := a.FirstChild; c != nil; c = c.NextSibling {
						if e := links(c); e != nil {
							return e
						}
					}
					return nil
				}
				if e := links(n); e != nil {
					return e
				}
				if anchors == 0 || seen[booking] {
					return errors.New("missing or duplicate native booking")
				}
				seen[booking] = true
				result.Rows = append(result.Rows, CourseRangeRow{BookingID: booking, CourseLabel: label, Date: date, StartTime: times[1], EndTime: times[2], Count: count})
				return nil
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if e := rows(c); e != nil {
					return e
				}
			}
			return nil
		}
		if e := rows(table); e != nil {
			return nil, e
		}
		if ordinal != len(parsed.Rows) {
			return nil, errors.New("range row inventory mismatch")
		}
	}
	if (matches != 1 || empty != 0) && (matches != 0 || empty != 1) {
		return nil, fmt.Errorf("ambiguous range collection: tables=%d empty=%d", matches, empty)
	}
	if matches == 1 && len(result.Rows) == 0 {
		return nil, errors.New("empty range requires explicit no-data marker")
	}
	return result, nil
}
