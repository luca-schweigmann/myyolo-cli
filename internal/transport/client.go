package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/luca-schweigmann/myyolo-cli/internal/mysign"
	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
)

const (
	DefaultBaseURL = "https://sign.azh-myyolo.info"
	maxResponse    = 16 << 20
	maxFetchCalls  = 3
)

var (
	ErrSessionExpired = errors.New("myYOLO session expired")
	ErrAuthentication = errors.New("myYOLO authentication failed")
	ErrSchemaDrift    = errors.New("myYOLO response schema changed")
	ErrRequestBudget  = errors.New("myYOLO request budget exhausted")
)

type Client struct {
	baseURL  *url.URL
	http     *http.Client
	delay    time.Duration
	sleep    func(context.Context, time.Duration) error
	mu       sync.Mutex
	lastCall time.Time
}

type Option func(*Client)

func WithDelay(delay time.Duration) Option {
	return func(client *Client) {
		client.delay = delay
	}
}

func WithSleeper(sleeper func(context.Context, time.Duration) error) Option {
	return func(client *Client) {
		client.sleep = sleeper
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
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
	}
	clone.CheckRedirect = sameHostRedirect(baseURL, clone.CheckRedirect)

	client := &Client{
		baseURL: baseURL,
		http:    &clone,
		delay:   time.Second,
		sleep:   sleepContext,
	}
	for _, option := range options {
		option(client)
	}
	return client, nil
}

func (client *Client) Login(
	ctx context.Context,
	credentials secrets.Credentials,
) (secrets.Session, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.login(ctx, credentials, newBudget(1))
}

func (client *Client) Fetch(
	ctx context.Context,
	credentials secrets.Credentials,
	session secrets.Session,
) (mysign.Snapshot, secrets.Session, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	client.restoreCookies(session)
	budget := newBudget(maxFetchCalls)
	if session.NextRequestToken != "" {
		snapshot, nextSession, err := client.read(ctx, session.NextRequestToken, budget)
		if err == nil {
			return snapshot, nextSession, nil
		}
		if !errors.Is(err, ErrSessionExpired) {
			return mysign.Snapshot{}, secrets.Session{}, err
		}
	}

	fresh, err := client.login(ctx, credentials, budget)
	if err != nil {
		return mysign.Snapshot{}, secrets.Session{}, err
	}
	snapshot, nextSession, err := client.read(ctx, fresh.NextRequestToken, budget)
	if err != nil {
		return mysign.Snapshot{}, secrets.Session{}, err
	}
	return snapshot, nextSession, nil
}

func (client *Client) login(
	ctx context.Context,
	credentials secrets.Credentials,
	budget *requestBudget,
) (secrets.Session, error) {
	if err := secrets.ValidateCredentials(credentials); err != nil {
		return secrets.Session{}, fmt.Errorf("%w: invalid credentials shape", ErrAuthentication)
	}
	body := struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		PartnerNumber string `json:"partnerNumber"`
	}{
		Username:      credentials.Username,
		Password:      credentials.Password,
		PartnerNumber: credentials.PartnerNumber,
	}
	response, err := client.postJSON(ctx, "/Home/LoginUser", body, budget)
	if err != nil {
		return secrets.Session{}, err
	}
	trimmed := bytes.TrimSpace(response)
	var message string
	if json.Unmarshal(trimmed, &message) == nil {
		return secrets.Session{}, fmt.Errorf("%w: %s", ErrAuthentication, safeMessage(message))
	}
	var result struct {
		NextRequestToken string `json:"nextRequestToken"`
	}
	if err := json.Unmarshal(trimmed, &result); err != nil {
		return secrets.Session{}, fmt.Errorf("%w: decode login response", ErrSchemaDrift)
	}
	if result.NextRequestToken == "" {
		return secrets.Session{}, fmt.Errorf("%w: login response has no request token", ErrSchemaDrift)
	}
	return client.captureSession(result.NextRequestToken), nil
}

func (client *Client) read(
	ctx context.Context,
	token string,
	budget *requestBudget,
) (mysign.Snapshot, secrets.Session, error) {
	response, err := client.postJSON(ctx, "/Home/GetListData", struct {
		NextRequestToken string `json:"nextRequestToken"`
	}{NextRequestToken: token}, budget)
	if err != nil {
		return mysign.Snapshot{}, secrets.Session{}, err
	}
	trimmed := bytes.TrimSpace(response)
	if looksLikeHTML(trimmed) {
		return mysign.Snapshot{}, secrets.Session{}, ErrSessionExpired
	}
	var message string
	if json.Unmarshal(trimmed, &message) == nil {
		if strings.Contains(strings.ToLower(message), "login") {
			return mysign.Snapshot{}, secrets.Session{}, ErrSessionExpired
		}
		return mysign.Snapshot{}, secrets.Session{}, fmt.Errorf("%w: unexpected response", ErrSchemaDrift)
	}

	snapshot, err := mysign.Parse(trimmed)
	if err != nil {
		return mysign.Snapshot{}, secrets.Session{}, fmt.Errorf("%w: %v", ErrSchemaDrift, err)
	}
	if snapshot.NextRequestToken == "" {
		return mysign.Snapshot{}, secrets.Session{}, fmt.Errorf("%w: data response has no request token", ErrSchemaDrift)
	}
	return snapshot, client.captureSession(snapshot.NextRequestToken), nil
}

func (client *Client) postJSON(
	ctx context.Context,
	path string,
	body any,
	budget *requestBudget,
) ([]byte, error) {
	if err := budget.take(); err != nil {
		return nil, err
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: path}).String()
	if err := ValidateReadRequest(http.MethodPost, endpoint); err != nil {
		return nil, err
	}
	if !client.lastCall.IsZero() && client.delay > 0 {
		if err := client.sleep(ctx, client.delay); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "myyolo-cli")

	response, err := client.http.Do(request)
	client.lastCall = time.Now()
	if err != nil {
		return nil, fmt.Errorf("request myYOLO: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, ErrSessionExpired
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("myYOLO rate limit: retry later")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("myYOLO returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read myYOLO response: %w", err)
	}
	if len(data) > maxResponse {
		return nil, fmt.Errorf("myYOLO response exceeds %d bytes", maxResponse)
	}
	return data, nil
}

func (client *Client) restoreCookies(session secrets.Session) {
	cookies := make([]*http.Cookie, 0, len(session.Cookies))
	for _, cookie := range session.Cookies {
		if cookie.Name == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	client.http.Jar.SetCookies(client.baseURL, cookies)
}

func (client *Client) captureSession(token string) secrets.Session {
	cookies := client.http.Jar.Cookies(client.baseURL)
	saved := make([]secrets.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		saved = append(saved, secrets.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	return secrets.Session{NextRequestToken: token, Cookies: saved}
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

func looksLikeHTML(data []byte) bool {
	lower := strings.ToLower(string(bytes.TrimSpace(data)))
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html")
}

func safeMessage(message string) string {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "invalid") || strings.Contains(lower, "login") {
		return "invalid login"
	}
	return "login rejected"
}

func sameHostRedirect(
	baseURL *url.URL,
	previous func(*http.Request, []*http.Request) error,
) func(*http.Request, []*http.Request) error {
	return func(request *http.Request, via []*http.Request) error {
		if !strings.EqualFold(request.URL.Hostname(), baseURL.Hostname()) ||
			request.URL.Scheme != "https" {
			return errors.New("blocked redirect outside approved HTTPS host")
		}
		if err := ValidateReadRequest(request.Method, request.URL.String()); err != nil {
			return fmt.Errorf("blocked redirect: %w", err)
		}
		if previous != nil {
			return previous(request, via)
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
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
