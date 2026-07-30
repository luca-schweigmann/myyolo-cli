package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
	"github.com/luca-schweigmann/myyolo-cli/internal/transport"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	DefaultBaseURL     = "https://www.azh-myyolo.info"
	DefaultDelay       = 2 * time.Second
	DefaultFetchBudget = 5
	MaxRequestBudget   = 10
	maxResponse        = 16 << 20
)

var (
	ErrSessionExpired = errors.New("myYOLO admin session expired")
	ErrAuthentication = errors.New("myYOLO admin authentication failed")
	ErrSchemaDrift    = errors.New("myYOLO admin response schema changed")
	ErrRequestBudget  = errors.New("myYOLO admin request budget exhausted")
	ErrRateLimited    = errors.New("myYOLO admin rate limit reached")
	ErrCAPTCHA        = errors.New("myYOLO admin CAPTCHA encountered")
)

var KnownReadPages = []string{
	"/Anwesenheit/Anwesend_Aktuell_Liste.asp",
	"/Anwesenheit/Anwesend_Heute_Liste.asp",
	"/Anwesenheit/Anwesend_Datum_Liste.asp",
	"/Anwesenheit/Reha_Anwesend_Datum_Liste.asp",
	"/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp",
	"/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp",
}

type Client struct {
	baseURL     *url.URL
	http        *http.Client
	delay       time.Duration
	fetchBudget int
	sleep       func(context.Context, time.Duration) error
	now         func() time.Time
	mu          sync.Mutex
	lastCall    time.Time
}

type Option func(*Client)

func WithDelay(delay time.Duration) Option {
	return func(client *Client) {
		client.delay = delay
	}
}

func WithRequestBudget(limit int) Option {
	return func(client *Client) {
		client.fetchBudget = limit
	}
}

func WithSleeper(sleeper func(context.Context, time.Duration) error) Option {
	return func(client *Client) {
		client.sleep = sleeper
	}
}

func WithClock(now func() time.Time) Option {
	return func(client *Client) {
		client.now = now
	}
}

func New(httpClient *http.Client, options ...Option) (*Client, error) {
	baseURL, err := url.Parse(DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clone := *httpClient
	if clone.Timeout == 0 {
		clone.Timeout = 30 * time.Second
	}
	if clone.Jar == nil {
		clone.Jar, err = cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create admin cookie jar: %w", err)
		}
	}
	// Redirects are followed manually so every hop passes the allowlist,
	// delay, and shared request budget.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &Client{
		baseURL:     baseURL,
		http:        &clone,
		delay:       DefaultDelay,
		fetchBudget: DefaultFetchBudget,
		sleep:       sleepContext,
		now:         time.Now,
	}
	for _, option := range options {
		option(client)
	}
	if client.delay < 0 {
		return nil, fmt.Errorf("admin request delay must not be negative")
	}
	if client.fetchBudget < 1 || client.fetchBudget > MaxRequestBudget {
		return nil, fmt.Errorf("admin request budget must be between 1 and %d", MaxRequestBudget)
	}
	if client.sleep == nil || client.now == nil {
		return nil, fmt.Errorf("admin sleeper and clock are required")
	}
	return client, nil
}

func (client *Client) Login(
	ctx context.Context,
	credentials secrets.Credentials,
) (secrets.AdminSession, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	budget := newBudget(client.fetchBudget)
	return client.login(ctx, credentials, budget)
}

func (client *Client) FetchAttendanceHome(
	ctx context.Context,
	credentials secrets.Credentials,
	session secrets.AdminSession,
) (Page, secrets.AdminSession, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	budget := newBudget(client.fetchBudget)
	if len(session.Cookies) > 0 {
		client.restoreCookies(session)
		page, err := client.readPage(ctx, "/start_Anwesenheit.asp", budget)
		if err == nil {
			return page, client.captureSession(session.AuthenticatedAt), nil
		}
		if !errors.Is(err, ErrSessionExpired) {
			return Page{}, secrets.AdminSession{}, err
		}
	}

	fresh, err := client.login(ctx, credentials, budget)
	if err != nil {
		return Page{}, secrets.AdminSession{}, err
	}
	page, err := client.readPage(ctx, "/start_Anwesenheit.asp", budget)
	if err != nil {
		return Page{}, secrets.AdminSession{}, err
	}
	return page, client.captureSession(fresh.AuthenticatedAt), nil
}

func (client *Client) Discover(
	ctx context.Context,
	credentials secrets.Credentials,
	session secrets.AdminSession,
) ([]Page, secrets.AdminSession, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	budget := newBudget(client.fetchBudget)
	pages := make([]Page, 0, len(KnownReadPages))
	startIndex := 0
	authenticatedAt := session.AuthenticatedAt
	if len(session.Cookies) > 0 {
		client.restoreCookies(session)
		_, err := client.readPage(ctx, "/start_Anwesenheit.asp", budget)
		if err == nil {
			startIndex = len(KnownReadPages)
		} else if !errors.Is(err, ErrSessionExpired) {
			return nil, secrets.AdminSession{}, err
		}
	}
	if startIndex != len(KnownReadPages) {
		fresh, err := client.login(ctx, credentials, budget)
		if err != nil {
			return nil, secrets.AdminSession{}, err
		}
		authenticatedAt = fresh.AuthenticatedAt
	}
	for _, path := range KnownReadPages {
		page, err := client.readPage(ctx, path, budget)
		if err != nil {
			return nil, secrets.AdminSession{}, err
		}
		pages = append(pages, page)
	}
	return pages, client.captureSession(authenticatedAt), nil
}

func (client *Client) login(
	ctx context.Context,
	credentials secrets.Credentials,
	budget *requestBudget,
) (secrets.AdminSession, error) {
	if err := secrets.ValidateCredentials(credentials); err != nil {
		return secrets.AdminSession{}, fmt.Errorf("%w: invalid credentials shape", ErrAuthentication)
	}
	if err := client.resetCookies(); err != nil {
		return secrets.AdminSession{}, err
	}
	form := url.Values{
		"PartnerNummer": {credentials.PartnerNumber},
		"Benutzername":  {credentials.Username},
		"Password":      {credentials.Password},
	}
	response, err := client.request(
		ctx,
		http.MethodPost,
		"/Anmelden.asp?vw=",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
		budget,
	)
	if err != nil {
		return secrets.AdminSession{}, err
	}

	var page Page
	if response.status >= 300 && response.status < 400 {
		if response.location == "" {
			return secrets.AdminSession{}, fmt.Errorf("%w: login redirect has no location", ErrSchemaDrift)
		}
		page, err = client.followLoginRedirect(ctx, response.location, budget)
		if err != nil {
			if errors.Is(err, ErrSessionExpired) {
				return secrets.AdminSession{}, ErrAuthentication
			}
			return secrets.AdminSession{}, err
		}
	} else if response.status == http.StatusOK {
		page, err = client.parseResponse("/Anmelden.asp", response)
		if err != nil {
			return secrets.AdminSession{}, err
		}
	} else {
		return secrets.AdminSession{}, fmt.Errorf(
			"%w: login returned HTTP %d",
			ErrAuthentication,
			response.status,
		)
	}
	if page.Metadata.LoginForm {
		return secrets.AdminSession{}, ErrAuthentication
	}

	authenticatedAt := client.now().UTC().Format(time.RFC3339)
	session := client.captureSession(authenticatedAt)
	if err := secrets.ValidateAdminSession(session); err != nil {
		return secrets.AdminSession{}, fmt.Errorf("%w: login did not establish a session", ErrSchemaDrift)
	}
	return session, nil
}

func (client *Client) followLoginRedirect(
	ctx context.Context,
	location string,
	budget *requestBudget,
) (Page, error) {
	for hop := 0; hop < 2; hop++ {
		target, err := client.baseURL.Parse(location)
		if err != nil {
			return Page{}, fmt.Errorf("%w: invalid login redirect", ErrSchemaDrift)
		}
		if pointsToLogin(client.baseURL, location) {
			return Page{}, ErrAuthentication
		}
		if !isApprovedLoginTarget(client.baseURL, target) {
			return Page{}, fmt.Errorf(
				"%w: unexpected login redirect (%s)",
				ErrSchemaDrift,
				sanitizedRedirect(target),
			)
		}
		if strings.EqualFold(target.EscapedPath(), "/Start.asp") {
			return client.readPage(ctx, target.EscapedPath(), budget)
		}

		response, err := client.request(
			ctx,
			http.MethodGet,
			target.EscapedPath(),
			"",
			nil,
			budget,
		)
		if err != nil {
			return Page{}, err
		}
		if response.status >= 300 && response.status < 400 {
			if response.location == "" {
				return Page{}, fmt.Errorf("%w: auth redirect has no location", ErrSchemaDrift)
			}
			location = response.location
			continue
		}
		if response.status == http.StatusOK {
			return client.parseResponse(target.EscapedPath(), response)
		}
		if response.status == http.StatusUnauthorized || response.status == http.StatusForbidden {
			return Page{}, ErrAuthentication
		}
		return Page{}, fmt.Errorf(
			"%w: auth handler returned HTTP %d",
			ErrAuthentication,
			response.status,
		)
	}
	return Page{}, fmt.Errorf("%w: too many login redirects", ErrSchemaDrift)
}

func isApprovedLoginTarget(baseURL *url.URL, target *url.URL) bool {
	if target == nil ||
		target.Scheme != "https" ||
		!strings.EqualFold(target.Hostname(), baseURL.Hostname()) ||
		(target.Port() != "" && target.Port() != "443") ||
		target.RawQuery != "" {
		return false
	}
	return strings.EqualFold(target.EscapedPath(), "/Start.asp") ||
		target.EscapedPath() == "/LoginHandler.asp"
}

func (client *Client) readPage(
	ctx context.Context,
	path string,
	budget *requestBudget,
) (Page, error) {
	response, err := client.request(ctx, http.MethodGet, path, "", nil, budget)
	if err != nil {
		return Page{}, err
	}
	if response.status >= 300 && response.status < 400 {
		if pointsToLogin(client.baseURL, response.location) {
			return Page{}, ErrSessionExpired
		}
		target, _ := client.baseURL.Parse(response.location)
		return Page{}, fmt.Errorf(
			"%w: unexpected redirect (%s)",
			ErrSchemaDrift,
			sanitizedRedirect(target),
		)
	}
	if response.status == http.StatusUnauthorized || response.status == http.StatusForbidden {
		return Page{}, ErrSessionExpired
	}
	if response.status != http.StatusOK {
		return Page{}, fmt.Errorf("myYOLO admin returned HTTP %d", response.status)
	}
	page, err := client.parseResponse(path, response)
	if err != nil {
		return Page{}, err
	}
	if page.Metadata.LoginForm {
		return Page{}, ErrSessionExpired
	}
	return page, nil
}

type response struct {
	status      int
	location    string
	contentType string
	body        []byte
}

func (client *Client) request(
	ctx context.Context,
	method string,
	path string,
	contentType string,
	body io.Reader,
	budget *requestBudget,
) (response, error) {
	if err := budget.take(); err != nil {
		return response{}, err
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{
		Path:     strings.SplitN(path, "?", 2)[0],
		RawQuery: rawQuery(path),
	}).String()
	if err := transport.ValidateReadRequest(method, endpoint); err != nil {
		return response{}, err
	}
	if !client.lastCall.IsZero() && client.delay > 0 {
		if err := client.sleep(ctx, client.delay); err != nil {
			return response{}, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return response{}, fmt.Errorf("create admin request: %w", err)
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("User-Agent", "myyolo-cli")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	httpResponse, err := client.http.Do(request)
	client.lastCall = client.now()
	if err != nil {
		return response{}, fmt.Errorf("request myYOLO admin: %w", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode == http.StatusTooManyRequests {
		return response{}, ErrRateLimited
	}
	data, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponse+1))
	if err != nil {
		return response{}, fmt.Errorf("read myYOLO admin response: %w", err)
	}
	if len(data) > maxResponse {
		return response{}, fmt.Errorf("myYOLO admin response exceeds %d bytes", maxResponse)
	}
	if looksLikeCAPTCHA(data) {
		return response{}, ErrCAPTCHA
	}
	return response{
		status:      httpResponse.StatusCode,
		location:    httpResponse.Header.Get("Location"),
		contentType: httpResponse.Header.Get("Content-Type"),
		body:        data,
	}, nil
}

func (client *Client) parseResponse(path string, value response) (Page, error) {
	reader, err := charset.NewReader(bytes.NewReader(value.body), value.contentType)
	if err != nil {
		return Page{}, fmt.Errorf("%w: decode response charset", ErrSchemaDrift)
	}
	document, err := html.Parse(reader)
	if err != nil {
		return Page{}, fmt.Errorf("%w: parse HTML", ErrSchemaDrift)
	}
	page, err := ParsePage(client.baseURL, path, document)
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrSchemaDrift, err)
	}
	return page, nil
}

func (client *Client) restoreCookies(session secrets.AdminSession) {
	cookies := make([]*http.Cookie, 0, len(session.Cookies))
	for _, cookie := range session.Cookies {
		if cookie.Name == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: "/"})
	}
	client.http.Jar.SetCookies(client.baseURL, cookies)
}

func (client *Client) captureSession(authenticatedAt string) secrets.AdminSession {
	cookies := client.http.Jar.Cookies(client.baseURL)
	saved := make([]secrets.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		saved = append(saved, secrets.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	return secrets.AdminSession{
		Cookies:         saved,
		AuthenticatedAt: authenticatedAt,
	}
}

func (client *Client) resetCookies() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("reset admin cookie jar: %w", err)
	}
	client.http.Jar = jar
	return nil
}

type requestBudget struct {
	remaining int
}

func newBudget(limit int) *requestBudget {
	return &requestBudget{remaining: limit}
}

func (budget *requestBudget) take() error {
	if budget.remaining == 0 {
		return ErrRequestBudget
	}
	budget.remaining--
	return nil
}

func rawQuery(path string) string {
	parts := strings.SplitN(path, "?", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func pointsToLogin(baseURL *url.URL, location string) bool {
	if location == "" {
		return false
	}
	target, err := baseURL.Parse(location)
	if err != nil {
		return false
	}
	return strings.EqualFold(target.Hostname(), baseURL.Hostname()) &&
		target.Scheme == "https" &&
		(target.Port() == "" || target.Port() == "443") &&
		(strings.EqualFold(target.EscapedPath(), "/Anmelden.asp") ||
			strings.EqualFold(target.EscapedPath(), "/default.asp"))
}

func sanitizedRedirect(target *url.URL) string {
	if target == nil {
		return "invalid target"
	}
	keys := make([]string, 0, len(target.Query()))
	for key := range target.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf(
		"scheme=%s host_matches=%t path=%s query_keys=%s",
		target.Scheme,
		strings.EqualFold(target.Hostname(), "www.azh-myyolo.info"),
		target.EscapedPath(),
		strings.Join(keys, ","),
	)
}

func looksLikeCAPTCHA(data []byte) bool {
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "g-recaptcha") ||
		strings.Contains(lower, "hcaptcha")
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
