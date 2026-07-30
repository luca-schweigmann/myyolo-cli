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
