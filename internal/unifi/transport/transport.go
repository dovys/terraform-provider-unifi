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

	"github.com/hashicorp/terraform-plugin-log/tflog"
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

// --- wire-level HTTP debug ---

// debugHTTP logs one request/response pair — method, URL, JSON body, with
// secret-bearing fields masked — through tflog at DEBUG. Every classic request
// and the login pass through it, so it captures the exact body the controller
// accepts or rejects: the tool for reverse-engineering which fields a surface
// will PUT/POST vs only GET. status 0 / nil respBody denotes a transport-level
// failure (no response).
//
// Routing through tflog (not raw stderr) is what makes it surface under
// `terraform`/`tofu`: the provider runs as a go-plugin child whose stderr the
// host discards without TF_LOG, but tflog rides the hclog sink the host parses.
// Capture it with `TF_LOG_PROVIDER=debug tofu apply 2>&1 | grep 'wire HTTP'`.
func debugHTTP(ctx context.Context, method, url string, reqBody any, status int, respBody []byte) {
	// Skip the marshal+redact work entirely when no provider logging is
	// configured — tflog.Debug would no-op anyway, but the fields (incl. the
	// JSON round-trip in redactJSON) are built eagerly before that gate. TF_LOG /
	// TF_LOG_PROVIDER is the same control tflog itself honors.
	if os.Getenv("TF_LOG") == "" && os.Getenv("TF_LOG_PROVIDER") == "" {
		return
	}
	fields := map[string]any{"method": method, "url": url}
	if reqBody != nil {
		if b, err := json.Marshal(reqBody); err == nil {
			fields["request"] = redactJSON(b)
		}
	}
	if status != 0 {
		fields["status"] = status
	}
	if len(respBody) > 0 {
		fields["response"] = redactJSON(respBody)
	}
	tflog.Debug(ctx, "wire HTTP", fields)
}

// secretKeyHints are JSON field-name substrings (case-insensitive) whose values
// debugHTTP masks. Credentials — the login body, and the passphrases / key
// material the controller echoes on GET (WLAN PSKs, IPsec/WireGuard preshared
// keys, CA and server private keys) — must never reach the debug stream. The
// "_key" hint covers the controller's *_key fields (x_ca_key, x_server_key,
// wireguard_client_preshared_key, …) without masking the bare "key" field
// (a static-DNS record's hostname). The api*/application*/consumer*/access*
// entries cover the camelCase DNS-provider credentials the console-certificate
// surface sends (apiKey, applicationKey, consumerKey, accessKeyId; apiToken /
// apiSecret / secretKey already match via "token" / "secret"). A bare-"key"
// value that is itself secret (a PEM private key — e.g. the BYO upload body,
// which sends the key under the field "key") is caught by value in isSecretValue.
var secretKeyHints = []string{
	"password", "passwd", "passphrase", "psk", "secret",
	"token", "cookie", "authorization", "privkey", "private_key", "wpa_psk",
	"_key", "pre_shared_key", "preshared_key",
	"apikey", "applicationkey", "consumerkey", "accesskey",
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
// secret-bearing key (by name) and every value that is itself secret material
// (a PEM private key, by content), at any depth.
func redactValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecretKey(k) || isSecretValue(val) {
				t[k] = "***REDACTED***"
				continue
			}
			redactValue(val)
		}
	case []any:
		for i, val := range t {
			if isSecretValue(val) {
				t[i] = "***REDACTED***"
				continue
			}
			redactValue(val)
		}
	}
}

// isSecretValue reports whether a decoded JSON value is itself secret material
// that must never be logged regardless of its field name — a PEM private key.
// The console BYO-upload body sends the key under the bare field "key" (which
// stays unmasked by name, since a bare "key" is usually a non-secret hostname),
// and the issue/upload response echoes it; matching on the PEM marker catches
// those by content. Public certificates ("BEGIN CERTIFICATE") are intentionally
// left visible.
func isSecretValue(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "PRIVATE KEY")
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
