package unifi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// fakeToken is a syntactically valid JWT with an empty ({}) payload — enough for
// the classic session at login time (GET probes need no CSRF/exp).
const fakeToken = "e30.e30.sig"

func classicTransport(t *testing.T, baseURL string) *transport.Classic {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return transport.NewClassic(baseURL, "admin", "pw", &http.Client{Jar: jar}, 0)
}

// TestProbe_Version verifies the Configure-time probe reads the Network
// application version from legacy /stat/sysinfo over the classic session.
func TestProbe_Version(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	caps, err := probe(context.Background(), classicTransport(t, srv.URL), "default")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if caps.VersionRaw != "10.4.57" {
		t.Errorf("VersionRaw = %q, want 10.4.57", caps.VersionRaw)
	}
	if caps.Version == nil {
		t.Fatal("Version not parsed")
	}
}

// TestProbe_UnparseableVersion keeps the raw string but leaves the parsed
// version nil when the controller reports something non-semver.
func TestProbe_UnparseableVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"weird-build"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	caps, err := probe(context.Background(), classicTransport(t, srv.URL), "default")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if caps.VersionRaw != "weird-build" {
		t.Errorf("VersionRaw = %q, want weird-build", caps.VersionRaw)
	}
	if caps.Version != nil {
		t.Errorf("Version = %v, want nil for unparseable string", caps.Version)
	}
}
