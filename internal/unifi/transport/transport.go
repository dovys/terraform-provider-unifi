// Package transport implements the HTTP surfaces of a UniFi OS console. Two
// transports cover the three surfaces: Classic carries the cookie+CSRF session
// shared by the legacy REST and v2 surfaces; Integration carries the X-API-KEY
// session for Integration v1. Envelope unwrapping is the caller's job — the
// three surfaces use different envelopes.
//
// Resource code never imports this package (development.md invariant 1); it
// reaches these transports only through the unifi.Client interface.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Response is the raw result of a transport request, after the HTTP exchange
// (and any re-auth / 429 retry) completes.
type Response struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

// IsSuccess reports whether the status is in the 2xx range.
func (r *Response) IsSuccess() bool { return r.StatusCode >= 200 && r.StatusCode < 300 }

// APIError is returned for a non-2xx response from a UniFi surface.
type APIError struct {
	Surface    string // "integration" or "classic"
	Method     string
	Path       string
	StatusCode int
	Message    string
	Body       []byte
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s API %s %s: HTTP %d: %s", e.Surface, e.Method, e.Path, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s API %s %s: HTTP %d", e.Surface, e.Method, e.Path, e.StatusCode)
}

func newRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func firstHeader(h http.Header, keys ...string) string {
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return ""
}

// extractMessage best-effort pulls a human-readable message out of an error
// body (a legacy {"meta":{"msg":…}} envelope, or a bare string).
func extractMessage(body []byte) string {
	var legacy struct {
		Meta struct {
			Msg string `json:"msg"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &legacy); err == nil && legacy.Meta.Msg != "" {
		return legacy.Meta.Msg
	}
	// Integration v1 error shape: {"statusCode":…,"statusName":…,"message":…}.
	var generic struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &generic); err == nil && generic.Message != "" {
		return generic.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

// retryAfter computes the wait before a 429 retry: the Retry-After header when
// present (architecture.md §6), else a capped exponential backoff.
func retryAfter(h http.Header, attempt int) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	d := time.Duration(500*(1<<attempt)) * time.Millisecond
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

// sleep waits for d, returning early if the context is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
