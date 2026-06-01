package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestIntegration_SetsAPIKeyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-KEY")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	i := NewIntegration(srv.URL, "secret-key", &http.Client{}, 0)
	if _, err := i.Do(context.Background(), http.MethodGet, "/proxy/network/integration/v1/sites", nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotKey != "secret-key" {
		t.Errorf("X-API-KEY = %q, want secret-key", gotKey)
	}
}

func TestIntegration_APIErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	i := NewIntegration(srv.URL, "k", &http.Client{}, 0)
	_, err := i.Do(context.Background(), http.MethodGet, "/proxy/network/integration/v1/nope", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Surface != "integration" {
		t.Errorf("surface = %q, want integration", apiErr.Surface)
	}
}

func TestIntegration_429Retry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	i := NewIntegration(srv.URL, "k", &http.Client{}, 2)
	resp, err := i.Do(context.Background(), http.MethodGet, "/proxy/network/integration/v1/sites", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (429 then retry)", got)
	}
}
