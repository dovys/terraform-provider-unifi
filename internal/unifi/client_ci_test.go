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
		case "/proxy/protect/api/nvr":
			_, _ = w.Write([]byte(`{"version":"7.1.69"}`))
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
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Site() != "default" {
		t.Errorf("Site() = %q, want default", c.Site())
	}
	if got := c.Capabilities().VersionRaw; got != "10.4.57" {
		t.Errorf("VersionRaw = %q, want 10.4.57", got)
	}
	if !c.Capabilities().ProtectInstalled {
		t.Errorf("ProtectInstalled = false, want true (console serves Protect)")
	}
	if got := c.Capabilities().ProtectVersionRaw; got != "7.1.69" {
		t.Errorf("ProtectVersionRaw = %q, want 7.1.69", got)
	}
}

// TestNewClient_ProtectAbsent: a console without Protect probes non-fatally and
// reports ProtectInstalled=false rather than failing NewClient.
func TestNewClient_ProtectAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/sysinfo":
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound) // no Protect application
		}
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{Host: srv.URL, Username: "admin", Password: "pw"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Capabilities().ProtectInstalled {
		t.Errorf("ProtectInstalled = true, want false (console has no Protect)")
	}
}

func TestNewClient_DefaultsSiteWhenEmpty(t *testing.T) {
	srv := consoleServer(t)
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{Host: srv.URL, Username: "admin", Password: "pw"})
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
