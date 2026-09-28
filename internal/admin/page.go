package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const (
	maxTables     = 100
	maxRows       = 20_000
	maxCells      = 200
	maxCellLength = 16 << 10
	maxTargets    = 2_000
)

type Page struct {
	CoursePlannerFacts    *CoursePlannerFacts    `json:"course_planner_facts,omitempty"`
	CourseRangeFacts      *CourseRangeFacts      `json:"course_range_facts,omitempty"`
	CourseSessionIdentity *CourseSessionIdentity `json:"course_session_identity,omitempty"`
	CourseSessionFacts    *CourseSessionFacts    `json:"course_session_facts,omitempty"`
	Metadata              PageMetadata           `json:"metadata"`
	Tables                []Table                `json:"tables,omitempty"`
	// Private provenance; never included in normal CLI output. Only validated
	// read-capability requests can populate these fields (no login/session data).
	RequestPath                string                      `json:"-"`
	RequestBody                string                      `json:"-"`
	CourseReferences           []CourseReference           `json:"course_references,omitempty"`
	CourseAttendanceIndicators []CourseAttendanceIndicator `json:"course_attendance_indicators,omitempty"`
}

// Only course-target identity, never member links, names or signature material.
type CourseReference struct {
	CourseID   string `json:"course_id"`
	Date       string `json:"date"`
	Planner    string `json:"planner,omitempty"`
	TableIndex int    `json:"table_index"`
	RowIndex   int    `json:"row_index"`
}

// Static UI evidence from the AW column only. No member IDs, URLs, form
// names/values or signature material are retained in these indicators.
type CourseAttendanceIndicator struct {
	TableIndex  int      `json:"table_index"`
	RowIndex    int      `json:"row_index"`
	ImageAssets []string `json:"image_assets,omitempty"`
	Checkboxes  []bool   `json:"checkboxes,omitempty"`
}

type PageMetadata struct {
	Route        string     `json:"route"`
	Title        string     `json:"title,omitempty"`
	Headings     []string   `json:"headings,omitempty"`
	TableHeaders [][]string `json:"table_headers,omitempty"`
	TableRows    []int      `json:"table_rows,omitempty"`
	Links        []Target   `json:"links,omitempty"`
	Forms        []Target   `json:"forms,omitempty"`
	LoginForm    bool       `json:"login_form"`
	Fingerprint  string     `json:"fingerprint"`
}

type Target struct {
	Method    string   `json:"method"`
	Path      string   `json:"path"`
	QueryKeys []string `json:"query_keys,omitempty"`
}

type Table struct {
	Headers []string   `json:"headers,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
}

func ParsePage(baseURL *url.URL, route string, document *html.Node) (Page, error) {
	if baseURL == nil || document == nil {
		return Page{}, fmt.Errorf("base URL and HTML document are required")
	}
	page := Page{Metadata: PageMetadata{Route: route}}
	var tableNodes []*html.Node
	var linkTargets []Target
	var formTargets []Target
	var formFields []map[string]struct{}

	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "title":
				if page.Metadata.Title == "" {
					page.Metadata.Title = nodeText(node)
				}
			case "h1", "h2", "h3", "h4":
				if heading := nodeText(node); heading != "" {
					page.Metadata.Headings = append(page.Metadata.Headings, heading)
				}
			case "table":
				if !hasDescendantTable(node) {
					tableNodes = append(tableNodes, node)
					if len(tableNodes) > maxTables {
						return fmt.Errorf("page exceeds %d leaf tables", maxTables)
					}
				}
			case "a":
				if target, ok := sanitizedTarget(baseURL, "GET", attr(node, "href")); ok {
					linkTargets = append(linkTargets, target)
				}
			case "form":
				method := strings.ToUpper(strings.TrimSpace(attr(node, "method")))
				if method == "" {
					method = "GET"
				}
				if target, ok := sanitizedTarget(baseURL, method, attr(node, "action")); ok {
					formTargets = append(formTargets, target)
				}
				fields := make(map[string]struct{})
				collectFormFields(node, fields)
				formFields = append(formFields, fields)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(document); err != nil {
		return Page{}, err
	}

	for tableIndex, tableNode := range tableNodes {
		table, err := parseTable(tableNode)
		if err != nil {
			return Page{}, err
		}
		page.Tables = append(page.Tables, table)
		page.Metadata.TableHeaders = append(page.Metadata.TableHeaders, table.Headers)
		page.Metadata.TableRows = append(page.Metadata.TableRows, len(table.Rows))
		if route == "capability:course-session" {
			page.CourseAttendanceIndicators = append(page.CourseAttendanceIndicators, tableAttendanceIndicators(tableNode, tableIndex, table.Headers)...)
		}
		if strings.HasPrefix(route, "capability:course-planner-") {
			page.CourseReferences = append(page.CourseReferences, tableCourseReferences(baseURL, tableNode, tableIndex)...)
		}
	}
	if route == "capability:course-planner-week" {
		facts, err := parseCoursePlanner(page)
		if err != nil {
			return Page{}, err
		}
		page.CoursePlannerFacts = facts
	}
	if facts, err := parseCourseRange(baseURL, route, document, tableNodes); err != nil {
		return Page{}, err
	} else {
		page.CourseRangeFacts = facts
	}
	if route == "capability:course-session" {
		facts, err := parseCourseSessionFacts(tableNodes)
		if err != nil {
			return Page{}, err
		}
		page.CourseSessionFacts = facts
		identity, err := parseCourseSessionIdentity(document)
		if err != nil {
			return Page{}, err
		}
		page.CourseSessionIdentity = identity
	}
	page.Metadata.Links = uniqueTargets(linkTargets)
	page.Metadata.Forms = uniqueTargets(formTargets)
	if len(page.Metadata.Links)+len(page.Metadata.Forms) > maxTargets {
		return Page{}, fmt.Errorf("page exceeds %d sanitized targets", maxTargets)
	}
	for _, fields := range formFields {
		if hasFields(fields, "partnernummer", "benutzername", "password") {
			page.Metadata.LoginForm = true
			break
		}
	}
	fingerprintInput := struct {
		Route        string
		Title        string
		Headings     []string
		TableHeaders [][]string
		Forms        []Target
	}{
		Route:        page.Metadata.Route,
		Title:        page.Metadata.Title,
		Headings:     page.Metadata.Headings,
		TableHeaders: page.Metadata.TableHeaders,
		Forms:        page.Metadata.Forms,
	}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil {
		return Page{}, fmt.Errorf("fingerprint page metadata: %w", err)
	}
	hash := sha256.Sum256(encoded)
	page.Metadata.Fingerprint = hex.EncodeToString(hash[:])
	return page, nil
}

func tableCourseReferences(base *url.URL, table *html.Node, tableIndex int) []CourseReference {
	var refs []CourseReference
	rowIndex := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" && nearestAncestor(n.Parent, "table") == table {
			header, cells := false, 0
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells++
					header = header || c.Data == "th"
				}
			}
			if cells > 0 && !(header && rowIndex == 0) {
				var anchors func(*html.Node)
				anchors = func(a *html.Node) {
					if a.Type == html.ElementNode && a.Data == "a" {
						if ref, ok := courseReference(base, attr(a, "href")); ok {
							ref.TableIndex = tableIndex
							ref.RowIndex = rowIndex
							refs = append(refs, ref)
						}
					}
					for c := a.FirstChild; c != nil; c = c.NextSibling {
						anchors(c)
					}
				}
				anchors(n)
				rowIndex++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return refs
}

func courseReference(base *url.URL, href string) (CourseReference, bool) {
	target, err := base.Parse(href)
	if err != nil || !strings.EqualFold(target.Hostname(), base.Hostname()) || target.Path != "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp" {
		return CourseReference{}, false
	}
	query := target.Query()
	id, when, planner := query.Get("Kurs"), query.Get("Datum"), query.Get("defaultMode")
	if id == "" || len(id) > 12 || strings.Trim(id, "0123456789") != "" || (planner != "" && planner != "A" && planner != "B") {
		return CourseReference{}, false
	}
	var parsed time.Time
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006"} {
		if parsed, err = time.Parse(layout, when); err == nil {
			break
		}
	}
	if err != nil {
		return CourseReference{}, false
	}
	return CourseReference{CourseID: id, Date: parsed.Format("2006-01-02"), Planner: planner}, true
}

func parseTable(tableNode *html.Node) (Table, error) {
	var rows [][]string
	var headerRows [][]string
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "tr") &&
			nearestAncestor(node.Parent, "table") == tableNode {
			var cells []string
			hasHeader := false
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type != html.ElementNode {
					continue
				}
				tag := strings.ToLower(child.Data)
				if tag != "td" && tag != "th" {
					continue
				}
				if tag == "th" {
					hasHeader = true
				}
				value := nodeText(child)
				if len(value) > maxCellLength {
					return fmt.Errorf("table cell exceeds %d bytes", maxCellLength)
				}
				cells = append(cells, value)
				if len(cells) > maxCells {
					return fmt.Errorf("table row exceeds %d cells", maxCells)
				}
			}
			if len(cells) > 0 {
				if hasHeader && len(rows) == 0 {
					headerRows = append(headerRows, cells)
				} else {
					rows = append(rows, cells)
					if len(rows) > maxRows {
						return fmt.Errorf("table exceeds %d rows", maxRows)
					}
				}
			}
			return nil
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && strings.EqualFold(child.Data, "table") &&
				child != tableNode {
				continue
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(tableNode); err != nil {
		return Table{}, err
	}
	var headers []string
	for _, row := range headerRows {
		headers = append(headers, row...)
	}
	return Table{Headers: headers, Rows: rows}, nil
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			if text := normalizeSpace(current.Data); text != "" {
				if builder.Len() > 0 {
					builder.WriteByte(' ')
				}
				builder.WriteString(text)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return normalizeSpace(builder.String())
}

func normalizeSpace(value string) string {
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func attr(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func nearestAncestor(node *html.Node, tag string) *html.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, tag) {
			return current
		}
	}
	return nil
}

func hasDescendantTable(node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && strings.EqualFold(child.Data, "table") {
			return true
		}
		if hasDescendantTable(child) {
			return true
		}
	}
	return false
}

func collectFormFields(node *html.Node, fields map[string]struct{}) {
	if node.Type == html.ElementNode {
		switch strings.ToLower(node.Data) {
		case "input", "select", "textarea", "button":
			if name := strings.ToLower(attr(node, "name")); name != "" {
				fields[name] = struct{}{}
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectFormFields(child, fields)
	}
}

func hasFields(fields map[string]struct{}, names ...string) bool {
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func sanitizedTarget(baseURL *url.URL, method string, rawTarget string) (Target, bool) {
	if rawTarget == "" || strings.HasPrefix(rawTarget, "#") ||
		strings.HasPrefix(strings.ToLower(rawTarget), "javascript:") {
		return Target{}, false
	}
	parsed, err := url.Parse(rawTarget)
	if err != nil {
		return Target{}, false
	}
	resolved := baseURL.ResolveReference(parsed)
	if resolved.Scheme != "https" ||
		!strings.EqualFold(resolved.Hostname(), baseURL.Hostname()) ||
		(resolved.Port() != "" && resolved.Port() != "443") {
		return Target{}, false
	}
	keys := make([]string, 0, len(resolved.Query()))
	for key := range resolved.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return Target{
		Method:    strings.ToUpper(method),
		Path:      resolved.EscapedPath(),
		QueryKeys: keys,
	}, true
}

func uniqueTargets(targets []Target) []Target {
	seen := make(map[string]struct{}, len(targets))
	unique := make([]Target, 0, len(targets))
	for _, target := range targets {
		key := target.Method + "\x00" + target.Path + "\x00" + strings.Join(target.QueryKeys, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, target)
	}
	sort.Slice(unique, func(left, right int) bool {
		if unique[left].Path != unique[right].Path {
			return unique[left].Path < unique[right].Path
		}
		if unique[left].Method != unique[right].Method {
			return unique[left].Method < unique[right].Method
		}
		return strings.Join(unique[left].QueryKeys, "\x00") <
			strings.Join(unique[right].QueryKeys, "\x00")
	})
	return unique
}

var staticAttendanceAsset = regexp.MustCompile(`^[a-zA-Z_-]+\.(gif|png|jpg|jpeg|svg)$`)

func tableAttendanceIndicators(table *html.Node, tableIndex int, headers []string) []CourseAttendanceIndicator {
	awIndex := -1
	for i, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), "AW") {
			awIndex = i
		}
	}
	if awIndex < 0 {
		return nil
	}
	var result []CourseAttendanceIndicator
	rowIndex := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" && nearestAncestor(n.Parent, "table") == table {
			var cells []*html.Node
			header := false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, c)
					header = header || c.Data == "th"
				}
			}
			if len(cells) > 0 && !(header && rowIndex == 0) {
				if len(cells) == len(headers) {
					item := CourseAttendanceIndicator{TableIndex: tableIndex, RowIndex: rowIndex}
					var extract func(*html.Node)
					extract = func(c *html.Node) {
						if c.Type == html.ElementNode && c.Data == "img" {
							u, err := url.Parse(attr(c, "src"))
							if err == nil && u.RawQuery == "" {
								asset := path.Base(u.Path)
								if staticAttendanceAsset.MatchString(asset) {
									item.ImageAssets = append(item.ImageAssets, asset)
								}
							}
						}
						if c.Type == html.ElementNode && c.Data == "input" && strings.EqualFold(attr(c, "type"), "checkbox") {
							checked := false
							for _, a := range c.Attr {
								if strings.EqualFold(a.Key, "checked") {
									checked = true
								}
							}
							item.Checkboxes = append(item.Checkboxes, checked)
						}
						for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
							extract(ch)
						}
					}
					extract(cells[awIndex])
					if len(item.ImageAssets) > 0 || len(item.Checkboxes) > 0 {
						result = append(result, item)
					}
				}
				rowIndex++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return result
}
