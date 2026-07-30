package admin

import (
	"context"
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

func TestFetchAttendanceUsesCachedAdminSessionInOneRequest(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) *http.Response {
		calls++
		if request.URL.Path != "/start_Anwesenheit.asp" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if cookie, err := request.Cookie("ASPSESSIONID"); err != nil || cookie.Value != "cached" {
			t.Fatalf("cached cookie = %#v, error = %v", cookie, err)
		}
		return htmlResponse(request, http.StatusOK, authenticatedHTML())
	})

	page, session, err := client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies:         []secrets.Cookie{{Name: "ASPSESSIONID", Value: "cached"}},
			AuthenticatedAt: "2026-07-30T08:00:00Z",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || page.Metadata.Title != "Attendance" ||
		session.AuthenticatedAt != "2026-07-30T08:00:00Z" {
		t.Fatalf("calls = %d, page = %#v, session = %#v", calls, page.Metadata, session)
	}
}

func TestFetchAttendanceRelogsExactlyOnceWithinSharedBudget(t *testing.T) {
	var paths []string
	client := testClient(t, func(request *http.Request) *http.Response {
		paths = append(paths, request.Method+" "+request.URL.RequestURI())
		switch len(paths) {
		case 1:
			return redirectResponse(request, "/Anmelden.asp?vw=")
		case 2:
			assertLoginForm(t, request)
			response := redirectResponse(request, "/Start.asp")
			response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/; Secure; HttpOnly")
			return response
		case 3:
			if cookie, err := request.Cookie("ASPSESSIONID"); err != nil || cookie.Value != "fresh" {
				t.Fatalf("fresh cookie on Start.asp = %#v, error = %v", cookie, err)
			}
			return htmlResponse(request, http.StatusOK, startHTML())
		default:
			return htmlResponse(request, http.StatusOK, authenticatedHTML())
		}
	})

	_, session, err := client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "expired"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /start_Anwesenheit.asp",
		"POST /Anmelden.asp?vw=",
		"GET /Start.asp",
		"GET /start_Anwesenheit.asp",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if len(session.Cookies) != 1 || session.Cookies[0].Value != "fresh" {
		t.Fatalf("session = %#v", session)
	}
}

func TestFetchAttendanceNeverRetriesAfterFinalExpiredRead(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			return redirectResponse(request, "/Anmelden.asp?vw=")
		case 2:
			response := redirectResponse(request, "/Start.asp")
			response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
			return response
		case 3:
			return htmlResponse(request, http.StatusOK, startHTML())
		default:
			return redirectResponse(request, "/Anmelden.asp?vw=")
		}
	})
	_, _, err := client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "expired"}},
		},
	)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v", err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
}

func TestExpiredAdminCookieRedirectsThroughObservedDefaultPage(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			return redirectResponse(request, "/default.asp")
		case 2:
			response := redirectResponse(request, "/LoginHandler.asp")
			response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
			return response
		case 3:
			return redirectResponse(request, "/Start.asp")
		case 4:
			return htmlResponse(request, http.StatusOK, startHTML())
		default:
			return htmlResponse(request, http.StatusOK, authenticatedHTML())
		}
	})
	_, _, err := client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "expired"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 5 {
		t.Fatalf("calls = %d, want 5", calls)
	}
}

func TestFetchAttendanceDoesNotRetryServerErrorsOrCAPTCHA(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
		want error
	}{
		{"server error", "temporary", http.StatusInternalServerError, nil},
		{"rate limit", "slow down", http.StatusTooManyRequests, ErrRateLimited},
		{"captcha", "<html>CAPTCHA</html>", http.StatusOK, ErrCAPTCHA},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls int
			client := testClient(t, func(request *http.Request) *http.Response {
				calls++
				return htmlResponse(request, test.code, test.body)
			})
			_, _, err := client.FetchAttendanceHome(
				context.Background(),
				testCredentials(),
				secrets.AdminSession{
					Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "cached"}},
				},
			)
			if err == nil {
				t.Fatal("expected read to fail")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
		})
	}
}

func TestFetchAttendanceBudgetCountsLoginRedirectAndFinalRead(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			switch calls {
			case 1:
				return redirectResponse(request, "/Anmelden.asp?vw="), nil
			case 2:
				response := redirectResponse(request, "/Start.asp")
				response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
				return response, nil
			default:
				return htmlResponse(request, http.StatusOK, startHTML()), nil
			}
		}),
	}
	client, err := New(httpClient, WithDelay(0), WithRequestBudget(3))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "expired"}},
		},
	)
	if !errors.Is(err, ErrRequestBudget) {
		t.Fatalf("error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestAdminDelayAppliesBetweenEveryRequest(t *testing.T) {
	var calls, sleeps int
	client := testClientWithOptions(
		t,
		func(request *http.Request) *http.Response {
			calls++
			switch calls {
			case 1:
				response := redirectResponse(request, "/Start.asp")
				response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
				return response
			case 2:
				return htmlResponse(request, http.StatusOK, startHTML())
			default:
				return htmlResponse(request, http.StatusOK, authenticatedHTML())
			}
		},
		WithDelay(2*time.Second),
		WithSleeper(func(context.Context, time.Duration) error {
			sleeps++
			return nil
		}),
	)
	if _, _, err := client.FetchAttendanceHome(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{},
	); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || sleeps != 2 {
		t.Fatalf("calls = %d, sleeps = %d", calls, sleeps)
	}
}

func TestAdminLoginRejectsCrossHostRedirect(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		return redirectResponse(request, "https://example.com/Start.asp")
	})
	_, err := client.Login(context.Background(), testCredentials())
	if !errors.Is(err, ErrSchemaDrift) {
		t.Fatalf("error = %v", err)
	}
}

func TestAdminLoginAcceptsObservedLowercaseStartAlias(t *testing.T) {
	var calls int
	client := testClient(t, func(request *http.Request) *http.Response {
		calls++
		if calls == 1 {
			response := redirectResponse(request, "/start.asp")
			response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
			return response
		}
		if request.URL.Path != "/start.asp" {
			t.Fatalf("start path = %q", request.URL.Path)
		}
		return htmlResponse(request, http.StatusOK, startHTML())
	})
	if _, err := client.Login(context.Background(), testCredentials()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestAdminLoginFollowsObservedHandlerThenStart(t *testing.T) {
	var paths []string
	client := testClient(t, func(request *http.Request) *http.Response {
		paths = append(paths, request.URL.Path)
		switch len(paths) {
		case 1:
			response := redirectResponse(request, "/LoginHandler.asp")
			response.Header.Add("Set-Cookie", "ASPFIXATION=one; Path=/")
			return response
		case 2:
			response := redirectResponse(request, "/start.asp")
			response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
			return response
		default:
			return htmlResponse(request, http.StatusOK, startHTML())
		}
	})
	session, err := client.Login(context.Background(), testCredentials())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/Anmelden.asp", "/LoginHandler.asp", "/start.asp"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if len(session.Cookies) != 2 {
		t.Fatalf("session = %#v", session)
	}
}

func TestDiscoverUsesOneSharedTenRequestBudgetAfterExpiredSession(t *testing.T) {
	var paths []string
	client := testClientWithOptions(
		t,
		func(request *http.Request) *http.Response {
			paths = append(paths, request.Method+" "+request.URL.RequestURI())
			switch len(paths) {
			case 1:
				return redirectResponse(request, "/Anmelden.asp?vw=")
			case 2:
				response := redirectResponse(request, "/LoginHandler.asp")
				response.Header.Add("Set-Cookie", "ASPSESSIONID=fresh; Path=/")
				return response
			case 3:
				return redirectResponse(request, "/Start.asp")
			case 4:
				return htmlResponse(request, http.StatusOK, startHTML())
			default:
				return htmlResponse(request, http.StatusOK, authenticatedHTML())
			}
		},
		WithRequestBudget(10),
	)
	pages, session, err := client.Discover(
		context.Background(),
		testCredentials(),
		secrets.AdminSession{
			Cookies: []secrets.Cookie{{Name: "ASPSESSIONID", Value: "expired"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 10 || len(pages) != len(KnownReadPages) {
		t.Fatalf("paths = %d, pages = %d", len(paths), len(pages))
	}
	if paths[0] != "GET /start_Anwesenheit.asp" ||
		paths[1] != "POST /Anmelden.asp?vw=" ||
		paths[2] != "GET /LoginHandler.asp" ||
		paths[3] != "GET /Start.asp" {
		t.Fatalf("request prefix = %#v", paths[:4])
	}
	for index, page := range pages {
		if page.Metadata.Route != KnownReadPages[index] {
			t.Fatalf("page %d route = %q", index, page.Metadata.Route)
		}
	}
	if len(session.Cookies) != 1 || session.Cookies[0].Value != "fresh" {
		t.Fatalf("session = %#v", session)
	}
}

func testClient(
	t *testing.T,
	responder func(*http.Request) *http.Response,
) *Client {
	t.Helper()
	return testClientWithOptions(t, responder)
}

func testClientWithOptions(
	t *testing.T,
	responder func(*http.Request) *http.Response,
	options ...Option,
) *Client {
	t.Helper()
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return responder(request), nil
		}),
	}
	defaults := []Option{
		WithDelay(0),
		WithSleeper(func(context.Context, time.Duration) error { return nil }),
		WithClock(func() time.Time {
			return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		}),
	}
	defaults = append(defaults, options...)
	client, err := New(httpClient, defaults...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func testCredentials() secrets.Credentials {
	return secrets.Credentials{
		PartnerNumber: "partner",
		Username:      "reader",
		Password:      "very-secret",
	}
}

func assertLoginForm(t *testing.T, request *http.Request) {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if values.Get("PartnerNummer") != "partner" ||
		values.Get("Benutzername") != "reader" ||
		values.Get("Password") != "very-secret" {
		t.Fatalf("login values = %#v", values)
	}
}

func htmlResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": {"text/html; charset=utf-8"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: request,
	}
}

func redirectResponse(request *http.Request, location string) *http.Response {
	response := htmlResponse(request, http.StatusFound, "")
	response.Header.Set("Location", location)
	return response
}

func authenticatedHTML() string {
	return `<!doctype html><html><head><title>Attendance</title></head>
		<body><h1>Attendance</h1><table>
		<tr><th>Course</th><th>Present</th></tr>
		<tr><td>Synthetic</td><td>2</td></tr>
		</table></body></html>`
}

func startHTML() string {
	return `<!doctype html><html><head><title>Start</title></head>
		<body><h1>Start</h1></body></html>`
}
