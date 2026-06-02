package network

import (
	"context"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// StaticRoutes is the static-route collection (legacy /rest/routing): user
// routes on the gateway — next-hop, interface, or blackhole. Part of the
// routing group; BGP/OSPF (v2) join later as gated client methods.
func (c *Client) StaticRoutes() RoutingCollection {
	return RoutingCollection{LegacyCollection[wire.Routing]{core: c.core, typ: "routing"}}
}

// RoutingCollection is the static-route CRUD collection. routing is a small,
// fully-owned legacy object, so it overrides Update only to take the id from
// the object (a typed PUT of the owned fields, not a partial-merge).
type RoutingCollection struct {
	LegacyCollection[wire.Routing]
}

// Update PUTs the route by its _id. _id must be set.
func (s RoutingCollection) Update(ctx context.Context, r *wire.Routing) (*wire.Routing, error) {
	id := ""
	if r.UnderscoreId != nil {
		id = *r.UnderscoreId
	}
	if id == "" {
		return nil, errMissingID("routing")
	}
	return s.LegacyCollection.Update(ctx, id, r)
}
