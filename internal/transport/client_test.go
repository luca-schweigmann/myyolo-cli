package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestFetchUsesCachedSessionInOneRequest(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) string {
		calls++
		if request.URL.Path != "/Home/GetListData" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		return syntheticSnapshot("next")
	})

	_, session, err := client.Fetch(
		context.Background(),
		secrets.Credentials{PartnerNumber: "p", Username: "u", Password: "secret"},
		secrets.Session{NextRequestToken: "cached"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || session.NextRequestToken != "next" {
		t.Fatalf("calls = %d, session = %#v", calls, session)
	}
}

func TestFetchRelogsInExactlyOnce(t *testing.T) {
	var paths []string
	client := testClient(t, func(request *http.Request) string {
		paths = append(paths, request.URL.Path)
		switch len(paths) {
		case 1:
			return "<html>login</html>"
		case 2:
			return `{"nextRequestToken":"fresh"}`
		default:
			return syntheticSnapshot("rotated")
		}
	})

	_, session, err := client.Fetch(
		context.Background(),
		secrets.Credentials{PartnerNumber: "p", Username: "u", Password: "secret"},
		secrets.Session{NextRequestToken: "expired"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/Home/GetListData", "/Home/LoginUser", "/Home/GetListData"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if session.NextRequestToken != "rotated" {
		t.Fatalf("token = %q", session.NextRequestToken)
	}
}

func TestFetchNeverMakesFourthRequest(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) string {
		calls++
		if request.URL.Path == "/Home/LoginUser" {
			return `{"nextRequestToken":"fresh"}`
		}
		return "<html>login</html>"
	})

	_, _, err := client.Fetch(
		context.Background(),
		secrets.Credentials{PartnerNumber: "p", Username: "u", Password: "secret"},
		secrets.Session{NextRequestToken: "expired"},
	)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestSchemaDriftDoesNotTriggerRelogin(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) string {
		calls++
		return `{"unexpected":true}`
	})

	_, _, err := client.Fetch(
		context.Background(),
		secrets.Credentials{PartnerNumber: "p", Username: "u", Password: "secret"},
		secrets.Session{NextRequestToken: "cached"},
	)
	if !errors.Is(err, ErrSchemaDrift) {
		t.Fatalf("error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestLoginBodyAndErrorsNeverExposePassword(t *testing.T) {
	client := testClient(t, func(request *http.Request) string {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var login map[string]string
		if err := json.Unmarshal(body, &login); err != nil {
			t.Fatal(err)
		}
		if login["password"] != "very-secret" {
			t.Fatal("password was not sent in the JSON body")
		}
		return `"Invalid Login"`
	})

	_, err := client.Login(context.Background(), secrets.Credentials{
		PartnerNumber: "p",
		Username:      "u",
		Password:      "very-secret",
	})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "very-secret") {
		t.Fatal("password leaked into error")
	}
}

func TestRedirectGuardKeepsExactAllowlist(t *testing.T) {
	guard := sameHostRedirect(mustURL(t, DefaultBaseURL), nil)
	if err := guard(&http.Request{
		Method: http.MethodGet,
		URL:    mustURL(t, DefaultBaseURL+"/Home/Other"),
	}, nil); err == nil {
		t.Fatal("unapproved same-host redirect was allowed")
	}
	if err := guard(&http.Request{
		Method: http.MethodPost,
		URL:    mustURL(t, DefaultBaseURL+"/Home/GetListData"),
	}, nil); err != nil {
		t.Fatalf("approved redirect was blocked: %v", err)
	}
}

func testClient(t *testing.T, responder func(*http.Request) string) *Client {
	t.Helper()
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(responder(request))),
				Request:    request,
			}, nil
		}),
	}
	client, err := New(
		httpClient,
		WithDelay(0),
		WithSleeper(func(context.Context, time.Duration) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func syntheticSnapshot(token string) string {
	return `{
		"nextRequestToken":"` + token + `",
		"KursBuchungen":[{"Id":1}],
		"Mitglieder":[{"Id":2}],
		"Verordnungen":[],
		"KursTeilnehmer":[{
			"Id":3,
			"MitgliedId":2,
			"KursBuchungId":1,
			"HatUnterschrift":false,
			"Teilgenommen":false,
			"Storniert":false
		}]
	}`
}

func mustURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
