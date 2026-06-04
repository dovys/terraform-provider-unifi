// Package console is the hand-written client for the UniFi OS (unifi-core)
// console surface — the c.Console() arm of the substruct API. Unlike the
// Network/Protect/Access apps, this is the OS layer itself: paths are
// baseURL-rooted under /api/... (a peer of /api/system, /api/users), not under
// /proxy/<app>/. It runs over the same one cookie+CSRF classic session the rest
// of the provider uses — the transport attaches CSRF to any mutating request
// regardless of path, so /api/... is reached unchanged.
//
//	c.Console().Certificates().List(ctx)
//	c.Console().Certificates().StartChallenge(ctx, req)
//
// Wire types (types.go) are HAND-WRITTEN — a documented exception to invariant 2
// (generated wire from ../unifi-specs). This OS surface is in no OpenAPI spec;
// it was reverse-engineered from the console UI bundle's zod schemas, so there
// is nothing to codegen from. The surface is tiny (one resource family), and
// hand-authoring a synthetic spec for it would be disproportionate. The endpoint
// map + schemas are recorded in ../unifi-specs/unifi-os for reproducibility.
package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// doer is the minimal session seam the console groups need: one authenticated
// classic request over the shared cookie+CSRF session. *transport.Classic
// satisfies it; tests inject a fake.
type doer interface {
	Do(ctx context.Context, method, path string, body any) (*transport.Response, error)
}

// core carries the shared session for every console group. The console OS
// surface is global (not site-scoped), so the only state is the session; the
// /api/ prefix is centralized in apiPath.
type core struct {
	session doer
}

// apiPath builds a unifi-core OS path: /api/<suffix>. Unlike the app clients
// there is no /proxy/<app>/ prefix — these are top-level OS routes.
func (c *core) apiPath(suffix string) string {
	return "/api/" + strings.TrimPrefix(suffix, "/")
}

// ErrNotFound is returned by Get/Renew/Revoke when the certificate is absent
// (HTTP 404 or the API's NOT_FOUND code). Resources translate it into
// remove-from-state on Read (invariant 7).
var ErrNotFound = errors.New("certificate not found")

// ConsoleError is a typed unifi-core error carrying the API's machine-readable
// code (one of the Err* constants in types.go). Resources match on Code to turn
// it into an actionable diagnostic rather than a raw 4xx (invariant 3). The OS
// surface reports errors as a {code, level, message} envelope, which may ride on
// a non-2xx response or (for the challenge endpoints) a 2xx body.
type ConsoleError struct {
	Code     string
	Level    string
	Message  string
	HTTPCode int
}

func (e *ConsoleError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

// parseConsoleError reports the {code,...} error envelope in body, or nil if
// body is not an error shape. Top-level `code` appears only on error envelopes:
// the success payloads (a Certificate, a {name,content} challenge result, a
// certificate list) never carry it, so this is a safe universal discriminator
// on both 2xx and non-2xx bodies.
func parseConsoleError(body []byte, httpCode int) *ConsoleError {
	var p struct {
		Code    string `json:"code"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.Code != "" {
		return &ConsoleError{Code: p.Code, Level: p.Level, Message: p.Message, HTTPCode: httpCode}
	}
	return nil
}

// do executes a console request and decodes the JSON body into out (out may be
// nil to discard). A {code,...} error envelope — on either a non-2xx response
// or a 2xx body — becomes a *ConsoleError; a NOT_FOUND (or HTTP 404) maps to
// ErrNotFound.
func do[T any](ctx context.Context, c *core, method, path string, body any, out *T) error {
	resp, err := c.session.Do(ctx, method, path, body)
	if err != nil {
		var apiErr *transport.APIError
		if errors.As(err, &apiErr) {
			if ce := parseConsoleError(apiErr.Body, apiErr.StatusCode); ce != nil {
				if ce.Code == ErrCodeNotFound {
					return ErrNotFound
				}
				return ce
			}
			if apiErr.StatusCode == http.StatusNotFound {
				return ErrNotFound
			}
		}
		return err
	}
	if ce := parseConsoleError(resp.Body, resp.StatusCode); ce != nil {
		if ce.Code == ErrCodeNotFound {
			return ErrNotFound
		}
		return ce
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("decode console %s %s: %w", method, path, err)
	}
	return nil
}

// sleep waits for d, returning early if the context is cancelled. (The transport
// has an identical unexported helper for its 429 backoff; the challenge-complete
// propagation retry needs the same here.)
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
