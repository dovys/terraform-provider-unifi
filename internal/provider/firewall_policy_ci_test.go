package provider

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	zoneInternal = "0a1b2c3d-1111-4111-8111-000000000001"
	zoneExternal = "0a1b2c3d-1111-4111-8111-000000000002"
	netTrusted   = "0a1b2c3d-2222-4222-8222-000000000001"
	netIoT       = "0a1b2c3d-2222-4222-8222-000000000002"
	ipGroup      = "0a1b2c3d-3333-4333-8333-000000000001"
)

// Live shapes from a migrated Network 11 site: the controller spells "unset"
// as [], "" or an absent key interchangeably.
var livePolicies = map[string]string{
	"network-to-network": `{"_id":"p-net","action":"BLOCK","connection_state_type":"CUSTOM","connection_states":["NEW"],
		"create_allow_respond":false,"enabled":true,"hits":7,"icmp_typename":"ANY","icmp_v6_typename":"ANY","index":10000,
		"ip_version":"IPV4","last_hit":1763000000,"logging":false,"match_ip_sec":false,"match_ip_sec_type":"MATCH_IP_SEC",
		"match_opposite_protocol":false,"name":"block iot to trusted","origin_id":"","origin_type":"custom_firewall_rule",
		"predefined":false,"protocol":"all","report_flow":false,"report_flow_supported":true,"schedule":{"mode":"ALWAYS"},
		"source":{"ip_group_id":"","ips":[],"match_mac":false,"match_opposite_ips":false,"match_opposite_networks":false,
			"match_opposite_ports":false,"matching_target":"NETWORK","matching_target_type":"SPECIFIC","network_ids":["` + netIoT + `"],
			"port":"","port_matching_type":"ANY","zone_id":"` + zoneInternal + `"},
		"destination":{"ips":[],"match_opposite_ips":false,"match_opposite_networks":false,"match_opposite_ports":false,
			"matching_target":"NETWORK","matching_target_type":"SPECIFIC","network_ids":["` + netTrusted + `"],"port":"",
			"port_matching_type":"ANY","zone_id":"` + zoneInternal + `"}}`,
	"group-to-wan": `{"_id":"p-grp","action":"BLOCK","connection_state_type":"ALL","connection_states":[],
		"create_allow_respond":false,"enabled":true,"hits":0,"index":10001,"ip_version":"IPV4","logging":true,
		"match_ip_sec":false,"match_opposite_protocol":false,"name":"no internet for cameras","predefined":false,
		"protocol":"all","schedule":{"mode":"ALWAYS"},
		"source":{"ip_group_id":"` + ipGroup + `","match_mac":false,"match_opposite_ips":false,"match_opposite_ports":false,
			"matching_target":"IP","matching_target_type":"OBJECT","port_matching_type":"ANY","zone_id":"` + zoneInternal + `"},
		"destination":{"match_opposite_ips":false,"match_opposite_ports":false,"matching_target":"ANY",
			"port_matching_type":"ANY","zone_id":"` + zoneExternal + `"}}`,
	"to-host-https": `{"_id":"p-https","action":"ALLOW","connection_state_type":"ALL","create_allow_respond":true,"enabled":true,
		"index":10002,"ip_version":"IPV4","logging":false,"match_ip_sec":false,"match_opposite_protocol":false,
		"name":"allow https to host","predefined":false,"protocol":"tcp","schedule":{"mode":"ALWAYS"},
		"source":{"matching_target":"ANY","port_matching_type":"ANY","zone_id":"` + zoneExternal + `","network_ids":[]},
		"destination":{"ips":["203.0.113.10"],"matching_target":"IP","matching_target_type":"SPECIFIC","port":"443",
			"port_matching_type":"SPECIFIC","zone_id":"` + zoneInternal + `"}}`,
	"multi-port": `{"_id":"p-ports","action":"ALLOW","connection_state_type":"ALL","enabled":false,"index":10003,
		"ip_version":"BOTH","name":"infra ports","predefined":false,"protocol":"tcp_udp","schedule":{"mode":"ALWAYS"},
		"source":{"matching_target":"NETWORK","matching_target_type":"SPECIFIC","network_ids":["` + netIoT + `","` + netTrusted + `"],
			"port_matching_type":"ANY","zone_id":"` + zoneInternal + `"},
		"destination":{"matching_target":"ANY","port":"22,53,80,443,8443","port_matching_type":"SPECIFIC",
			"match_opposite_ports":true,"zone_id":"` + zoneInternal + `"}}`,
}

// intents are configs that state only what each policy means.
var intents = map[string]map[string]any{
	"network-to-network": {
		"name": "block iot to trusted", "action": "BLOCK",
		"connection_state_type": "CUSTOM", "connection_states": []string{"NEW"},
		"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "NETWORK", "network_ids": []string{netIoT}},
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "NETWORK", "network_ids": []string{netTrusted}},
	},
	"group-to-wan": {
		"name": "no internet for cameras", "action": "BLOCK", "logging": true,
		"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "IP", "matching_target_type": "OBJECT", "ip_group_id": ipGroup},
		"destination": map[string]any{"zone_id": zoneExternal, "matching_target": "ANY"},
	},
	"to-host-https": {
		"name": "allow https to host", "action": "ALLOW", "protocol": "tcp", "create_allow_respond": true,
		"source":      map[string]any{"zone_id": zoneExternal, "matching_target": "ANY"},
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "IP", "ips": []string{"203.0.113.10"}, "port": "443"},
	},
	"multi-port": {
		"name": "infra ports", "action": "ALLOW", "enabled": false, "ip_version": "BOTH", "protocol": "tcp_udp",
		"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "NETWORK", "network_ids": []string{netTrusted, netIoT}},
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "ANY", "port": "22,53,80,443,8443", "match_opposite_ports": true},
	},
}

const policyType = "unifi_network_firewall_policy"

// policyServer configures a provider server against a fake that serves the
// given policies by id.
func policyServer(t *testing.T, routes map[string]string) (tfprotov6.ProviderServer, tftypes.Object, *fakeController) {
	t.Helper()
	t.Setenv("UNIFI_DISABLE_CLIENT_CACHE", "1")
	fc := newFakeController(t, routes)
	srv := providerserver.NewProtocol6(New("test")())()
	gpr, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	configureBindProvider(t, srv, gpr, fc.URL)
	return srv, gpr.ResourceSchemas[policyType].ValueType().(tftypes.Object), fc
}

// TestFirewallPolicy_ImportPlansClean is the import property: import a live
// policy, refresh it, and plan its intent-only config — the plan must be empty.
func TestFirewallPolicy_ImportPlansClean(t *testing.T) {
	ctx := context.Background()
	for name, live := range livePolicies {
		t.Run(name, func(t *testing.T) {
			var doc struct {
				ID string `json:"_id"`
			}
			if err := json.Unmarshal([]byte(live), &doc); err != nil {
				t.Fatal(err)
			}
			srv, ty, _ := policyServer(t, map[string]string{"GET /firewall-policies/" + doc.ID: live})

			imp, err := srv.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: policyType, ID: doc.ID})
			if err != nil {
				t.Fatal(err)
			}
			assertNoErrorDiags(t, "import", imp.Diagnostics)
			read, err := srv.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: policyType, CurrentState: imp.ImportedResources[0].State})
			if err != nil {
				t.Fatal(err)
			}
			assertNoErrorDiags(t, "read", read.Diagnostics)
			prior, err := read.NewState.Unmarshal(ty)
			if err != nil {
				t.Fatal(err)
			}

			config := tfValue(ty, intents[name])
			proposed := proposedNewState(config, prior)
			cfgDV, _ := tfprotov6.NewDynamicValue(ty, config)
			propDV, _ := tfprotov6.NewDynamicValue(ty, proposed)
			plan, err := srv.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
				TypeName: policyType, PriorState: read.NewState, Config: &cfgDV, ProposedNewState: &propDV,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertNoErrorDiags(t, "plan", plan.Diagnostics)
			planned, _ := plan.PlannedState.Unmarshal(ty)
			if diffs, _ := prior.Diff(planned); len(diffs) > 0 {
				for _, d := range diffs {
					t.Errorf("diff at %s: %v -> %v", d.Path, d.Value1, d.Value2)
				}
			}
			if len(plan.RequiresReplace) > 0 {
				t.Errorf("requires replace: %v", plan.RequiresReplace)
			}
		})
	}
}

// TestFirewallPolicy_CreateSendsFullDTO plans and applies a new policy and
// checks the POST carries what the v2 DTO requires, and no ordering.
func TestFirewallPolicy_CreateSendsFullDTO(t *testing.T) {
	ctx := context.Background()
	created := `{"_id":"p-new","action":"BLOCK","connection_state_type":"ALL","connection_states":[],"enabled":true,
		"index":10010,"ip_version":"IPV4","logging":false,"name":"block iot to trusted","predefined":false,"protocol":"all",
		"create_allow_respond":false,"match_ip_sec":false,"match_opposite_protocol":false,"schedule":{"mode":"ALWAYS"},
		"source":{"zone_id":"` + zoneInternal + `","matching_target":"NETWORK","matching_target_type":"SPECIFIC",
			"network_ids":["` + netIoT + `"],"ips":[],"port":"","port_matching_type":"ANY","match_mac":false,
			"match_opposite_ips":false,"match_opposite_networks":false,"match_opposite_ports":false},
		"destination":{"zone_id":"` + zoneInternal + `","matching_target":"NETWORK","matching_target_type":"SPECIFIC",
			"network_ids":["` + netTrusted + `"],"port_matching_type":"ANY","match_opposite_ips":false,
			"match_opposite_networks":false,"match_opposite_ports":false}}`
	srv, ty, fc := policyServer(t, map[string]string{"POST /firewall-policies": created})

	config := tfValue(ty, map[string]any{
		"name": "block iot to trusted", "action": "BLOCK",
		"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "NETWORK", "network_ids": []string{netIoT}},
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "NETWORK", "network_ids": []string{netTrusted}},
	})
	prior := tftypes.NewValue(ty, nil)
	cfgDV, _ := tfprotov6.NewDynamicValue(ty, config)
	priorDV, _ := tfprotov6.NewDynamicValue(ty, prior)
	plan, err := srv.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName: policyType, PriorState: &priorDV, Config: &cfgDV, ProposedNewState: &cfgDV,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "plan", plan.Diagnostics)
	apply, err := srv.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName: policyType, PriorState: &priorDV, Config: &cfgDV, PlannedState: plan.PlannedState,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "apply", apply.Diagnostics)

	call, ok := fc.lastCall(http.MethodPost, "/firewall-policies")
	if !ok {
		t.Fatal("no POST")
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(call.Body), &body)
	for k, want := range map[string]any{"ip_version": "IPV4", "protocol": "all", "enabled": true, "connection_state_type": "ALL"} {
		if body[k] != want {
			t.Errorf("%s = %v, want %v", k, body[k], want)
		}
	}
	if s, _ := json.Marshal(body["schedule"]); string(s) != `{"mode":"ALWAYS"}` {
		t.Errorf("schedule = %s", s)
	}
	if _, ok := body["index"]; ok {
		t.Error("create sent index; ordering is the controller's")
	}
	src := body["source"].(map[string]any)
	if src["port_matching_type"] != "ANY" || src["matching_target_type"] != "SPECIFIC" {
		t.Errorf("source defaults = %v", src)
	}
}

func TestFirewallPolicy_PredefinedRefused(t *testing.T) {
	fc := newFakeController(t, map[string]string{
		"GET /firewall-policies/p-sys": `{"_id":"p-sys","name":"Allow Return Traffic","action":"ALLOW","predefined":true,
			"ip_version":"BOTH","source":{"zone_id":"z","matching_target":"ANY"},"destination":{"zone_id":"z","matching_target":"ANY"}}`,
	})
	r := &firewallPolicyResource{net: fc.network(t)}
	state := stateWith(resourceSchema(t, r), map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "p-sys")})
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Predefined") {
		t.Errorf("want a predefined-policy error, got %v", resp.Diagnostics)
	}
}

func TestFirewallPolicy_ImportByName(t *testing.T) {
	ctx := context.Background()
	list := "[" + livePolicies["group-to-wan"] + "," + livePolicies["multi-port"] + "]"
	srv, ty, _ := policyServer(t, map[string]string{"GET /firewall-policies": list})
	imp, err := srv.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: policyType, ID: "name=infra ports"})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "import", imp.Diagnostics)
	v, err := imp.ImportedResources[0].State.Unmarshal(ty)
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]tftypes.Value
	_ = v.As(&attrs)
	var id string
	_ = attrs["id"].As(&id)
	if id != "p-ports" {
		t.Errorf("imported id = %q, want p-ports", id)
	}
}

// tfValue builds a value of ty from a Go literal; missing keys are null.
func tfValue(ty tftypes.Type, v any) tftypes.Value {
	if v == nil {
		return tftypes.NewValue(ty, nil)
	}
	switch tv := v.(type) {
	case map[string]any:
		obj := ty.(tftypes.Object)
		vals := map[string]tftypes.Value{}
		for name, at := range obj.AttributeTypes {
			vals[name] = tfValue(at, tv[name])
		}
		return tftypes.NewValue(ty, vals)
	case []string:
		elems := make([]tftypes.Value, len(tv))
		for i, s := range tv {
			elems[i] = tftypes.NewValue(tftypes.String, s)
		}
		return tftypes.NewValue(ty, elems)
	case int:
		return tftypes.NewValue(ty, big.NewFloat(float64(tv)))
	default:
		return tftypes.NewValue(ty, tv)
	}
}

// proposedNewState mimics Terraform core for this schema: a null config
// attribute takes the prior value (every Optional attribute is also Computed).
func proposedNewState(config, prior tftypes.Value) tftypes.Value {
	if config.IsNull() {
		return prior
	}
	obj, ok := config.Type().(tftypes.Object)
	if !ok {
		return config
	}
	var cfg, pri map[string]tftypes.Value
	_ = config.As(&cfg)
	if !prior.IsNull() {
		_ = prior.As(&pri)
	}
	out := map[string]tftypes.Value{}
	for name, at := range obj.AttributeTypes {
		p, ok := pri[name]
		if !ok {
			p = tftypes.NewValue(at, nil)
		}
		out[name] = proposedNewState(cfg[name], p)
	}
	return tftypes.NewValue(obj, out)
}
