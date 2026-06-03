package transport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// makeToken builds an unsigned JWT carrying the given csrf claim and expiry —
// enough for the classic session, which only reads claims (never verifies).
func makeToken(t *testing.T, csrf string, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := map[string]any{"csrfToken": csrf}
	if !exp.IsZero() {
		payload["exp"] = exp.Unix()
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(pb) + ".sig"
}

func newClassicForTest(t *testing.T, baseURL string, retries int) *Classic {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return NewClassic(baseURL, "admin", "pw", &http.Client{Jar: jar}, retries)
}

func TestClassicLogin_CSRFFromHeader(t *testing.T) {
	var gotCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			// Header wins over the JWT claim, even when both are present.
			w.Header().Set("X-CSRF-Token", "hdr-csrf")
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: makeToken(t, "jwt-csrf", time.Now().Add(time.Hour)), Path: "/"})
			w.WriteHeader(http.StatusOK)
		default:
			gotCSRF = r.Header.Get("X-Csrf-Token")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		}
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 0)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := c.Do(context.Background(), http.MethodPost, "/proxy/network/api/s/default/rest/x", map[string]any{"a": 1}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotCSRF != "hdr-csrf" {
		t.Errorf("CSRF on mutating request = %q, want %q", gotCSRF, "hdr-csrf")
	}
}

func TestClassicLogin_CSRFFromJWTFallback(t *testing.T) {
	var gotCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			// No CSRF header — must fall back to the TOKEN JWT's csrfToken claim.
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: makeToken(t, "jwt-csrf", time.Now().Add(time.Hour)), Path: "/"})
			w.WriteHeader(http.StatusOK)
		default:
			gotCSRF = r.Header.Get("X-Csrf-Token")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 0)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := c.Do(context.Background(), http.MethodPut, "/proxy/network/api/s/default/rest/x", map[string]any{}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotCSRF != "jwt-csrf" {
		t.Errorf("CSRF on mutating request = %q, want %q (JWT fallback)", gotCSRF, "jwt-csrf")
	}
}

func TestClassicLogin_TwoFactor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(499)
		_, _ = w.Write([]byte(`api.err.Ubic2faTokenRequired`))
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 0)
	err := c.Login(context.Background())
	var twoFA *TwoFactorError
	if !errors.As(err, &twoFA) {
		t.Fatalf("login error = %v, want *TwoFactorError", err)
	}
}

func TestClassicLogin_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"meta":{"rc":"error","msg":"api.err.Invalid"}}`))
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 0)
	err := c.Login(context.Background())
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("login error = %v, want *AuthError", err)
	}
	if authErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", authErr.StatusCode)
	}
}

func TestClassicLogin_429Retry(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			t.Errorf("unexpected path %q", r.URL.Path)
			return
		}
		// First login is rate-limited; the second succeeds.
		if atomic.AddInt32(&logins, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: makeToken(t, "c", time.Now().Add(time.Hour)), Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 3)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	if got := atomic.LoadInt32(&logins); got != 2 {
		t.Errorf("login attempts = %d, want 2 (429 then retry)", got)
	}
}

func TestClassicLogin_429ExhaustsRetries(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&logins, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 2)
	err := c.Login(context.Background())
	var authErr *AuthError
	if !errors.As(err, &authErr) || authErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("login error = %v, want *AuthError with 429", err)
	}
	// maxRetries=2 → attempts 0,1 retry, attempt 2 returns the 429: 3 total.
	if got := atomic.LoadInt32(&logins); got != 3 {
		t.Errorf("login attempts = %d, want 3 (maxRetries=2 + final)", got)
	}
}

func TestClassic_ReauthOnUnauthorized(t *testing.T) {
	var logins, dataCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			atomic.AddInt32(&logins, 1)
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: makeToken(t, "c", time.Now().Add(time.Hour)), Path: "/"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if atomic.AddInt32(&dataCalls, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized) // first data call: token "lapsed"
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 0)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	resp, err := c.Do(context.Background(), http.MethodGet, "/proxy/network/api/s/default/stat/sysinfo", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&logins); got != 2 {
		t.Errorf("logins = %d, want 2 (initial + reauth)", got)
	}
	if got := atomic.LoadInt32(&dataCalls); got != 2 {
		t.Errorf("data calls = %d, want 2 (401 then retry)", got)
	}
}

func TestClassic_429Retry(t *testing.T) {
	var dataCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: makeToken(t, "c", time.Now().Add(time.Hour)), Path: "/"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if atomic.AddInt32(&dataCalls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer srv.Close()

	c := newClassicForTest(t, srv.URL, 3)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	resp, err := c.Do(context.Background(), http.MethodGet, "/proxy/network/api/s/default/rest/x", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&dataCalls); got != 2 {
		t.Errorf("data calls = %d, want 2 (429 then retry)", got)
	}
}

func TestDecodeJWTClaims(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	tok := makeToken(t, "the-csrf", exp)
	claims, err := decodeJWTClaims(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if claims.CSRFToken != "the-csrf" {
		t.Errorf("csrf = %q, want the-csrf", claims.CSRFToken)
	}
	if !claims.expiry().Equal(exp) {
		t.Errorf("exp = %v, want %v", claims.expiry(), exp)
	}
	if _, err := decodeJWTClaims("notajwt"); err == nil {
		t.Error("expected error decoding non-JWT")
	}
}
