package unifi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func consoleServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
		case "/proxy/network/integration/v1/sites":
			_, _ = w.Write([]byte(`{"data":[{"id":"uuid-1","internalReference":"default","name":"Default"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestNewClient_Success(t *testing.T) {
	srv := consoleServer(t)
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		Host:     srv.URL,
		Username: "admin",
		Password: "pw",
		APIKey:   "k",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Site() != "default" {
		t.Errorf("Site() = %q, want default", c.Site())
	}
	if c.SiteID() != "uuid-1" {
		t.Errorf("SiteID() = %q, want uuid-1", c.SiteID())
	}
	if got := c.Capabilities().VersionRaw; got != "10.4.57" {
		t.Errorf("VersionRaw = %q, want 10.4.57", got)
	}
	// Local DNS stays on v2 even on 10.4.57: the Integration dns/policies write
	// path isn't implemented yet (deriveFlags keeps IntegrationDNSPolicies off).
	if got := c.Router().Backend(OpLocalDNS); got != BackendV2 {
		t.Errorf("local DNS backend = %s, want v2 (Integration DNS not implemented)", got)
	}
	// Firmware-gated writes that ARE wired (adoption) route to Integration at 10.4.57.
	if got := c.Router().Backend(OpDeviceAdopt); got != BackendIntegration {
		t.Errorf("device adopt backend = %s, want integration (10.4.57)", got)
	}
}

func TestNewClient_DefaultsSiteWhenEmpty(t *testing.T) {
	srv := consoleServer(t)
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{Host: srv.URL, Username: "admin", Password: "pw", APIKey: "k"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Site() != "default" {
		t.Errorf("Site() = %q, want default", c.Site())
	}
}

func TestNewClient_MissingHost(t *testing.T) {
	_, err := NewClient(context.Background(), Config{Username: "a", Password: "b"})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v, want *ConfigError", err)
	}
}

func TestNewClient_NoCredentials(t *testing.T) {
	_, err := NewClient(context.Background(), Config{Host: "https://192.0.2.1"})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v, want *ConfigError", err)
	}
}

func TestNormalizeHost(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.1":             "https://192.0.2.1",
		"https://192.0.2.1/":    "https://192.0.2.1",
		"http://192.0.2.1:8443": "http://192.0.2.1:8443",
		"  unifi.example.com ":  "https://unifi.example.com",
	} {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
