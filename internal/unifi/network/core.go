// Package network is the hand-written, ergonomic client for the UniFi Network
// application — the c.Network() arm of the substruct API. It composes the
// generated wire types (internal/unifi/network/wire) with one authenticated
// classic (cookie+CSRF) session, and organizes operations into groups:
//
//	c.Network().DNS().Records().List(ctx)
//	c.Network().WAN().MagicPause(ctx)
//	c.Network().Firewall().Rules().Create(ctx, &r)
//
// Two surfaces ride the same session: legacy /api/s/<site>/rest/<type> (the
// widest, most stable writes) and v2 /v2/api/site/<site>/<resource> (newer
// feature areas). The proxy path prefixes and wire envelopes live here; groups
// build on the generic LegacyCollection/V2Collection CRUD primitives.
package network

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
// tests inject a fake. Both surfaces (legacy /rest and v2) use the same Do.
type doer interface {
	Do(ctx context.Context, method, path string, body any) (*transport.Response, error)
}

// core carries the shared session + site for every Network group. The UniFi OS
// proxy path prefixes are centralized in the path builders below.
type core struct {
	session doer
	site    string
}

// legacyPath builds a legacy surface path: /proxy/network/api/s/<site>/<suffix>.
func (c *core) legacyPath(suffix string) string {
	return fmt.Sprintf("/proxy/network/api/s/%s/%s", c.site, strings.TrimPrefix(suffix, "/"))
}

// v2Path builds a site-scoped v2 surface path:
// /proxy/network/v2/api/site/<site>/<suffix>.
func (c *core) v2Path(suffix string) string {
	return fmt.Sprintf("/proxy/network/v2/api/site/%s/%s", c.site, strings.TrimPrefix(suffix, "/"))
}

// ErrNotFound is returned by Get/Update/Delete when the object is absent.
// Resources translate it into remove-from-state on Read.
var ErrNotFound = errors.New("object not found")

// isNotFound reports whether a transport error denotes a missing object. The
// legacy API is inconsistent: a missing object may be HTTP 404, or HTTP 400/200
// with rc=error and an "UnknownObject"-class message.
func isNotFound(err error) bool {
	var apiErr *transport.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound {
			return true
		}
		m := strings.ToLower(apiErr.Message)
		return strings.Contains(m, "unknownobject") ||
			strings.Contains(m, "not found") ||
			strings.Contains(m, "no such")
	}
	return false
}

// --- legacy surface (/api/s/<site>/rest/<type>) — {meta,data:[]} envelope ---

// legacyEnvelope wraps every legacy REST/stat response.
type legacyEnvelope[T any] struct {
	Meta legacyMeta `json:"meta"`
	Data []T        `json:"data"`
}

type legacyMeta struct {
	RC  string `json:"rc"`
	Msg string `json:"msg,omitempty"`
}

func (m legacyMeta) ok() bool { return m.RC == "ok" }

// legacyData executes a legacy request and returns the unwrapped data slice. A
// not-found (HTTP 404 or rc=error UnknownObject) maps to ErrNotFound.
func legacyData[T any](ctx context.Context, c *core, method, path string, body any) ([]T, error) {
	resp, err := c.session.Do(ctx, method, path, body)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var env legacyEnvelope[T]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("decode legacy %s %s: %w", method, path, err)
	}
	if !env.Meta.ok() {
		if strings.Contains(strings.ToLower(env.Meta.Msg), "unknownobject") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("controller error on %s %s: %s", method, path, env.Meta.Msg)
	}
	return env.Data, nil
}

// --- v2 surface (/v2/api/site/<site>/<path>) — bare JSON ---

// v2Decode executes a v2 request and decodes the bare-JSON body into out. out
// may be nil to discard (e.g. DELETE). A not-found maps to ErrNotFound.
func v2Decode[T any](ctx context.Context, c *core, method, suffix string, body any, out *T) error {
	resp, err := c.session.Do(ctx, method, c.v2Path(suffix), body)
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
		return fmt.Errorf("decode v2 %s %s: %w", method, suffix, err)
	}
	return nil
}
