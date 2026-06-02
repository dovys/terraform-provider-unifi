// Package access is the hand-written, ergonomic client for the UniFi Access
// application — the c.Access() arm of the substruct API. It composes the
// generated wire types (internal/unifi/access/wire) with the one authenticated
// classic (cookie+CSRF) session the console client owns, and organizes
// operations into groups:
//
//	c.Access().DoorGroups().List(ctx)
//
// Surface: the private web-UI API under /proxy/access/api/... (v1 + v2), over
// the same session as the Network and Protect apps — no api_key, one session
// (see docs/apps/access.md). Every response is wrapped in an {code,msg,data}
// envelope; the data payload is the typed object.
package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// doer is the minimal session seam the groups need: one authenticated classic
// request over the shared cookie+CSRF session. *transport.Classic satisfies it;
// tests inject a fake.
type doer interface {
	Do(ctx context.Context, method, path string, body any) (*transport.Response, error)
}

// core carries the shared session for every Access group. Access is a
// console-global app (not site-scoped); the /proxy/access prefix is centralized
// in proxyPath.
type core struct {
	session doer
}

// proxyPath builds an Access private-API path: /proxy/access/<suffix>, where
// suffix is the API-relative path (e.g. "api/v2/access/door_group").
func (c *core) proxyPath(suffix string) string {
	return "/proxy/access/" + strings.TrimPrefix(suffix, "/")
}

// ErrNotFound is returned by Get/Update/Delete when the object is absent.
// Resources translate it into remove-from-state on Read.
var ErrNotFound = errors.New("object not found")

// envelope is the Access response wrapper. The vendor types `code` as an integer
// but real responses have used both numeric (0) and string ("SUCCESS") success
// codes across versions, so it's decoded loosely. The success set is confirmed
// pending a live capture (see TODO.md).
type envelope struct {
	Code json.RawMessage `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (e envelope) ok() bool {
	switch strings.ToLower(strings.Trim(string(e.Code), `" `)) {
	case "", "0", "200", "success", "null":
		return true
	default:
		return false
	}
}

// isNotFound reports whether a transport error denotes a missing object: an HTTP
// 404, or a 400 whose message reads as not-found. Gated to 400 so an unrelated
// 5xx is never silently mapped to remove-from-state.
func isNotFound(err error) bool {
	var apiErr *transport.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound {
			return true
		}
		if apiErr.StatusCode == http.StatusBadRequest {
			return msgNotFound(apiErr.Message)
		}
	}
	return false
}

func msgNotFound(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") ||
		strings.Contains(m, "nonexistent") ||
		strings.Contains(m, "does not exist") ||
		strings.Contains(m, "no such")
}

// do executes an Access request, unwraps the {code,msg,data} envelope, and
// decodes the data payload into out (out may be nil to discard). A transport
// 404, or an envelope error whose message reads as not-found, maps to
// ErrNotFound; any other non-success envelope code is an error carrying msg.
func do[T any](ctx context.Context, c *core, method, suffix string, body any, out *T) error {
	resp, err := c.session.Do(ctx, method, c.proxyPath(suffix), body)
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if len(resp.Body) == 0 {
		return nil
	}
	var env envelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("decode access %s %s: %w", method, suffix, err)
	}
	if !env.ok() {
		if msgNotFound(env.Msg) {
			return ErrNotFound
		}
		return fmt.Errorf("access error on %s %s: %s (code %s)", method, suffix, env.Msg, string(env.Code))
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode access data %s %s: %w", method, suffix, err)
	}
	return nil
}
