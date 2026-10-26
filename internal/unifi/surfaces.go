package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// LegacyEnvelope wraps every legacy REST/stat response:
// {"meta":{"rc":"ok"},"data":[...]}.
type LegacyEnvelope[T any] struct {
	Meta LegacyMeta `json:"meta"`
	Data []T        `json:"data"`
}

// LegacyMeta is the status block on a legacy response. RC is "ok" on success;
// on failure it is "error" and Msg carries the controller's reason.
type LegacyMeta struct {
	RC  string `json:"rc"`
	Msg string `json:"msg,omitempty"`
}

// OK reports whether the legacy call succeeded.
func (m LegacyMeta) OK() bool { return m.RC == "ok" }

// ErrNotFound is returned by typed Get/Update/Delete methods when the object is
// absent. Resources translate it into remove-from-state on Read (invariant 6).
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
		return strings.Contains(m, "unknownobject") || strings.Contains(m, "not found") || strings.Contains(m, "no such")
	}
	return false
}

// --- legacy envelope surface (/proxy/network/api/...) ---

// legacyData executes a legacy request and returns the unwrapped data slice. It
// fails if the classic session is unavailable, the controller reports rc=error,
// or the body can't be decoded; a not-found maps to ErrNotFound.
func legacyData[T any](ctx context.Context, c *client, method, path string, body any) ([]T, error) {
	if c.classic == nil {
		return nil, &ConfigError{Msg: "this operation requires username/password (a local admin session)"}
	}
	resp, err := c.classic.Do(ctx, method, path, body)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var env LegacyEnvelope[T]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("decode legacy %s %s: %w", method, path, err)
	}
	if !env.Meta.OK() {
		if strings.Contains(strings.ToLower(env.Meta.Msg), "unknownobject") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("controller error on %s %s: %s", method, path, env.Meta.Msg)
	}
	return env.Data, nil
}
