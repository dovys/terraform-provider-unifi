package unifi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-version"

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

func TestDeriveFlags(t *testing.T) {
	if f := deriveFlags(nil); f.IntegrationWrites || f.IntegrationDNSPolicies {
		t.Errorf("deriveFlags(nil) = %+v, want all false", f)
	}
	mustV := func(s string) *version.Version {
		v, err := version.NewVersion(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return v
	}
	for _, c := range []struct {
		v    string
		want bool
	}{
		{"9.0.0", false},
		{"10.0.0", false},
		{"10.1.83", false},
		{"10.1.84", true},
		{"10.4.57", true},
		{"11.0.0", true},
	} {
		f := deriveFlags(mustV(c.v))
		if f.IntegrationWrites != c.want {
			t.Errorf("v=%s IntegrationWrites=%v, want %v", c.v, f.IntegrationWrites, c.want)
		}
		if f.IntegrationDNSPolicies != c.want {
			t.Errorf("v=%s IntegrationDNSPolicies=%v, want %v", c.v, f.IntegrationDNSPolicies, c.want)
		}
		if f.StaticDNSOnV2() == c.want {
			t.Errorf("v=%s StaticDNSOnV2=%v, want %v (inverse of DNS policies)", c.v, f.StaticDNSOnV2(), !c.want)
		}
	}
}

func TestProbe_VersionAndSites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
		case "/proxy/network/integration/v1/sites":
			_, _ = w.Write([]byte(`{"offset":0,"limit":25,"count":1,"totalCount":1,"data":[{"id":"uuid-1","internalReference":"default","name":"Default"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	classic := classicTransport(t, srv.URL)
	integ := transport.NewIntegration(srv.URL, "k", &http.Client{}, 0)

	caps, err := probe(context.Background(), classic, integ, "default")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if caps.VersionRaw != "10.4.57" {
		t.Errorf("VersionRaw = %q, want 10.4.57", caps.VersionRaw)
	}
	if caps.Version == nil {
		t.Fatal("Version not parsed")
	}
	if !caps.Flags.IntegrationWrites {
		t.Error("IntegrationWrites = false at 10.4.57, want true")
	}
	site, ok := caps.Sites["default"]
	if !ok {
		t.Fatalf("site %q not in map %+v", "default", caps.Sites)
	}
	if site.ID != "uuid-1" || site.Name != "Default" {
		t.Errorf("site = %+v, want id uuid-1 / name Default", site)
	}
}

func TestProbe_ClassicOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"9.0.0"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	caps, err := probe(context.Background(), classicTransport(t, srv.URL), nil, "default")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if caps.VersionRaw != "9.0.0" {
		t.Errorf("VersionRaw = %q, want 9.0.0", caps.VersionRaw)
	}
	if caps.Flags.IntegrationWrites {
		t.Error("IntegrationWrites = true at 9.0.0, want false")
	}
	if len(caps.Sites) != 0 {
		t.Errorf("Sites = %+v, want empty (no integration transport)", caps.Sites)
	}
}

func TestProbe_IntegrationOnlyVersionFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxy/network/integration/v1/sites":
			_, _ = w.Write([]byte(`{"data":[{"id":"u","internalReference":"default","name":"Default"}]}`))
		case "/proxy/network/integration/v1/application-info":
			_, _ = w.Write([]byte(`{"applicationVersion":"10.2.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	integ := transport.NewIntegration(srv.URL, "k", &http.Client{}, 0)
	caps, err := probe(context.Background(), nil, integ, "default")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if caps.VersionRaw != "10.2.0" {
		t.Errorf("VersionRaw = %q, want 10.2.0 (application-info fallback)", caps.VersionRaw)
	}
	if !caps.Flags.IntegrationWrites {
		t.Error("IntegrationWrites = false at 10.2.0, want true")
	}
}
