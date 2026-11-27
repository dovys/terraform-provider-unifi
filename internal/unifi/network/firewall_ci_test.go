package network

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// livePolicyJSON is a user policy in the shape a migrated Network 11 site
// returns, including fields the provider doesn't model.
const livePolicyJSON = `{
  "_id": "6f1c2a9e-0000-4000-8000-000000000001",
  "action": "BLOCK", "connection_state_type": "CUSTOM", "connection_states": ["NEW"],
  "create_allow_respond": false, "enabled": true, "hits": 42, "last_hit": 1763000000,
  "icmp_typename": "ANY", "icmp_v6_typename": "ANY", "index": 10004, "ip_version": "IPV4",
  "logging": false, "match_ip_sec": false, "match_opposite_protocol": false,
  "name": "isolate iot", "origin_type": "custom_firewall_rule", "predefined": false,
  "protocol": "all", "report_flow": true,
  "schedule": {"mode": "EVERY_WEEK", "repeat_on_days": ["mon"], "time_all_day": false},
  "source": {"zone_id": "zone-int", "matching_target": "NETWORK", "matching_target_type": "SPECIFIC",
    "network_ids": ["net-a"], "ips": [], "port": "", "port_matching_type": "ANY",
    "match_opposite_ips": false, "match_opposite_networks": false, "match_opposite_ports": false,
    "match_mac": false, "client_macs": ["unmodeled"]},
  "destination": {"zone_id": "zone-int", "matching_target": "NETWORK", "network_ids": ["net-b"],
    "port_matching_type": "ANY", "match_opposite_ports": false}
}`

func TestFirewallPolicyGet_DecodesSubclassFields(t *testing.T) {
	f := fakeQueue(livePolicyJSON)
	p, err := newTestClient(f).Firewall().Policies().Get(context.Background(), "6f1c2a9e-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if f.method != http.MethodGet || f.path != "/proxy/network/v2/api/site/default/firewall-policies/6f1c2a9e-0000-4000-8000-000000000001" {
		t.Errorf("request = %s %s", f.method, f.path)
	}
	if p.Name != "isolate iot" || p.Source.ZoneID != "zone-int" || len(p.Source.NetworkIDs) != 1 || p.Destination.NetworkIDs[0] != "net-b" {
		t.Errorf("decoded %+v", p)
	}
	if p.Index == nil || *p.Index != 10004 {
		t.Errorf("index = %v", p.Index)
	}
}

func TestFirewallPolicyGet_NotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if _, err := newTestClient(f).Firewall().Policies().Get(context.Background(), "gone"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Update must keep everything it doesn't own: ordering, schedule, unmodeled
// endpoint keys. Counters are dropped.
func TestFirewallPolicyUpdate_MergesOverLiveDocument(t *testing.T) {
	f := fakeQueue(livePolicyJSON, livePolicyJSON)
	p := &FirewallPolicy{}
	p.Name = "isolate iot v2"
	p.Action = "REJECT"
	p.Source = PolicyEndpoint{ZoneID: "zone-int", MatchingTarget: "NETWORK", NetworkIDs: []string{"net-c"}}
	p.Destination = PolicyEndpoint{ZoneID: "zone-ext", MatchingTarget: "IP", IPs: []string{"203.0.113.7"}, Port: "443"}

	id := "6f1c2a9e-0000-4000-8000-000000000001"
	if _, err := newTestClient(f).Firewall().Policies().Update(context.Background(), id, p); err != nil {
		t.Fatal(err)
	}
	if f.method != http.MethodPut || f.path != "/proxy/network/v2/api/site/default/firewall-policies/"+id {
		t.Fatalf("request = %s %s", f.method, f.path)
	}
	b, _ := json.Marshal(f.body)
	var doc map[string]any
	_ = json.Unmarshal(b, &doc)

	want := map[string]any{"_id": id, "name": "isolate iot v2", "action": "REJECT", "index": float64(10004),
		"ip_version": "IPV4", "protocol": "all", "predefined": false, "report_flow": true}
	for k, v := range want {
		if doc[k] != v {
			t.Errorf("%s = %v, want %v", k, doc[k], v)
		}
	}
	for _, k := range []string{"hits", "last_hit"} {
		if _, ok := doc[k]; ok {
			t.Errorf("counter %q sent back", k)
		}
	}
	if sched, _ := json.Marshal(doc["schedule"]); string(sched) != `{"mode":"EVERY_WEEK","repeat_on_days":["mon"],"time_all_day":false}` {
		t.Errorf("schedule = %s", sched)
	}
	src := doc["source"].(map[string]any)
	if src["client_macs"] == nil || src["matching_target_type"] != "SPECIFIC" {
		t.Errorf("unmodeled/unset source keys lost: %v", src)
	}
	if ids := src["network_ids"].([]any); len(ids) != 1 || ids[0] != "net-c" {
		t.Errorf("source.network_ids = %v", ids)
	}
	dst := doc["destination"].(map[string]any)
	if dst["zone_id"] != "zone-ext" || dst["port"] != "443" || dst["matching_target"] != "IP" {
		t.Errorf("destination = %v", dst)
	}
}

func TestZoneBasedFirewall(t *testing.T) {
	cases := map[string]struct {
		f       *fakeDoer
		want    bool
		wantErr bool
	}{
		"migrated":           {f: fakeQueue(`[{"feature":"ZONE_BASED_FIREWALL","timestamp":1763164800000}]`), want: true},
		"created zone-based": {f: fakeQueue(`[]`, `[{"_id":"z1","zone_key":"internal"}]`), want: true},
		"classic":            {f: fakeQueue(`[]`, `[]`)},
		"old firmware (404)": {f: &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}},
		"unexpected 400":     {f: &fakeDoer{err: &transport.APIError{StatusCode: http.StatusBadRequest}}, wantErr: true},
	}
	for name, tc := range cases {
		got, err := newTestClient(tc.f).ZoneBasedFirewall(context.Background())
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("%s: got %v, %v; want %v, err=%v", name, got, err, tc.want, tc.wantErr)
		}
	}
}

// A mode change must not leave the old mode's operand in the document.
func TestFirewallPolicyUpdate_DropsStaleOperands(t *testing.T) {
	live := `{"_id":"p1","name":"n","action":"ALLOW","ip_version":"IPV4","schedule":{"mode":"ALWAYS"},
		"source":{"zone_id":"z","matching_target":"ANY","port_matching_type":"ANY"},
		"destination":{"zone_id":"z","matching_target":"IP","matching_target_type":"SPECIFIC","ips":["203.0.113.10"],
			"port":"443","port_matching_type":"SPECIFIC","app_ids":["unmodeled"]}}`
	f := fakeQueue(live, live)
	p := &FirewallPolicy{}
	p.Name, p.Action = "n", "ALLOW"
	p.Source = PolicyEndpoint{ZoneID: "z", MatchingTarget: "ANY", PortMatchingType: "ANY"}
	p.Destination = PolicyEndpoint{ZoneID: "z", MatchingTarget: "ANY", PortMatchingType: "ANY"}
	if _, err := newTestClient(f).Firewall().Policies().Update(context.Background(), "p1", p); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(f.body)
	var doc struct {
		Destination map[string]any `json:"destination"`
	}
	_ = json.Unmarshal(b, &doc)
	for _, k := range []string{"ips", "port", "matching_target_type"} {
		if _, ok := doc.Destination[k]; ok {
			t.Errorf("stale %q kept: %v", k, doc.Destination)
		}
	}
	if doc.Destination["app_ids"] == nil || doc.Destination["port_matching_type"] != "ANY" {
		t.Errorf("destination = %v", doc.Destination)
	}
}

func TestFirewallPolicyCreate_AcceptsArrayEcho(t *testing.T) {
	f := fakeQueue(`[{"_id":"p9","name":"n","action":"BLOCK","ip_version":"IPV4","source":{"zone_id":"z","matching_target":"ANY"},"destination":{"zone_id":"z","matching_target":"ANY"}}]`)
	p := &FirewallPolicy{}
	p.Name = "n"
	got, err := newTestClient(f).Firewall().Policies().Create(context.Background(), p)
	if err != nil || got.UnderscoreId == nil || *got.UnderscoreId != "p9" {
		t.Fatalf("got %v, %v", got, err)
	}
}
