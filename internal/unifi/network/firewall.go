package network

import "github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"

// FirewallService is the firewall group. Today it owns the classic firewall
// (legacy /rest/firewallgroup + /rest/firewallrule) — the surface that is
// actually enabled on the live controller. The v2 zone-based firewall
// (policies/zones/NAT/ACL) is modeled as gated client methods in a later phase;
// the classic-vs-zone capability gate is decided at plan time, never mixed.
type FirewallService struct{ core *core }

// Firewall returns the firewall group.
func (c *Client) Firewall() FirewallService { return FirewallService{core: c.core} }

// Groups is the firewall-group (address/port group) collection
// (legacy /rest/firewallgroup).
func (s FirewallService) Groups() LegacyCollection[wire.Firewallgroup] {
	return LegacyCollection[wire.Firewallgroup]{core: s.core, typ: "firewallgroup"}
}

// Rules is the classic firewall-rule collection (legacy /rest/firewallrule).
func (s FirewallService) Rules() LegacyCollection[wire.Firewallrule] {
	return LegacyCollection[wire.Firewallrule]{core: s.core, typ: "firewallrule"}
}

// PortForwards is the port-forward (DNAT) collection (legacy /rest/portforward).
func (c *Client) PortForwards() LegacyCollection[wire.Portforward] {
	return LegacyCollection[wire.Portforward]{core: c.core, typ: "portforward"}
}
