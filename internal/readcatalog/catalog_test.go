package readcatalog

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCatalogNamesAreUniqueAndSorted(t *testing.T) {
	items := List()
	if len(items) != 99 {
		t.Fatalf("expected 99 classified capabilities, got %d", len(items))
	}
	for index, item := range items {
		if item.Name == "" || item.Group == "" || item.Description == "" ||
			item.Path == "" || item.Method == "" {
			t.Fatalf("capability %d is incomplete: %+v", index, item)
		}
		if index > 0 && items[index-1].Name >= item.Name {
			t.Fatalf("catalog is not strictly sorted at %q", item.Name)
		}
	}
}

func TestEveryCatalogCapabilityBuildsAndValidates(t *testing.T) {
	for _, capability := range List() {
		values := make(map[string]string)
		for _, parameter := range capability.Parameters {
			if parameter.Name == "" || !parameter.Required {
				continue
			}
			switch parameter.Kind {
			case Date:
				values[parameter.Name] = "2026-07-01"
			case PositiveID:
				values[parameter.Name] = "1"
			case NonNegativeInt:
				values[parameter.Name] = "0"
			case Year:
				values[parameter.Name] = "2026"
			case Week:
				values[parameter.Name] = "1"
			case Enum:
				if len(parameter.Allowed) == 0 {
					t.Fatalf("%s parameter %s has no enum values", capability.Name, parameter.Name)
				}
				values[parameter.Name] = parameter.Allowed[0]
			case SearchText:
				values[parameter.Name] = "sample"
			case IKNumber:
				values[parameter.Name] = "123456789"
			default:
				t.Fatalf(
					"%s parameter %s has unsupported kind %q",
					capability.Name,
					parameter.Name,
					parameter.Kind,
				)
			}
		}
		request, err := Build(capability.Name, values)
		if err != nil {
			t.Fatalf("%s: %v", capability.Name, err)
		}
		if !ValidateRequest(request) {
			t.Fatalf("%s built an invalid request: %#v", capability.Name, request)
		}
	}
}

func TestBuildNormalizesDatesAndRejectsUnrelatedParameters(t *testing.T) {
	request, err := Build("course-attended", map[string]string{
		"from": "2026-07-01",
		"to":   "31.07.2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Path != "/Kursplaner_Auswertungen/Kurs_Teilnehmer_anwesend.asp" {
		t.Fatalf("unexpected path: %s", request.Path)
	}
	if request.Body != "bis=31.07.2026&von=01.07.2026" {
		t.Fatalf("unexpected form body: %s", request.Body)
	}

	_, err = Build("course-attended", map[string]string{
		"from":      "2026-07-01",
		"to":        "2026-07-31",
		"member-id": "12",
	})
	if err == nil || !strings.Contains(err.Error(), "does not accept --member-id") {
		t.Fatalf("expected unrelated parameter error, got %v", err)
	}
}

func TestBuildCreatesBoundedDynamicQuery(t *testing.T) {
	request, err := Build("course-session", map[string]string{
		"course-id": "41707",
		"date":      "2026-07-27",
		"planner":   "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse("https://www.azh-myyolo.info" + request.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateAdminURL(http.MethodGet, parsed) {
		t.Fatalf("built query was not accepted: %s", request.Path)
	}
	if parsed.Query().Get("Datum") != "27.07.2026" {
		t.Fatalf("date was not normalized: %s", parsed.RawQuery)
	}
}

func TestBuildRejectsReversedAndOversizedRanges(t *testing.T) {
	for _, values := range []map[string]string{
		{"from": "2026-07-31", "to": "2026-07-01"},
		{"from": "2024-01-01", "to": "2026-01-01"},
	} {
		_, err := Build("studio-hourly-load", values)
		if err == nil {
			t.Fatalf("range %#v was accepted", values)
		}
	}
	_, err := Build("attendance-week-range", map[string]string{
		"year":      "2026",
		"week-from": "20",
		"week-to":   "10",
	})
	if err == nil || !strings.Contains(err.Error(), "--week-to") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAdminURLRejectsUnknownAndMalformedQueries(t *testing.T) {
	tests := []string{
		"https://www.azh-myyolo.info/Mitglieder/Mitglied_aendern.asp?ID=12&edit=1",
		"https://www.azh-myyolo.info/Mitglieder/Mitglied_aendern.asp?ID=-1",
		"https://www.azh-myyolo.info/Suchen/Suchen.asp?Flag1=1&Start=1&Auswahl=1",
		"https://www.azh-myyolo.info/Abrechnung_Reha_Digital/Aktive/Digital_Reha_Aktive_Abgerechnete_Liste.asp?IK=123",
	}
	for _, rawURL := range tests {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if ValidateAdminURL(http.MethodGet, parsed) {
			t.Fatalf("unexpectedly allowed %s", rawURL)
		}
	}
}

func TestValidateRequestRejectsForgedFormBody(t *testing.T) {
	request, err := Build("studio-hourly-load", map[string]string{
		"from": "2026-07-01",
		"to":   "2026-07-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateRequest(request) {
		t.Fatal("built request was rejected")
	}
	request.Body += "&delete=1"
	if ValidateRequest(request) {
		t.Fatal("forged form body was accepted")
	}
}

func TestValidateRequestAcceptsOmittedOptionalFormField(t *testing.T) {
	request, err := Build("reha-prescription-summary", map[string]string{
		"from": "2026-07-01",
		"to":   "2026-07-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateRequest(request) {
		t.Fatalf("request with omitted optional referrer was rejected: %#v", request)
	}
}

func TestSensitivityRouteKeyRoundTrip(t *testing.T) {
	sensitivity, ok := SensitivityForRouteKey(RouteKey("member-bank-export"))
	if !ok || sensitivity != Financial {
		t.Fatalf("unexpected sensitivity: %q %t", sensitivity, ok)
	}
}

func TestSensitivityForStoredRouteUsesMostRestrictiveMatchingCapability(t *testing.T) {
	for _, test := range []struct {
		route string
		want  Sensitivity
	}{
		{"capability:attendance-monthly", Aggregate},
		{"/Anwesenheit/Anwesend_Aktuell_Liste.asp", Personal},
		{
			"/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend_Gesundheitsplaner.asp",
			Health,
		},
		{"/Excel/Sonstiges/E_Mitglieder_IBAN.asp", Financial},
	} {
		got, ok := SensitivityForStoredRoute(test.route)
		if !ok || got != test.want {
			t.Fatalf("route %q = %q, %t; want %q", test.route, got, ok, test.want)
		}
	}
	if _, ok := SensitivityForStoredRoute("/not-classified.asp"); ok {
		t.Fatal("unknown stored route was classified")
	}
}
