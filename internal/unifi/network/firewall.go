package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// FirewallService is the firewall group: the classic firewall (legacy
// /rest/firewallgroup, /rest/firewallrule) and the zone-based firewall (v2
// firewall-policies, firewall/zone). Firewall groups serve both.
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

// ZoneBasedFirewall reports whether the site has migrated to the zone-based
// firewall. After the migration legacy /rest/firewallrule lists nothing and
// rules live as v2 firewall policies. Firmware without the surface (404) is
// classic.
func (c *Client) ZoneBasedFirewall(ctx context.Context) (bool, error) {
	var migrations []wire.SiteFeatureMigration
	err := v2Decode(ctx, c.core, http.MethodGet, "site-feature-migration", nil, &migrations)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, m := range migrations {
		if m.Feature != nil && *m.Feature == wire.ZONEBASEDFIREWALL {
			return true, nil
		}
	}
	return false, nil
}

// Zones is the zone-based firewall's zone list (v2 /firewall/zone).
func (s FirewallService) Zones() V2Collection[wire.FirewallZone] {
	return V2Collection[wire.FirewallZone]{core: s.core, path: "firewall/zone"}
}

// Policies is the zone-based firewall policy collection (v2 /firewall-policies).
func (s FirewallService) Policies() PolicyCollection {
	return PolicyCollection{V2Collection[FirewallPolicy]{core: s.core, path: policiesPath}}
}

// PolicyEndpoint is a policy's source or destination. The generated
// wire.FirewallPoliciesSource/Destination model only the decompiled base DTO;
// the controller deserializes polymorphically on matching_target, and the
// subclass fields (ips, network_ids, ip_group_id, match_mac, match_opposite_*)
// are absent from the spec. Tags follow the live document.
type PolicyEndpoint struct {
	ZoneID                string   `json:"zone_id"`
	MatchingTarget        string   `json:"matching_target"`
	MatchingTargetType    string   `json:"matching_target_type,omitempty"`
	IPs                   []string `json:"ips,omitempty"`
	NetworkIDs            []string `json:"network_ids,omitempty"`
	IPGroupID             string   `json:"ip_group_id,omitempty"`
	Port                  string   `json:"port,omitempty"`
	PortGroupID           string   `json:"port_group_id,omitempty"`
	PortMatchingType      string   `json:"port_matching_type,omitempty"`
	MatchOppositeIPs      *bool    `json:"match_opposite_ips,omitempty"`
	MatchOppositeNetworks *bool    `json:"match_opposite_networks,omitempty"`
	MatchOppositePorts    *bool    `json:"match_opposite_ports,omitempty"`
	MatchMAC              *bool    `json:"match_mac,omitempty"` // source only
}

// FirewallPolicy is wire.FirewallPolicies with the endpoints replaced by
// PolicyEndpoint. schedule and cloud_template stay raw: the provider doesn't
// model them, and the generated schedule type decodes dates strictly.
type FirewallPolicy struct {
	wire.FirewallPolicies
	Source        PolicyEndpoint  `json:"source"`
	Destination   PolicyEndpoint  `json:"destination"`
	Schedule      json.RawMessage `json:"schedule,omitempty"`
	CloudTemplate json.RawMessage `json:"cloud_template,omitempty"`
}

// policyOwnedKeys are the top-level keys Update overlays onto the live
// document. index (ordering), predefined, schedule and everything unmodeled
// keep the controller's value.
var policyOwnedKeys = []string{
	"name", "description", "action", "enabled", "protocol", "ip_version",
	"connection_state_type", "connection_states", "create_allow_respond",
	"logging", "match_ip_sec", "match_opposite_protocol",
}

// policyCounterKeys are read-only hit counters, dropped from the PUT body.
var policyCounterKeys = []string{"hits", "last_hit"}

// PolicyCollection is the firewall policy CRUD collection. The v2 PUT
// replaces the whole DTO, so Update GET-merges instead of sending a sparse body.
type PolicyCollection struct {
	V2Collection[FirewallPolicy]
}

const policiesPath = "firewall-policies"

// Get returns the policy with the given _id, or ErrNotFound.
func (s PolicyCollection) Get(ctx context.Context, id string) (*FirewallPolicy, error) {
	raw, err := s.getRaw(ctx, id)
	if err != nil {
		return nil, err
	}
	var p FirewallPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode firewall policy %s: %w", id, err)
	}
	return &p, nil
}

// GetByName returns the only policy with the given name. Names aren't unique
// on the controller, so a duplicate is an error rather than a guess.
func (s PolicyCollection) GetByName(ctx context.Context, name string) (*FirewallPolicy, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	var found *FirewallPolicy
	for i := range all {
		if all[i].Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("several firewall policies are named %q; import by _id", name)
		}
		found = &all[i]
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
}

// getRaw GETs one policy document. A one-element array is accepted as well as
// a bare object.
func (s PolicyCollection) getRaw(ctx context.Context, id string) (json.RawMessage, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	var raw json.RawMessage
	if err := v2Decode(ctx, s.core, http.MethodGet, s.path+"/"+id, nil, &raw); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, ErrNotFound
	}
	if trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, ErrNotFound
		}
		return items[0], nil
	}
	return trimmed, nil
}

// Create POSTs a new policy. The body is the full DTO, so p must carry every
// field the controller requires (schedule, port_matching_type, ip_version).
func (s PolicyCollection) Create(ctx context.Context, p *FirewallPolicy) (*FirewallPolicy, error) {
	out, err := s.V2Collection.Create(ctx, p)
	if err != nil {
		return nil, err
	}
	if out.UnderscoreId == nil || *out.UnderscoreId == "" {
		return nil, fmt.Errorf("create firewall policy %q returned no _id", p.Name)
	}
	return out, nil
}

// Update overlays p's owned fields onto the live document and PUTs it back by
// id. Endpoints merge key by key, so unmodeled endpoint keys survive too. A
// nil, empty-string or omitted value in p leaves the live value alone.
func (s PolicyCollection) Update(ctx context.Context, id string, p *FirewallPolicy) (*FirewallPolicy, error) {
	if id == "" {
		return nil, errMissingID("firewall policy")
	}
	live, err := s.getRaw(ctx, id)
	if err != nil {
		return nil, err
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal(live, &doc); err != nil {
		return nil, fmt.Errorf("decode firewall policy %s: %w", id, err)
	}
	owned, err := structToRawMap(p)
	if err != nil {
		return nil, err
	}
	for _, k := range policyOwnedKeys {
		if v, ok := owned[k]; ok && !emptyRaw(v) {
			doc[k] = v
		}
	}
	for _, k := range []string{"source", "destination"} {
		merged, err := mergeRawObject(doc[k], owned[k])
		if err != nil {
			return nil, fmt.Errorf("merge firewall policy %s: %w", k, err)
		}
		doc[k] = merged
	}
	for _, k := range policyCounterKeys {
		delete(doc, k)
	}
	idJSON, _ := json.Marshal(id)
	doc["_id"] = idJSON

	var out FirewallPolicy
	if err := v2Decode(ctx, s.core, http.MethodPut, s.path+"/"+id, doc, &out); err != nil {
		return nil, err
	}
	if out.UnderscoreId == nil {
		return s.Get(ctx, id) // empty echo
	}
	return &out, nil
}

// mergeRawObject overlays the keys of over onto base (both JSON objects).
func mergeRawObject(base, over json.RawMessage) (json.RawMessage, error) {
	m := map[string]json.RawMessage{}
	if len(base) > 0 && !bytes.Equal(bytes.TrimSpace(base), []byte("null")) {
		if err := json.Unmarshal(base, &m); err != nil {
			return nil, err
		}
	}
	if len(over) > 0 {
		o := map[string]json.RawMessage{}
		if err := json.Unmarshal(over, &o); err != nil {
			return nil, err
		}
		for k, v := range o {
			if !emptyRaw(v) {
				m[k] = v
			}
		}
	}
	return json.Marshal(m)
}

// emptyRaw reports a JSON null or "" — an unset value that must not clobber
// the live one in a merge.
func emptyRaw(v json.RawMessage) bool {
	t := bytes.TrimSpace(v)
	return len(t) == 0 || bytes.Equal(t, []byte("null")) || bytes.Equal(t, []byte(`""`))
}
