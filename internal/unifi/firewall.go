package unifi

import (
	"context"
	"fmt"
)

const (
	firewallGroupType = "firewallgroup"
	firewallRuleType  = "firewallrule"
)

// FirewallGroup is a legacy `/rest/firewallgroup` document — a reusable set of
// addresses or ports referenced by firewall rules. group_members is
// non-omitempty so emptying it actually clears the group (an unset slice would
// otherwise be dropped from the GET-merge-PUT overlay).
type FirewallGroup struct {
	ID           string   `json:"_id,omitempty"`     // controller object ID; the resource import key
	SiteID       string   `json:"site_id,omitempty"` // owning site (controller-assigned)
	Name         string   `json:"name"`              // display name
	GroupType    string   `json:"group_type"`        // address-group | port-group | ipv6-address-group
	GroupMembers []string `json:"group_members"`     // IPs/CIDRs (address) or ports (port-group)
}

// FirewallRule is a legacy `/rest/firewallrule` document — one classic
// (pre-zone) firewall rule. Field names mirror the controller wire shape
// (confirmed against a live UCG Max). Pointers + omitempty give three-state
// semantics on GET-merge-PUT: nil = leave the controller's value, set = write it
// (including an explicit false). MAC-address matching and the IPv6-specific
// address fields are not modeled yet; they're carried through untouched by
// GET-merge-PUT. The newer zone-based firewall is a separate surface (P3).
type FirewallRule struct {
	ID                string `json:"_id,omitempty"`                     // controller object ID; the resource import key
	SiteID            string `json:"site_id,omitempty"`                 // owning site (controller-assigned)
	Name              string `json:"name"`                              // rule name
	Enabled           *bool  `json:"enabled,omitempty"`                 // whether the rule is active
	Ruleset           string `json:"ruleset"`                           // chain: LAN_IN/OUT/LOCAL, WAN_*, GUEST_*, and v6 variants
	RuleIndex         *int   `json:"rule_index,omitempty"`              // evaluation order within the ruleset
	Action            string `json:"action"`                            // accept | drop | reject
	Protocol          string `json:"protocol,omitempty"`                // all | tcp | udp | tcp_udp | icmp | <number> | …
	ProtocolV6        string `json:"protocol_v6,omitempty"`             // IPv6 protocol selector (for v6 rulesets)
	ProtocolExcepted  *bool  `json:"protocol_match_excepted,omitempty"` // match every protocol EXCEPT the one above
	ICMPTypename      string `json:"icmp_typename,omitempty"`           // ICMP type to match (when protocol=icmp)
	Logging           *bool  `json:"logging,omitempty"`                 // log packets matching this rule
	IPSec             string `json:"ipsec,omitempty"`                   // match-ipsec | match-none
	SettingPreference string `json:"setting_preference,omitempty"`      // auto | manual controller toggle (read-only here)

	// Connection-state matching.
	StateNew         *bool `json:"state_new,omitempty"`
	StateEstablished *bool `json:"state_established,omitempty"`
	StateRelated     *bool `json:"state_related,omitempty"`
	StateInvalid     *bool `json:"state_invalid,omitempty"`

	// Source match.
	SrcAddress          string   `json:"src_address,omitempty"`           // IP/CIDR
	SrcPort             string   `json:"src_port,omitempty"`              // port or range
	SrcNetworkConfID    string   `json:"src_networkconf_id,omitempty"`    // match a network (networkconf _id)
	SrcNetworkConfType  string   `json:"src_networkconf_type,omitempty"`  // NETv4 | ADDRv4 | …
	SrcFirewallGroupIDs []string `json:"src_firewallgroup_ids,omitempty"` // referenced address/port groups

	// Destination match.
	DstAddress          string   `json:"dst_address,omitempty"`
	DstPort             string   `json:"dst_port,omitempty"`
	DstNetworkConfID    string   `json:"dst_networkconf_id,omitempty"`
	DstNetworkConfType  string   `json:"dst_networkconf_type,omitempty"`
	DstFirewallGroupIDs []string `json:"dst_firewallgroup_ids,omitempty"`
}

// Firewall group CRUD (legacy /rest/firewallgroup; router OpFirewallClassic).
// Update is GET-merge-PUT so controller-managed fields survive.

func (c *client) ListFirewallGroups(ctx context.Context) ([]FirewallGroup, error) {
	return legacyList[FirewallGroup](ctx, c, firewallGroupType)
}

func (c *client) GetFirewallGroup(ctx context.Context, id string) (*FirewallGroup, error) {
	return legacyGet[FirewallGroup](ctx, c, firewallGroupType, id)
}

func (c *client) CreateFirewallGroup(ctx context.Context, g *FirewallGroup) (*FirewallGroup, error) {
	return legacyCreate[FirewallGroup](ctx, c, firewallGroupType, g)
}

func (c *client) UpdateFirewallGroup(ctx context.Context, g *FirewallGroup) (*FirewallGroup, error) {
	if g.ID == "" {
		return nil, fmt.Errorf("UpdateFirewallGroup: missing id")
	}
	return legacyMergePut[FirewallGroup](ctx, c, firewallGroupType, g.ID, g)
}

func (c *client) DeleteFirewallGroup(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, firewallGroupType, id)
}

// Firewall rule CRUD (legacy /rest/firewallrule; router OpFirewallClassic).

func (c *client) ListFirewallRules(ctx context.Context) ([]FirewallRule, error) {
	return legacyList[FirewallRule](ctx, c, firewallRuleType)
}

func (c *client) GetFirewallRule(ctx context.Context, id string) (*FirewallRule, error) {
	return legacyGet[FirewallRule](ctx, c, firewallRuleType, id)
}

func (c *client) CreateFirewallRule(ctx context.Context, r *FirewallRule) (*FirewallRule, error) {
	return legacyCreate[FirewallRule](ctx, c, firewallRuleType, r)
}

func (c *client) UpdateFirewallRule(ctx context.Context, r *FirewallRule) (*FirewallRule, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateFirewallRule: missing id")
	}
	return legacyMergePut[FirewallRule](ctx, c, firewallRuleType, r.ID, r)
}

func (c *client) DeleteFirewallRule(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, firewallRuleType, id)
}
