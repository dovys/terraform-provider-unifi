// Package transport implements the HTTP surface of a UniFi OS console. The
// Classic transport carries the cookie+CSRF session shared by the legacy REST
// (`/api/s/<site>/...`) and v2 (`/v2/api/site/<site>/...`) surfaces. Envelope
// unwrapping is the caller's job — the two surfaces use different envelopes.
//
// Resource code never imports this package (development.md invariant 1); it
// reaches the transport only through the unifi.Client interface.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	Surface    string // "classic"
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
	// Generic error shape some endpoints use: {"message":…}.
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

// --- wire-level HTTP debug (UNIFI_DEBUG_HTTP) ---

// debugHTTPEnabled reports whether wire-level request/response dumping is on,
// gated by a non-empty UNIFI_DEBUG_HTTP so it never fires in normal runs.
func debugHTTPEnabled() bool { return os.Getenv("UNIFI_DEBUG_HTTP") != "" }

// debugHTTP dumps one request/response pair to stderr, with secret-bearing JSON
// fields masked, when UNIFI_DEBUG_HTTP is set. Every classic request and the
// login pass through it, so it captures the exact body the controller accepts or
// rejects — the tool for reverse-engineering which fields a surface will
// PUT/POST vs only GET. status 0 / nil respBody denotes a transport-level
// failure (no response). Output goes to stderr, never stdout: stdout carries the
// go-plugin gRPC handshake, while Terraform forwards plugin stderr to the
// operator.
func debugHTTP(method, url string, reqBody any, status int, respBody []byte) {
	if !debugHTTPEnabled() {
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== UNIFI HTTP %s %s ===\n", method, url)
	if reqBody != nil {
		if b, err := json.Marshal(reqBody); err == nil {
			fmt.Fprintf(&sb, "--> request:\n%s\n", redactJSON(b))
		}
	}
	if status != 0 {
		fmt.Fprintf(&sb, "<-- status: %d\n", status)
	}
	if len(respBody) > 0 {
		fmt.Fprintf(&sb, "<-- response:\n%s\n", redactJSON(respBody))
	}
	fmt.Fprint(os.Stderr, sb.String())
}

// secretKeyHints are JSON field-name substrings (case-insensitive) whose values
// debugHTTP masks. Credentials — login body, write-only passphrases the
// controller echoes — must never reach the debug stream.
var secretKeyHints = []string{
	"password", "passwd", "passphrase", "psk", "secret",
	"token", "cookie", "authorization", "privkey", "private_key", "wpa_psk",
}

// redactJSON pretty-prints JSON with secret-bearing values masked. On a parse
// failure it returns the raw text, truncated, so a non-JSON body still shows.
func redactJSON(b []byte) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		s := strings.TrimSpace(string(b))
		if len(s) > 2000 {
			s = s[:2000] + "…(truncated)"
		}
		return s
	}
	redactValue(v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(b)
	}
	return string(out)
}

// redactValue walks a decoded JSON value in place, masking every value under a
// secret-bearing key at any depth.
func redactValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecretKey(k) {
				t[k] = "***REDACTED***"
				continue
			}
			redactValue(val)
		}
	case []any:
		for _, val := range t {
			redactValue(val)
		}
	}
}

func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	for _, h := range secretKeyHints {
		if strings.Contains(lk, h) {
			return true
		}
	}
	return false
}
