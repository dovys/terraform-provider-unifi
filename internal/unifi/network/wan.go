package network

import (
	"context"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// WANService is the WAN group: the WAN Magic managed-WAN service, failover SLAs,
// load balancing, and the per-WAN network groups. All on the v2 /wan/* surface.
// The WAN interface networks themselves are networkconf objects, managed via
// Networks(); this group is the WAN-specific behavior layered on top.
type WANService struct{ core *core }

// WAN returns the WAN group.
func (c *Client) WAN() WANService { return WANService{core: c.core} }

// MagicPause pauses the WAN Magic service (PUT /wan/magic/pause).
func (s WANService) MagicPause(ctx context.Context) error {
	return v2Decode[any](ctx, s.core, http.MethodPut, "wan/magic/pause", nil, nil)
}

// MagicResume resumes the WAN Magic service (PUT /wan/magic/resume).
func (s WANService) MagicResume(ctx context.Context) error {
	return v2Decode[any](ctx, s.core, http.MethodPut, "wan/magic/resume", nil, nil)
}

// MagicConfig returns the current WAN Magic configuration.
func (s WANService) MagicConfig(ctx context.Context) (*wire.WanMagicConfiguration, error) {
	var out wire.WanMagicConfiguration
	if err := v2Decode(ctx, s.core, http.MethodGet, "wan/magic/configuration", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetMagicConfig updates the WAN Magic configuration.
func (s WANService) SetMagicConfig(ctx context.Context, cfg *wire.WanMagicConfiguration) (*wire.WanMagicConfiguration, error) {
	var out wire.WanMagicConfiguration
	if err := v2Decode(ctx, s.core, http.MethodPut, "wan/magic/configuration", cfg, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListNetworkGroups returns the site's WAN network groups (WAN, WAN2, …) with
// their load-balancing mode, priority, and bound network. The v2 surface wraps
// the list in a {wan_network_groups:[…]} object; this unwraps it.
func (s WANService) ListNetworkGroups(ctx context.Context) ([]wire.WanNetworkgroupsWanNetworkGroup, error) {
	var out wire.WanNetworkgroups
	if err := v2Decode(ctx, s.core, http.MethodGet, "wan/networkgroups", nil, &out); err != nil {
		return nil, err
	}
	if out.WanNetworkGroups == nil {
		return nil, nil
	}
	return *out.WanNetworkGroups, nil
}

// LoadBalancing returns the WAN load-balancing configuration (mode + per-WAN
// interface weights).
func (s WANService) LoadBalancing(ctx context.Context) (*wire.Wan, error) {
	var out wire.Wan
	if err := v2Decode(ctx, s.core, http.MethodGet, "wan/load-balancing/configuration", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetLoadBalancing updates the WAN load-balancing configuration.
func (s WANService) SetLoadBalancing(ctx context.Context, cfg *wire.Wan) (*wire.Wan, error) {
	var out wire.Wan
	if err := v2Decode(ctx, s.core, http.MethodPut, "wan/load-balancing/configuration", cfg, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SLAs is the WAN failover SLA (health-monitor) collection (v2 /wan-slas).
func (s WANService) SLAs() SLACollection {
	return SLACollection{V2Collection[wire.WanSlas]{core: s.core, path: "wan-slas"}}
}

// SLACollection is the WAN SLA CRUD collection. It overrides Update to send the
// SLA's _id in both the URL and the body (the v2 write contract).
type SLACollection struct {
	V2Collection[wire.WanSlas]
}

// Update PUTs the SLA, sending its _id in both URL and body. _id must be set.
func (s SLACollection) Update(ctx context.Context, sla *wire.WanSlas) (*wire.WanSlas, error) {
	if sla.UnderscoreId == nil || *sla.UnderscoreId == "" {
		return nil, errMissingID("wan-sla")
	}
	return s.V2Collection.Update(ctx, *sla.UnderscoreId, sla)
}
