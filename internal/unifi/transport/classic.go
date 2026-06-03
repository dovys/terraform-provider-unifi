package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// reauthSkew re-authenticates this long before the token's exp so a
// long-running apply can't die when the JWT lapses mid-flight.
const reauthSkew = 60 * time.Second

// TwoFactorError is returned when the account requires MFA. Cloud/SSO accounts
// force 2FA and the login returns HTTP 499 (api.err.Ubic2faTokenRequired); the
// provider surfaces this as "use a local admin account".
type TwoFactorError struct{ Body string }

func (e *TwoFactorError) Error() string {
	return "account requires 2FA/MFA (HTTP 499): use a local UniFi OS admin account, not a cloud/SSO account"
}

// AuthError is a login failure (bad credentials, or an unreachable/unexpected
// auth endpoint).
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("classic login failed: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("classic login failed: HTTP %d", e.StatusCode)
}

// Classic is the cookie+CSRF session shared by the legacy REST and v2 surfaces.
// It logs in eagerly (so credential problems surface at Configure time), caches
// the CSRF token and the token's expiry parsed from the TOKEN JWT, attaches
// CSRF on mutating requests, and re-authenticates once on 401/403 or just
// before expiry.
type Classic struct {
	baseURL    string
	username   string
	password   string
	http       *http.Client
	maxRetries int

	mu       sync.Mutex
	csrf     string
	exp      time.Time
	loggedIn bool
}

// NewClassic builds a classic session. httpClient must carry a cookie jar so
// the TOKEN cookie persists across requests.
func NewClassic(baseURL, username, password string, httpClient *http.Client, maxRetries int) *Classic {
	return &Classic{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		http:       httpClient,
		maxRetries: maxRetries,
	}
}

// Login authenticates eagerly. Configure calls it once.
func (c *Classic) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx)
}

func (c *Classic) loginLocked(ctx context.Context) error {
	body := map[string]any{
		"username":   c.username,
		"password":   c.password,
		"rememberMe": true,
	}
	// The UniFi OS controller rate-limits logins (a handful per window); back-to-back
	// `terraform import` runs each log in fresh and trip a 429. Honor Retry-After (or
	// fall back to exponential backoff) and retry, just like the normal request path —
	// the request is rebuilt each attempt because the body reader is consumed.
	for attempt := 0; ; attempt++ {
		req, err := newRequest(ctx, http.MethodPost, c.baseURL+"/api/auth/login", body)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("classic login request: %w", err)
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		debugHTTP(ctx, http.MethodPost, c.baseURL+"/api/auth/login", body, resp.StatusCode, data)

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := sleep(ctx, retryAfter(resp.Header, attempt)); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == 499 {
			return &TwoFactorError{Body: string(data)}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &AuthError{StatusCode: resp.StatusCode, Message: extractMessage(data)}
		}

		// CSRF: prefer the response header, fall back to the TOKEN JWT claim.
		c.csrf = firstHeader(resp.Header, "X-Updated-Csrf-Token", "X-Csrf-Token")
		c.readTokenCookie(resp) // always sets exp; sets csrf only if the header was empty
		c.loggedIn = true
		return nil
	}
}

// readTokenCookie extracts exp (always) and the CSRF token (only when not
// already taken from a response header) from the TOKEN cookie on the login
// response. Reading directly from the response's Set-Cookie — rather than the
// jar — avoids depending on the cookie's Path matching the base URL.
func (c *Classic) readTokenCookie(resp *http.Response) {
	for _, ck := range resp.Cookies() {
		if ck.Name != "TOKEN" {
			continue
		}
		claims, err := decodeJWTClaims(ck.Value)
		if err != nil {
			return
		}
		c.exp = claims.expiry()
		if c.csrf == "" {
			c.csrf = claims.CSRFToken
		}
		return
	}
}

// Do executes a request against a classic surface. path is the full path after
// the host (e.g. "/proxy/network/api/s/default/rest/networkconf"). It
// authenticates lazily, attaches CSRF on mutating requests, retries 429s, and
// re-authenticates + retries once on 401/403.
func (c *Classic) Do(ctx context.Context, method, path string, body any) (*Response, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// The token may have lapsed or rotated; re-auth once and retry.
		if err := c.reauth(ctx); err != nil {
			return nil, err
		}
		resp, err = c.do(ctx, method, path, body)
		if err != nil {
			return nil, err
		}
	}
	if !resp.IsSuccess() {
		return resp, &APIError{
			Surface:    "classic",
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Message:    extractMessage(resp.Body),
			Body:       resp.Body,
		}
	}
	return resp, nil
}

func (c *Classic) ensureAuth(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loggedIn {
		return c.loginLocked(ctx)
	}
	if !c.exp.IsZero() && time.Now().After(c.exp.Add(-reauthSkew)) {
		return c.loginLocked(ctx)
	}
	return nil
}

func (c *Classic) reauth(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx)
}

// do performs one logical request, retrying on 429. The request is rebuilt each
// attempt because the body reader is consumed.
func (c *Classic) do(ctx context.Context, method, path string, body any) (*Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := newRequest(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, err
		}
		if isMutating(method) {
			c.mu.Lock()
			csrf := c.csrf
			c.mu.Unlock()
			if csrf != "" {
				req.Header.Set("X-Csrf-Token", csrf)
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			debugHTTP(ctx, method, c.baseURL+path, body, 0, nil)
			return nil, fmt.Errorf("classic %s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		debugHTTP(ctx, method, c.baseURL+path, body, resp.StatusCode, data)
		// A rotated CSRF token may ride along on any response.
		if v := firstHeader(resp.Header, "X-Updated-Csrf-Token"); v != "" {
			c.mu.Lock()
			c.csrf = v
			c.mu.Unlock()
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := sleep(ctx, retryAfter(resp.Header, attempt)); err != nil {
				return nil, err
			}
			continue
		}
		return &Response{StatusCode: resp.StatusCode, Body: data, Header: resp.Header}, nil
	}
}
