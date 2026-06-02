package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// The generic CRUD primitives every group builds on. A group embeds the
// collection for its surface and adds object-specific behavior (Get-by-filter
// where the API lacks GET-by-id, merge-update for partly-owned legacy docs,
// the v2 _id-in-url-and-body contract). T is the generated wire type.

// LegacyCollection is a CRUD collection over a legacy /rest/<type> object,
// keyed by the controller's collection-name token (e.g. "networkconf").
type LegacyCollection[T any] struct {
	core *core
	typ  string
}

func (s LegacyCollection[T]) restPath(id string) string {
	p := "rest/" + s.typ
	if id != "" {
		p += "/" + id
	}
	return s.core.legacyPath(p)
}

// List returns every object of the type.
func (s LegacyCollection[T]) List(ctx context.Context) ([]T, error) {
	return legacyData[T](ctx, s.core, http.MethodGet, s.restPath(""), nil)
}

// Get returns the object with the given _id, or ErrNotFound.
func (s LegacyCollection[T]) Get(ctx context.Context, id string) (*T, error) {
	items, err := legacyData[T](ctx, s.core, http.MethodGet, s.restPath(id), nil)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

// Create adds an object and returns it as the controller stored it.
func (s LegacyCollection[T]) Create(ctx context.Context, body *T) (*T, error) {
	items, err := legacyData[T](ctx, s.core, http.MethodPost, s.restPath(""), body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("create %s returned no object", s.typ)
	}
	return &items[0], nil
}

// Update PUTs the object by id. The body is the typed object; omitempty fields
// drop out, so only set fields are written. Partly-owned documents that must
// not be whole-PUT use a group-level merge-update instead (added per group).
func (s LegacyCollection[T]) Update(ctx context.Context, id string, body *T) (*T, error) {
	items, err := legacyData[T](ctx, s.core, http.MethodPut, s.restPath(id), body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		// Some legacy PUTs echo nothing; re-read.
		return s.Get(ctx, id)
	}
	return &items[0], nil
}

// Delete removes the object by id. An already-absent object is not an error.
func (s LegacyCollection[T]) Delete(ctx context.Context, id string) error {
	_, err := legacyData[json.RawMessage](ctx, s.core, http.MethodDelete, s.restPath(id), nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// UpdateMerge performs GET-merge-PUT for a partly-owned legacy document: fetch
// the existing doc as a raw field map, overlay the owned (omitempty) fields of
// owned, and PUT the result. Controller-managed and unmodeled fields survive
// untouched — the invariant for legacy docs whose full surface the provider
// does not own (never a whole-document PUT). owned is typically a *T or a subset
// struct whose omitempty fields are exactly the ones the caller intends to set.
func (s LegacyCollection[T]) UpdateMerge(ctx context.Context, id string, owned any) (*T, error) {
	existing, err := legacyData[map[string]json.RawMessage](ctx, s.core, http.MethodGet, s.restPath(id), nil)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, ErrNotFound
	}
	doc := existing[0]
	fields, err := structToRawMap(owned)
	if err != nil {
		return nil, err
	}
	for k, v := range fields {
		doc[k] = v
	}
	items, err := legacyData[T](ctx, s.core, http.MethodPut, s.restPath(id), doc)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return s.Get(ctx, id)
	}
	return &items[0], nil
}

// structToRawMap marshals v to a JSON field map. omitempty fields drop out, so
// only set fields overlay onto the fetched document in UpdateMerge.
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

// V2Collection is a CRUD collection over a site-scoped v2 resource (bare JSON).
// path is the resource segment, e.g. "static-dns".
type V2Collection[T any] struct {
	core *core
	path string
}

// List returns every object at the resource path.
func (s V2Collection[T]) List(ctx context.Context) ([]T, error) {
	var out []T
	if err := v2Decode(ctx, s.core, http.MethodGet, s.path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Create adds an object and returns the controller's stored copy.
func (s V2Collection[T]) Create(ctx context.Context, body *T) (*T, error) {
	var out T
	if err := v2Decode(ctx, s.core, http.MethodPost, s.path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update PUTs the object at <path>/<id>. The v2 write contract requires _id in
// both the URL and the body — callers pass id and a body carrying _id.
func (s V2Collection[T]) Update(ctx context.Context, id string, body *T) (*T, error) {
	var out T
	if err := v2Decode(ctx, s.core, http.MethodPut, s.path+"/"+id, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes the object at <path>/<id>. An already-absent object (and the
// 204 some v2 deletes return) is not an error.
func (s V2Collection[T]) Delete(ctx context.Context, id string) error {
	err := v2Decode[any](ctx, s.core, http.MethodDelete, s.path+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
