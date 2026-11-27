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

// ZoneBasedFirewall reports whether the site runs the zone-based firewall:
// either it recorded the migration, or it lists zones (a site created
// zone-based records no migration). There, legacy /rest/firewallrule lists
// nothing. A 404 from both probes means classic; any other probe error is
// returned with false, for the caller to decide.
func (c *Client) ZoneBasedFirewall(ctx context.Context) (bool, error) {
	var migrations []wire.SiteFeatureMigration
	mErr := v2Decode(ctx, c.core, http.MethodGet, "site-feature-migration", nil, &migrations)
	for _, m := range migrations {
		if m.Feature != nil && *m.Feature == wire.ZONEBASEDFIREWALL {
			return true, nil
		}
	}
	zones, zErr := c.Firewall().Zones().List(ctx)
	if zErr == nil && len(zones) > 0 {
		return true, nil
	}
	for _, err := range []error{mErr, zErr} {
		if err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	return false, nil
}

// Zone returns the one zone matching every non-empty key.
func (s FirewallService) Zone(ctx context.Context, zoneKey, name string) (*wire.FirewallZone, error) {
	zones, err := s.Zones().List(ctx)
	if err != nil {
		return nil, err
	}
	return findOne(zones, func(z *wire.FirewallZone) bool {
		return (zoneKey == "" || deref(z.ZoneKey) == zoneKey) && (name == "" || deref(z.Name) == name)
	})
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
// are absent from the spec. Tags follow the live document. Replace with
// generated types once the spec has the subtypes (#7).
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
// model them, and the generated schedule type decodes dates strictly (#7).
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

// endpointMatchKeys are the endpoint's match operands. Update owns them
// outright: one absent from the body is removed from the document, so a mode
// change (ips to ip_group_id, port to ANY) leaves no stale operand behind.
var endpointMatchKeys = []string{"ips", "network_ids", "ip_group_id", "port", "port_group_id"}

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
	return decodePolicy(raw)
}

// GetByName returns the only policy with the given name; names aren't unique
// on the controller, so a duplicate is ErrAmbiguous.
func (s PolicyCollection) GetByName(ctx context.Context, name string) (*FirewallPolicy, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findOne(all, func(p *FirewallPolicy) bool { return p.Name == name })
}

func (s PolicyCollection) getRaw(ctx context.Context, id string) (json.RawMessage, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	return s.do(ctx, http.MethodGet, s.path+"/"+id, nil)
}

// do sends a single-policy request and returns the one document in the reply.
// The decompiled routes declare a bare object, but a one-element array is
// accepted too; null, empty or [] reads as ErrNotFound.
func (s PolicyCollection) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := v2Decode(ctx, s.core, method, path, body, &raw); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, ErrNotFound
		}
		trimmed = bytes.TrimSpace(items[0])
	}
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, ErrNotFound
	}
	return trimmed, nil
}

func decodePolicy(raw json.RawMessage) (*FirewallPolicy, error) {
	var p FirewallPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode firewall policy: %w", err)
	}
	return &p, nil
}

// Create POSTs a new policy. The body is the full DTO, so p must carry every
// field the controller requires (schedule, port_matching_type, ip_version).
func (s PolicyCollection) Create(ctx context.Context, p *FirewallPolicy) (*FirewallPolicy, error) {
	raw, err := s.do(ctx, http.MethodPost, s.path, p)
	if errors.Is(err, ErrNotFound) {
		err = fmt.Errorf("create firewall policy %q returned no policy", p.Name)
	}
	if err != nil {
		return nil, err
	}
	out, err := decodePolicy(raw)
	if err != nil {
		return nil, err
	}
	if out.UnderscoreId == nil || *out.UnderscoreId == "" {
		return nil, fmt.Errorf("create firewall policy %q returned no _id", p.Name)
	}
	return out, nil
}

// Update overlays p's owned fields onto the live document and PUTs it back by
// id. Endpoints merge key by key, so unmodeled endpoint keys survive. Outside
// endpointMatchKeys, a nil or empty value in p keeps the live value. An empty
// matching_target_type on an ANY endpoint is removed, since ANY takes no type.
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
		merged, err := mergeEndpoint(doc[k], owned[k])
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

	raw, err := s.do(ctx, http.MethodPut, s.path+"/"+id, doc)
	if errors.Is(err, ErrNotFound) {
		return s.Get(ctx, id) // empty echo
	}
	if err != nil {
		return nil, err
	}
	out, err := decodePolicy(raw)
	if err != nil {
		return nil, err
	}
	if out.UnderscoreId == nil {
		return s.Get(ctx, id)
	}
	return out, nil
}

// mergeEndpoint overlays the owned endpoint over onto the live base.
func mergeEndpoint(base, over json.RawMessage) (json.RawMessage, error) {
	m := map[string]json.RawMessage{}
	if !emptyRaw(base) {
		if err := json.Unmarshal(base, &m); err != nil {
			return nil, err
		}
	}
	o := map[string]json.RawMessage{}
	if !emptyRaw(over) {
		if err := json.Unmarshal(over, &o); err != nil {
			return nil, err
		}
	}
	for _, k := range endpointMatchKeys {
		delete(m, k)
	}
	for k, v := range o {
		if !emptyRaw(v) {
			m[k] = v
		}
	}
	if string(m["matching_target"]) == `"ANY"` && emptyRaw(o["matching_target_type"]) {
		delete(m, "matching_target_type")
	}
	return json.Marshal(m)
}

// emptyRaw reports a JSON null or "" — an unset value that must not clobber
// the live one in a merge.
func emptyRaw(v json.RawMessage) bool {
	t := bytes.TrimSpace(v)
	return len(t) == 0 || bytes.Equal(t, []byte("null")) || bytes.Equal(t, []byte(`""`))
}
