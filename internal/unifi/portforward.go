package unifi

import (
	"context"
	"fmt"
)

const portForwardType = "portforward"

// PortForward is a legacy `/rest/portforward` document — a destination-NAT rule
// that forwards a WAN port to an internal host. Modeled from documented
// controller behavior — the field set is HIL-unverified (this network has no
// port-forward rules) — see TODO.md. Pointers + omitempty give three-state
// GET-merge-PUT semantics.
type PortForward struct {
	ID            string `json:"_id,omitempty"`
	SiteID        string `json:"site_id,omitempty"`
	Name          string `json:"name"`
	Enabled       *bool  `json:"enabled,omitempty"`
	PfwdInterface string `json:"pfwd_interface,omitempty"` // wan | wan2 | both
	Proto         string `json:"proto,omitempty"`          // tcp_udp | tcp | udp
	Src           string `json:"src,omitempty"`            // source filter: "any" or CIDR/IP
	DstPort       string `json:"dst_port,omitempty"`       // external port(s), e.g. "443" or "8000-8010"
	Fwd           string `json:"fwd,omitempty"`            // internal host IP to forward to
	FwdPort       string `json:"fwd_port,omitempty"`       // internal port(s)
	Log           *bool  `json:"log,omitempty"`
}

// Port-forward CRUD (legacy /rest/portforward). Update is GET-merge-PUT so
// controller-managed fields survive.

func (c *client) ListPortForwards(ctx context.Context) ([]PortForward, error) {
	return legacyList[PortForward](ctx, c, portForwardType)
}

func (c *client) GetPortForward(ctx context.Context, id string) (*PortForward, error) {
	return legacyGet[PortForward](ctx, c, portForwardType, id)
}

func (c *client) CreatePortForward(ctx context.Context, p *PortForward) (*PortForward, error) {
	return legacyCreate[PortForward](ctx, c, portForwardType, p)
}

func (c *client) UpdatePortForward(ctx context.Context, p *PortForward) (*PortForward, error) {
	if p.ID == "" {
		return nil, fmt.Errorf("UpdatePortForward: missing id")
	}
	return legacyMergePut[PortForward](ctx, c, portForwardType, p.ID, p)
}

func (c *client) DeletePortForward(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, portForwardType, id)
}
