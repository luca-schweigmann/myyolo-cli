package admin

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestParsePageExtractsTablesAndSanitizedCapabilities(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<!doctype html>
		<html>
			<head><title>Attendance overview</title></head>
			<body>
				<h1>Today</h1>
				<a href="/Statistik.asp?member=123&amp;month=7">Details</a>
				<a href="https://example.com/outside">Outside</a>
				<form method="post" action="/Anmelden.asp?vw=">
					<input name="PartnerNummer">
					<input name="Benutzername">
					<input name="Password">
				</form>
				<table>
					<tr><th>Course</th><th>Present</th></tr>
					<tr><td>Example A</td><td>2</td></tr>
				</table>
			</body>
		</html>`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(DefaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := ParsePage(baseURL, "/start_Anwesenheit.asp", document)
	if err != nil {
		t.Fatal(err)
	}
	if page.Metadata.Title != "Attendance overview" ||
		len(page.Metadata.Headings) != 1 ||
		!page.Metadata.LoginForm {
		t.Fatalf("metadata = %#v", page.Metadata)
	}
	if len(page.Tables) != 1 ||
		len(page.Tables[0].Rows) != 1 ||
		page.Tables[0].Headers[0] != "Course" {
		t.Fatalf("tables = %#v", page.Tables)
	}
	if len(page.Metadata.Links) != 1 {
		t.Fatalf("links = %#v", page.Metadata.Links)
	}
	link := page.Metadata.Links[0]
	if link.Path != "/Statistik.asp" ||
		strings.Join(link.QueryKeys, ",") != "member,month" {
		t.Fatalf("sanitized link = %#v", link)
	}
	if strings.Contains(page.Metadata.Fingerprint, "Example A") ||
		len(page.Metadata.Fingerprint) != 64 {
		t.Fatalf("fingerprint = %q", page.Metadata.Fingerprint)
	}
}

func TestPageFingerprintDoesNotIncludeRowValues(t *testing.T) {
	baseURL, err := url.Parse(DefaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(value string) Page {
		t.Helper()
		document, parseErr := html.Parse(strings.NewReader(
			`<html><title>Overview</title><table>` +
				`<tr><th>Member</th></tr><tr><td>` + value + `</td></tr>` +
				`</table></html>`,
		))
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		page, pageErr := ParsePage(baseURL, "/start_Anwesenheit.asp", document)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		return page
	}
	first := parse("Synthetic One")
	second := parse("Synthetic Two")
	if first.Metadata.Fingerprint != second.Metadata.Fingerprint {
		t.Fatalf(
			"row values changed sanitized fingerprint: %s != %s",
			first.Metadata.Fingerprint,
			second.Metadata.Fingerprint,
		)
	}
}

func TestParsePageRejectsOversizedCell(t *testing.T) {
	document, err := html.Parse(strings.NewReader(
		`<table><tr><td>` + strings.Repeat("x", maxCellLength+1) + `</td></tr></table>`,
	))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(DefaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePage(baseURL, "/", document); err == nil {
		t.Fatal("expected oversized cell to fail closed")
	}
}

func TestParsePageKeepsLeafDataTableInsideLayoutTable(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`
		<table id="layout"><tr><td>
			<table id="data">
				<tr><th>Course</th><th>Count</th></tr>
				<tr><td>Synthetic</td><td>2</td></tr>
			</table>
		</td></tr></table>`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(DefaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := ParsePage(baseURL, "/", document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tables) != 1 ||
		strings.Join(page.Tables[0].Headers, ",") != "Course,Count" ||
		len(page.Tables[0].Rows) != 1 {
		t.Fatalf("tables = %#v", page.Tables)
	}
}
