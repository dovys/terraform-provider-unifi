package unifi

import (
	"context"
	"fmt"
)

const routingType = "routing"

// StaticRoute is a legacy `/rest/routing` document — a user-defined route on the
// gateway. The controller's `type` is always "static-route"; the
// `static-route_*` keys carry the route itself. A route is one of three kinds
// (static-route_type): nexthop-route (via a gateway IP), interface-route (out an
// interface), or blackhole (drop). Modeled from documented controller behavior —
// the field set is HIL-unverified (this network has no static routes) — see
// TODO.md. Pointers + omitempty give three-state GET-merge-PUT semantics.
type StaticRoute struct {
	ID        string `json:"_id,omitempty"`
	SiteID    string `json:"site_id,omitempty"`
	Name      string `json:"name"`
	Enabled   *bool  `json:"enabled,omitempty"`
	Type      string `json:"type,omitempty"` // always "static-route"
	Network   string `json:"static-route_network,omitempty"`
	NextHop   string `json:"static-route_nexthop,omitempty"`
	Distance  *int   `json:"static-route_distance,omitempty"`
	RouteType string `json:"static-route_type,omitempty"` // nexthop-route | interface-route | blackhole
	Interface string `json:"static-route_interface,omitempty"`
}

// Static route CRUD (legacy /rest/routing). Update is GET-merge-PUT so
// controller-managed fields survive.

func (c *client) ListStaticRoutes(ctx context.Context) ([]StaticRoute, error) {
	return legacyList[StaticRoute](ctx, c, routingType)
}

func (c *client) GetStaticRoute(ctx context.Context, id string) (*StaticRoute, error) {
	return legacyGet[StaticRoute](ctx, c, routingType, id)
}

func (c *client) CreateStaticRoute(ctx context.Context, r *StaticRoute) (*StaticRoute, error) {
	return legacyCreate[StaticRoute](ctx, c, routingType, r)
}

func (c *client) UpdateStaticRoute(ctx context.Context, r *StaticRoute) (*StaticRoute, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateStaticRoute: missing id")
	}
	return legacyMergePut[StaticRoute](ctx, c, routingType, r.ID, r)
}

func (c *client) DeleteStaticRoute(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, routingType, id)
}
