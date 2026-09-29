package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/transport"
)

func TestClassifyErrorMapsSentinels(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
		exit int
	}{
		{fmt.Errorf("request: %w", context.DeadlineExceeded), "NETWORK_TIMEOUT", 11},
		{fmt.Errorf("%w: Profil", errProfileMissing), "AUTH_PROFILE_MISSING", 20},
		{fmt.Errorf("%w: bad", transport.ErrAuthentication), "AUTH_REJECTED", 21},
		{admin.ErrSessionExpired, "AUTH_SESSION_EXPIRED", 22},
		{transport.ErrRateLimited, "PROVIDER_RATE_LIMITED", 30},
		{admin.ErrCAPTCHA, "PROVIDER_CAPTCHA", 31},
		{fmt.Errorf("%w: HTTP 502", transport.ErrHTTPStatus), "PROVIDER_HTTP_ERROR", 32},
		{fmt.Errorf("%w: x", admin.ErrSchemaDrift), "PROVIDER_SCHEMA_DRIFT", 40},
		{transport.ErrRequestBudget, "REQUEST_BUDGET_EXHAUSTED", 41},
	} {
		code, ok := classifyError(tc.err)
		if !ok || code.Code != tc.code || code.Exit != tc.exit {
			t.Errorf("%v: got %+v %v, want %s/%d", tc.err, code, ok, tc.code, tc.exit)
		}
	}
	if _, ok := classifyError(errors.New("REHA_SCOPE_FAILED: x")); ok {
		t.Fatal("own-coded errors must stay unclassified with exit 1")
	}
}

func TestClassifyErrorDetectsInvalidCertificate(t *testing.T) {
	// httptest's self-signed certificate is untrusted by a default client,
	// which is the same failure class as the expired provider certificate.
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	_, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{}}}).Get(server.URL)
	if err == nil {
		t.Fatal("expected TLS error")
	}
	var stderr bytes.Buffer
	if exit := reportError(&stderr, fmt.Errorf("request myYOLO: %w", err)); exit != 10 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "Fehler: NETWORK_TLS_CERTIFICATE_INVALID: ") || !strings.Contains(stderr.String(), "\nZu tun: ") {
		t.Fatalf("stderr contract broken: %q", stderr.String())
	}
}

func TestReportErrorKeepsUnclassifiedFormat(t *testing.T) {
	var stderr bytes.Buffer
	if exit := reportError(&stderr, errors.New("REHA_SCOPE_FAILED: Scope ungültig")); exit != 1 {
		t.Fatalf("exit = %d", exit)
	}
	if stderr.String() != "Fehler: REHA_SCOPE_FAILED: Scope ungültig\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestErrorCodesAreUniqueAndDocumented(t *testing.T) {
	codes, exits := map[string]bool{}, map[int]bool{}
	for _, item := range errorCodes {
		if codes[item.Code] || exits[item.Exit] || item.Exit <= 1 || item.ActionDE == "" {
			t.Fatalf("invalid contract entry %+v", item)
		}
		codes[item.Code], exits[item.Exit] = true, true
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"errors", "--format", "json"}, nil, &out); err != nil || !strings.Contains(out.String(), `"exit_code": 10`) {
		t.Fatalf("errors command: %v %s", err, out.String())
	}
}
