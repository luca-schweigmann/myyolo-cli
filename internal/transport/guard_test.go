package transport

import (
	"net/http"
	"testing"
)

func TestReadAllowlist(t *testing.T) {
	for _, path := range []string{"/Home/LoginUser", "/Home/GetListData"} {
		if err := ValidateReadRequest(
			http.MethodPost,
			"https://sign.azh-myyolo.info"+path,
		); err != nil {
			t.Fatalf("classified route %s was blocked: %v", path, err)
		}
	}
	for _, route := range []struct {
		method string
		url    string
	}{
		{http.MethodPost, "https://www.azh-myyolo.info/Anmelden.asp?vw="},
		{http.MethodGet, "https://www.azh-myyolo.info/LoginHandler.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Start.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/start.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/start_Anwesenheit.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Anwesenheit/Anwesend_Aktuell_Liste.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Anwesenheit/Anwesend_Heute_Liste.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Anwesenheit/Anwesend_Datum_Liste.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Anwesenheit/Reha_Anwesend_Datum_Liste.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp"},
		{http.MethodGet, "https://www.azh-myyolo.info/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp"},
	} {
		if err := ValidateReadRequest(route.method, route.url); err != nil {
			t.Fatalf("classified admin route %s was blocked: %v", route.url, err)
		}
	}
}

func TestWriteAndUnknownRoutesAreBlocked(t *testing.T) {
	tests := []struct {
		name   string
		method string
		url    string
	}{
		{"signature write", http.MethodPost, "https://sign.azh-myyolo.info/Home/StoreTeilnehmerTeilnahmeExt"},
		{"course note write", http.MethodPost, "https://sign.azh-myyolo.info/Home/StoreKursBuchungBemerkungen"},
		{"add participant", http.MethodPost, "https://sign.azh-myyolo.info/Home/AddTeilnehmerToKurs"},
		{"unclassified validation", http.MethodPost, "https://sign.azh-myyolo.info/Home/CheckMerkmaleAndLimits"},
		{"wrong method", http.MethodGet, "https://sign.azh-myyolo.info/Home/GetListData"},
		{"wrong host", http.MethodPost, "https://example.com/Home/GetListData"},
		{"plaintext", http.MethodPost, "http://sign.azh-myyolo.info/Home/GetListData"},
		{"query string", http.MethodPost, "https://sign.azh-myyolo.info/Home/GetListData?extra=1"},
		{"admin login missing query", http.MethodPost, "https://www.azh-myyolo.info/Anmelden.asp"},
		{"admin login changed query", http.MethodPost, "https://www.azh-myyolo.info/Anmelden.asp?vw=1"},
		{"admin read changed query", http.MethodGet, "https://www.azh-myyolo.info/start_Anwesenheit.asp?month=7"},
		{"alternate port", http.MethodPost, "https://sign.azh-myyolo.info:8443/Home/GetListData"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateReadRequest(test.method, test.url); err == nil {
				t.Fatal("expected request to be blocked")
			}
		})
	}
}
