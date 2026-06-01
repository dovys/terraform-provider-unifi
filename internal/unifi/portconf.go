package unifi

import (
	"context"
	"fmt"
)

const portConfType = "portconf"

// PortProfile is a legacy `/rest/portconf` document — a reusable switch-port
// configuration (VLAN behavior, PoE, port security, STP, 802.1X, rate limits)
// applied to switch ports by `port_idx`. Field names mirror the controller wire
// shape, which is the same field set the per-port `port_overrides` array uses
// (confirmed against a live UCG Max). Pointers + omitempty give three-state
// semantics on GET-merge-PUT.
//
// NOTE: the recon site had no custom port profiles, so the accepted enum values
// for forward/poe_mode/dot1x are modeled from documented behavior and tracked in
// TODO.md for hardware verification.
type PortProfile struct {
	ID     string `json:"_id,omitempty"`     // controller object ID; the resource import key
	SiteID string `json:"site_id,omitempty"` // owning site (controller-assigned)
	Name   string `json:"name"`              // profile name

	// VLAN behavior. ExcludedNetworkConfIDs is an owned list (non-omitempty): the
	// resource always sends it so an explicit empty set clears the exclusions
	// rather than reading back null (mirrors FirewallGroup.GroupMembers).
	Forward                string   `json:"forward,omitempty"`               // all | native | customize | disabled
	NativeNetworkConfID    string   `json:"native_networkconf_id,omitempty"` // untagged/native network (networkconf _id)
	TaggedVLANMgmt         string   `json:"tagged_vlan_mgmt,omitempty"`      // auto | block_all | custom
	ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids"`        // tagged networks to exclude (when customize)
	VoiceNetworkConfID     string   `json:"voice_networkconf_id,omitempty"`  // voice VLAN

	// Power over Ethernet.
	POEMode string `json:"poe_mode,omitempty"` // auto | pasv24 | passthrough | off

	// Link.
	Autoneg    *bool  `json:"autoneg,omitempty"`     // auto-negotiate speed/duplex
	Speed      *int   `json:"speed,omitempty"`       // forced link speed (Mbps) when autoneg off
	FullDuplex *bool  `json:"full_duplex,omitempty"` // forced duplex when autoneg off
	OpMode     string `json:"op_mode,omitempty"`     // switch | mirror | aggregate

	// Security / control. PortSecurityMACAddress is owned (non-omitempty), same as
	// ExcludedNetworkConfIDs, so an explicit empty allow-list round-trips.
	PortSecurityEnabled    *bool    `json:"port_security_enabled,omitempty"`
	PortSecurityMACAddress []string `json:"port_security_mac_address"`
	Dot1XCtrl              string   `json:"dot1x_ctrl,omitempty"` // auto | force_authorized | force_unauthorized | mac_based | multi_host
	Dot1XIdleTimeout       *int     `json:"dot1x_idle_timeout,omitempty"`
	Isolation              *bool    `json:"isolation,omitempty"`                      // port isolation
	EgressRateLimitEnabled *bool    `json:"egress_rate_limit_kbps_enabled,omitempty"` // attr name drops the `_kbps_` infix
	EgressRateLimitKbps    *int     `json:"egress_rate_limit_kbps,omitempty"`

	// Spanning tree (types confirmed against live port_overrides: stp_port_mode is
	// a bool, stp_edge_state is a string enum).
	STPPortMode  *bool  `json:"stp_port_mode,omitempty"` // participate in STP
	STPBPDUGuard *bool  `json:"stp_bpdu_guard_enabled,omitempty"`
	STPEdgeState string `json:"stp_edge_state,omitempty"` // auto | enabled | disabled

	LLDPMEDEnabled      *bool  `json:"lldpmed_enabled,omitempty"`
	PortKeepaliveOn     *bool  `json:"port_keepalive_enabled,omitempty"`
	MulticastRouterMode string `json:"multicast_router_mode,omitempty"`
}

// Port profile CRUD (legacy /rest/portconf; router OpPortProfile → classic
// backend). Update is GET-merge-PUT so controller-managed fields survive.

func (c *client) ListPortProfiles(ctx context.Context) ([]PortProfile, error) {
	return legacyList[PortProfile](ctx, c, portConfType)
}

func (c *client) GetPortProfile(ctx context.Context, id string) (*PortProfile, error) {
	return legacyGet[PortProfile](ctx, c, portConfType, id)
}

func (c *client) CreatePortProfile(ctx context.Context, p *PortProfile) (*PortProfile, error) {
	return legacyCreate[PortProfile](ctx, c, portConfType, p)
}

func (c *client) UpdatePortProfile(ctx context.Context, p *PortProfile) (*PortProfile, error) {
	if p.ID == "" {
		return nil, fmt.Errorf("UpdatePortProfile: missing id")
	}
	return legacyMergePut[PortProfile](ctx, c, portConfType, p.ID, p)
}

func (c *client) DeletePortProfile(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, portConfType, id)
}
