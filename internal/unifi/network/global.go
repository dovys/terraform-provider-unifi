package network

import (
	"context"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// GlobalNetworkService is the global network-config group: the site-level
// singleton at v2 /global/config/network. It carries the site's L2/L3 multicast
// knobs (IGMP snooping, multicast flooding) and the mDNS reflector scope
// (mdns_enabled_for + mdns_enabled_for_network_ids).
//
// Unlike the legacy /rest surface (a server-side shallow merge), this v2 PUT is
// REPLACE-style: the controller revalidates the whole DTO and several fields are
// required (igmp_snooping_for[_network_ids], mdns_enabled_for[_network_ids]). A
// caller that owns only some fields therefore Gets the current document, overlays
// its fields, and Sets the whole thing back — never a sparse PUT.
type GlobalNetworkService struct {
	core *core
}

// GlobalNetwork returns the global network-config group (v2 singleton).
func (c *Client) GlobalNetwork() GlobalNetworkService {
	return GlobalNetworkService{core: c.core}
}

// globalNetworkPath is the v2 singleton path (no per-object id).
const globalNetworkPath = "global/config/network"

// Get returns the site's global network-config document, or ErrNotFound when the
// surface is absent (older firmware without the ucore/fabric global config).
func (s GlobalNetworkService) Get(ctx context.Context) (*wire.Global, error) {
	var out wire.Global
	if err := v2Decode(ctx, s.core, http.MethodGet, globalNetworkPath, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Set PUTs the full global network-config document (replace-style; the v2
// controller revalidates the whole DTO). Callers pass a document they obtained
// from Get and mutated, so the required IGMP/mDNS fields stay populated.
func (s GlobalNetworkService) Set(ctx context.Context, doc *wire.Global) (*wire.Global, error) {
	var out wire.Global
	if err := v2Decode(ctx, s.core, http.MethodPut, globalNetworkPath, doc, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
