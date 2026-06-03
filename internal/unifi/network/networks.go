package network

import (
	"context"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// Networks is the networks group: LANs/VLANs and the WAN interface networks,
// all carried by the legacy networkconf object (the widest, most stable write
// surface). WAN Magic / failover / load-balancing actions that sit on top of
// the WAN networks live in the WAN() group.
func (c *Client) Networks() NetworkCollection {
	return NetworkCollection{LegacyCollection[wire.Networkconf]{core: c.core, typ: "networkconf"}}
}

// NetworkCollection is the networkconf CRUD collection. networkconf is a
// partly-owned legacy document (the controller manages and returns fields the
// provider does not model), so updates go through GET-merge-PUT (Update,
// overridden here) rather than a whole-document PUT.
type NetworkCollection struct {
	LegacyCollection[wire.Networkconf]
}

// GetByName returns the networkconf with the given name (exact match), or
// ErrNotFound. Network names are unique per site, so this resolves a
// human-friendly import key (e.g. "WAN1", "Default") to the controller `_id`.
func (s NetworkCollection) GetByName(ctx context.Context, name string) (*wire.Networkconf, error) {
	confs, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range confs {
		if confs[i].Name != nil && *confs[i].Name == name {
			return &confs[i], nil
		}
	}
	return nil, ErrNotFound
}

// Update writes the provider-owned fields of n with a sparse PUT. The legacy
// /rest PUT is a server-side partial merge, so omitted fields — both
// controller-managed (attr_no_delete, routing_table_id, …) and unmodeled — keep
// their stored value untouched. n carries only what the provider sets (its
// omitempty fields) plus its _id; never the controller's full GET document.
func (s NetworkCollection) Update(ctx context.Context, n *wire.Networkconf) (*wire.Networkconf, error) {
	id := ""
	if n.UnderscoreId != nil {
		id = *n.UnderscoreId
	}
	if id == "" {
		return nil, errMissingID("networkconf")
	}
	return s.LegacyCollection.Update(ctx, id, n)
}
