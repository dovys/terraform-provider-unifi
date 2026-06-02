// Package protect is the hand-written, ergonomic client for the UniFi Protect
// application — the c.Protect() arm of the substruct API. It composes the
// generated wire types (internal/unifi/protect/wire) with the one authenticated
// classic (cookie+CSRF) session the console client owns, and organizes
// operations into groups:
//
//	c.Protect().Cameras().List(ctx)
//	c.Protect().NVR().Get(ctx)
//	c.Protect().Liveviews().Create(ctx, &lv)
//
// Surface: the private REST API the web UI calls, under /proxy/protect/api/...,
// over the same session as the Network app — no api_key, one session (see
// docs/apps/protect.md). Responses are bare JSON (not the legacy {meta,data}
// envelope); writes are PATCH of the owned fields only.
package protect

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

// core carries the shared session for every Protect group. Protect is a
// console-global app (not site-scoped), so the only state is the session; the
// /proxy/protect/api prefix is centralized in apiPath.
type core struct {
	session doer
}

// apiPath builds a Protect private-API path: /proxy/protect/api/<suffix>.
func (c *core) apiPath(suffix string) string {
	return "/proxy/protect/api/" + strings.TrimPrefix(suffix, "/")
}

// ErrNotFound is returned by Get/Update/Delete when the object is absent.
// Resources translate it into remove-from-state on Read.
var ErrNotFound = errors.New("object not found")

// isNotFound reports whether a transport error denotes a missing object: an
// HTTP 404, or a 400 whose message reads as not-found (Protect occasionally
// returns 400 for a bad/absent id). The message heuristic is gated to 400 so an
// unrelated 5xx whose body happens to contain "not found" is never silently
// mapped to remove-from-state.
func isNotFound(err error) bool {
	var apiErr *transport.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound {
			return true
		}
		if apiErr.StatusCode == http.StatusBadRequest {
			m := strings.ToLower(apiErr.Message)
			return strings.Contains(m, "not found") ||
				strings.Contains(m, "nonexistent") ||
				strings.Contains(m, "could not find") ||
				strings.Contains(m, "does not exist")
		}
	}
	return false
}

// decode executes a Protect request and decodes the bare-JSON body into out.
// out may be nil to discard (e.g. DELETE). A not-found maps to ErrNotFound.
func decode[T any](ctx context.Context, c *core, method, path string, body any, out *T) error {
	resp, err := c.session.Do(ctx, method, path, body)
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("decode protect %s %s: %w", method, path, err)
	}
	return nil
}
