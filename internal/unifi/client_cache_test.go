package unifi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// loginCountingServer is a console mock that tallies /api/auth/login hits so a
// test can assert how many times a session was established.
func loginCountingServer(t *testing.T, logins *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			atomic.AddInt32(logins, 1)
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestClientCacheReusesSession: identical configs reuse one authenticated client
// (one login), while a credential change builds a fresh one.
func TestClientCacheReusesSession(t *testing.T) {
	resetClientCache()
	t.Cleanup(resetClientCache)

	var logins int32
	srv := loginCountingServer(t, &logins)
	defer srv.Close()

	cfg := Config{Host: srv.URL, Username: "admin", Password: "pw"}
	c1, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient #1: %v", err)
	}
	c2, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient #2: %v", err)
	}
	if c1 != c2 {
		t.Error("identical config returned different clients; want the cached one reused")
	}
	if n := atomic.LoadInt32(&logins); n != 1 {
		t.Errorf("logins = %d, want 1 (session reused across NewClient calls)", n)
	}

	// A different password must not reuse the session.
	if _, err := NewClient(context.Background(), Config{Host: srv.URL, Username: "admin", Password: "other"}); err != nil {
		t.Fatalf("NewClient #3: %v", err)
	}
	if n := atomic.LoadInt32(&logins); n != 2 {
		t.Errorf("logins = %d after a credential change, want 2", n)
	}
}

// TestClientCacheDisabled: with UNIFI_DISABLE_CLIENT_CACHE set, each NewClient
// builds and logs in afresh.
func TestClientCacheDisabled(t *testing.T) {
	resetClientCache()
	t.Cleanup(resetClientCache)
	t.Setenv("UNIFI_DISABLE_CLIENT_CACHE", "1")

	var logins int32
	srv := loginCountingServer(t, &logins)
	defer srv.Close()

	cfg := Config{Host: srv.URL, Username: "admin", Password: "pw"}
	c1, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient #1: %v", err)
	}
	c2, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient #2: %v", err)
	}
	if c1 == c2 {
		t.Error("cache disabled but the same client was returned")
	}
	if n := atomic.LoadInt32(&logins); n != 2 {
		t.Errorf("logins = %d with the cache disabled, want 2", n)
	}
}
