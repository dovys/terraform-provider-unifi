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
	//
	// Forward is the port's VLAN forwarding mode: "all" = trunk (every VLAN),
	// "native" = access port (untagged native network only), "customize" = trunk
	// of native + a chosen tagged set (refined by TaggedVLANMgmt), "disabled" =
	// port administratively shut. TaggedVLANMgmt picks which tagged VLANs trunk:
	// "auto" (allow all), "block_all" (access — native only), "custom" (allow all
	// except ExcludedNetworkConfIDs). VoiceNetworkConfID is a second tagged VLAN
	// for VoIP phones (advertised via LLDP-MED).
	Forward                string   `json:"forward,omitempty"`               // all | native | customize | disabled
	NativeNetworkConfID    string   `json:"native_networkconf_id,omitempty"` // untagged/native network (networkconf _id)
	TaggedVLANMgmt         string   `json:"tagged_vlan_mgmt,omitempty"`      // auto (allow all) | block_all (access) | custom
	ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids"`        // VLANs to block on the trunk when TaggedVLANMgmt=custom
	VoiceNetworkConfID     string   `json:"voice_networkconf_id,omitempty"`  // voice VLAN (networkconf _id)

	// Power over Ethernet. "auto" negotiates active PoE (802.3af/at/bt) per device;
	// "pasv24" forces always-on passive 24V (legacy gear, supporting switches only);
	// "passthrough" re-outputs the switch's input PoE; "off" cuts power. The set of
	// accepted values depends on the switch model.
	POEMode string `json:"poe_mode,omitempty"` // auto | pasv24 | passthrough | off

	// Link. When Autoneg is false the port uses the forced Speed/FullDuplex. OpMode
	// is the port's operation: "switch" (normal), "mirror" (SPAN for capture), or
	// "aggregate" (LACP across ports).
	Autoneg    *bool  `json:"autoneg,omitempty"`     // auto-negotiate speed/duplex
	Speed      *int   `json:"speed,omitempty"`       // forced link speed (Mbps) when autoneg off
	FullDuplex *bool  `json:"full_duplex,omitempty"` // forced full duplex when autoneg off
	OpMode     string `json:"op_mode,omitempty"`     // switch | mirror | aggregate

	// Security / control. PortSecurityMACAddress is owned (non-omitempty), same as
	// ExcludedNetworkConfIDs, so an explicit empty allow-list round-trips.
	//
	// PortSecurityEnabled locks the port to the PortSecurityMACAddress allow-list.
	// Dot1XCtrl is 802.1X port access control: "force_authorized" (always open,
	// default), "auto" (require RADIUS), "force_unauthorized" (always blocked),
	// "mac_based" (per-MAC auth for non-802.1X clients), "multi_host" (auth once,
	// then allow others — e.g. PCs behind a VoIP phone). Isolation blocks traffic
	// between isolated ports (untrusted/guest/IoT use). EgressRateLimit* caps
	// outbound throughput.
	PortSecurityEnabled    *bool    `json:"port_security_enabled,omitempty"`
	PortSecurityMACAddress []string `json:"port_security_mac_address"`                // allowed MACs when PortSecurityEnabled
	Dot1XCtrl              string   `json:"dot1x_ctrl,omitempty"`                     // auto | force_authorized | force_unauthorized | mac_based | multi_host
	Dot1XIdleTimeout       *int     `json:"dot1x_idle_timeout,omitempty"`             // seconds before an authed 802.1X session is cleared
	Isolation              *bool    `json:"isolation,omitempty"`                      // isolate from other isolated ports
	EgressRateLimitEnabled *bool    `json:"egress_rate_limit_kbps_enabled,omitempty"` // attr name drops the `_kbps_` infix
	EgressRateLimitKbps    *int     `json:"egress_rate_limit_kbps,omitempty"`         // egress cap in kbps when enabled

	// Spanning tree (types confirmed against live port_overrides: stp_port_mode is
	// a bool, stp_edge_state is a string enum). STPBPDUGuard shuts the port if it
	// receives a BPDU (rogue-switch/loop protection on an edge port); STPEdgeState
	// is PortFast-style edge behavior (forward immediately, skip listening/learning).
	STPPortMode  *bool  `json:"stp_port_mode,omitempty"`          // participate in STP/RSTP
	STPBPDUGuard *bool  `json:"stp_bpdu_guard_enabled,omitempty"` // shut the port on a received BPDU
	STPEdgeState string `json:"stp_edge_state,omitempty"`         // auto | enabled | disabled (edge/PortFast)

	LLDPMEDEnabled      *bool  `json:"lldpmed_enabled,omitempty"`        // advertise LLDP-MED (VoIP voice-VLAN/QoS auto-config)
	PortKeepaliveOn     *bool  `json:"port_keepalive_enabled,omitempty"` // link keep-alives
	MulticastRouterMode string `json:"multicast_router_mode,omitempty"`  // multicast-router (IGMP/MLD querier) handling
}

// Port profile CRUD (legacy /rest/portconf; router OpPortProfile → classic
// backend). Update is GET-merge-PUT so controller-managed fields survive.
//
// On why legacy and not v2: portconf predates the v2 API and has no v2 endpoint
// on current firmware. Probing a live console (read-only) for the obvious v2
// names — v2/.../port-configs, /portconf, /port-profiles, /switch/port-profiles,
// /port-overrides, /profiles, … — every one 404s, while /rest/portconf returns
// the documents. The v2 surface covers only the newer feature areas (clients,
// traffic, zone firewall, ap-groups, static-dns); port config stays on /rest.
// Recheck on future firmware: if a v2 write lands, it's a one-line router change.

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
