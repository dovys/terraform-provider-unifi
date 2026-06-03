package protect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Collection is the generic CRUD primitive every Protect group builds on, over
// a private-API resource at /proxy/protect/api/<name> (bare JSON). T is the
// generated wire read type.
//
// Writes carry only the owned fields. The generated wire types have no
// omitempty (every field, incl. controller-managed telemetry, would serialize),
// so marshaling a full read object on PATCH would clobber state. Create/Update
// therefore take an owned-field overlay (a small struct with omitempty tags
// holding exactly the modeled fields) — the Protect analogue of the Network
// sparse PUT. PATCH is partial server-side, so no read-then-merge round-trip is
// needed for top-level fields: the overlay is sent as-is. (Nested sub-objects
// the controller replaces wholesale are merged client-side by the resource.)
type Collection[T any] struct {
	core *core
	name string // plural resource segment, e.g. "cameras", "liveviews"
}

func (s Collection[T]) path(id string) string {
	p := s.name
	if id != "" {
		p += "/" + url.PathEscape(id) // defense-in-depth: ids may originate from import input
	}
	return s.core.apiPath(p)
}

// List returns every object of the type.
func (s Collection[T]) List(ctx context.Context) ([]T, error) {
	var out []T
	if err := decode(ctx, s.core, http.MethodGet, s.path(""), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns the object with the given id, or ErrNotFound.
func (s Collection[T]) Get(ctx context.Context, id string) (*T, error) {
	var out T
	if err := decode(ctx, s.core, http.MethodGet, s.path(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create POSTs an owned-field overlay and returns the controller's stored copy.
func (s Collection[T]) Create(ctx context.Context, owned any) (*T, error) {
	var out T
	if err := decode(ctx, s.core, http.MethodPost, s.path(""), owned, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update PATCHes an owned-field overlay onto the object by id and returns the
// controller's stored copy. owned must carry only the modeled fields (omitempty
// overlay), never the full read object.
func (s Collection[T]) Update(ctx context.Context, id string, owned any) (*T, error) {
	if id == "" {
		return nil, fmt.Errorf("protect: update %s: missing id", s.name)
	}
	var out T
	if err := decode(ctx, s.core, http.MethodPatch, s.path(id), owned, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes the object by id. An already-absent object is not an error.
func (s Collection[T]) Delete(ctx context.Context, id string) error {
	err := decode[any](ctx, s.core, http.MethodDelete, s.path(id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
