package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// ErrNotFound is returned by typed Get/Update/Delete methods when the object is
// absent. Resources translate it into remove-from-state on Read (invariant 5).
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

// structToRawMap marshals v to JSON and back into a field map — the overlay used
// by GET-merge-PUT. omitempty fields drop out, so only set fields overwrite.
func structToRawMap(v any) (map[string]json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// --- legacy REST surface (/proxy/network/api/s/<site>/rest/<type>) ---

func legacyRESTPath(site, typ, id string) string {
	p := "rest/" + typ
	if id != "" {
		p += "/" + id
	}
	return legacyPath(site, p)
}

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
	var env models.LegacyEnvelope[T]
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

func legacyList[T any](ctx context.Context, c *client, typ string) ([]T, error) {
	return legacyData[T](ctx, c, http.MethodGet, legacyRESTPath(c.cfg.Site, typ, ""), nil)
}

func legacyGet[T any](ctx context.Context, c *client, typ, id string) (*T, error) {
	items, err := legacyData[T](ctx, c, http.MethodGet, legacyRESTPath(c.cfg.Site, typ, id), nil)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

func legacyCreate[T any](ctx context.Context, c *client, typ string, body any) (*T, error) {
	items, err := legacyData[T](ctx, c, http.MethodPost, legacyRESTPath(c.cfg.Site, typ, ""), body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("create %s returned no object", typ)
	}
	return &items[0], nil
}

func legacyPut[T any](ctx context.Context, c *client, typ, id string, body any) (*T, error) {
	items, err := legacyData[T](ctx, c, http.MethodPut, legacyRESTPath(c.cfg.Site, typ, id), body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		// Some legacy PUTs echo nothing; re-read.
		return legacyGet[T](ctx, c, typ, id)
	}
	return &items[0], nil
}

func legacyDelete(ctx context.Context, c *client, typ, id string) error {
	_, err := legacyData[json.RawMessage](ctx, c, http.MethodDelete, legacyRESTPath(c.cfg.Site, typ, id), nil)
	if errors.Is(err, ErrNotFound) {
		return nil // already gone
	}
	return err
}

// legacyGetMerged GETs a doc as a field map for GET-merge-PUT (invariant: owned
// fields only, unmodeled fields carried through as raw JSON).
func legacyGetMerged(ctx context.Context, c *client, typ, id string) (map[string]json.RawMessage, error) {
	items, err := legacyData[map[string]json.RawMessage](ctx, c, http.MethodGet, legacyRESTPath(c.cfg.Site, typ, id), nil)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

// --- v2 surface (/proxy/network/v2/api/site/<site>/<path>) — bare JSON ---

func v2Decode[T any](ctx context.Context, c *client, method, suffix string, body any, out *T) error {
	if c.classic == nil {
		return &ConfigError{Msg: "this operation requires username/password (a local admin session)"}
	}
	resp, err := c.classic.Do(ctx, method, v2Path(c.cfg.Site, suffix), body)
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

// --- Integration v1 surface (/proxy/network/integration/v1/<path>) ---

// integrationList pages through an Integration list endpoint and returns all
// items. limit follows the controller's page size.
func integrationListAll[T any](ctx context.Context, c *client, suffix string) ([]T, error) {
	if c.integration == nil {
		return nil, &ConfigError{Msg: "this operation requires an api_key (Integration API)"}
	}
	var all []T
	offset := 0
	for {
		sep := "?"
		if strings.Contains(suffix, "?") {
			sep = "&"
		}
		path := integrationPath(fmt.Sprintf("%s%soffset=%d&limit=200", suffix, sep, offset))
		resp, err := c.integration.Do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var page models.IntegrationPage[T]
		if err := json.Unmarshal(resp.Body, &page); err != nil {
			return nil, fmt.Errorf("decode integration %s: %w", suffix, err)
		}
		all = append(all, page.Data...)
		offset += len(page.Data)
		if len(page.Data) == 0 || offset >= page.TotalCount {
			break
		}
	}
	return all, nil
}

func integrationGet[T any](ctx context.Context, c *client, suffix string) (*T, error) {
	if c.integration == nil {
		return nil, &ConfigError{Msg: "this operation requires an api_key (Integration API)"}
	}
	resp, err := c.integration.Do(ctx, http.MethodGet, integrationPath(suffix), nil)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var out T
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode integration %s: %w", suffix, err)
	}
	return &out, nil
}
