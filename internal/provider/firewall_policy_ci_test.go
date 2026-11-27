package provider

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
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
	srv, ty, fc, _ := policyServerSchema(t, routes)
	return srv, ty, fc
}

func policyServerSchema(t *testing.T, routes map[string]string) (tfprotov6.ProviderServer, tftypes.Object, *fakeController, *tfprotov6.Schema) {
	t.Helper()
	t.Setenv("UNIFI_DISABLE_CLIENT_CACHE", "1")
	fc := newFakeController(t, routes)
	srv := providerserver.NewProtocol6(New("test")())()
	gpr, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	configureBindProvider(t, srv, gpr, fc.URL)
	sch := gpr.ResourceSchemas[policyType]
	return srv, sch.ValueType().(tftypes.Object), fc, sch
}

// importAndRead imports the policy with the given id and refreshes it, as
// `terraform import` does.
func importAndRead(t *testing.T, srv tfprotov6.ProviderServer, id string) *tfprotov6.DynamicValue {
	t.Helper()
	ctx := context.Background()
	imp, err := srv.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: policyType, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "import", imp.Diagnostics)
	read, err := srv.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: policyType, CurrentState: imp.ImportedResources[0].State})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "read", read.Diagnostics)
	return read.NewState
}

// planChange plans config against prior the way Terraform core does.
func planChange(t *testing.T, srv tfprotov6.ProviderServer, sch *tfprotov6.Schema, prior *tfprotov6.DynamicValue, config tftypes.Value) (*tfprotov6.PlanResourceChangeResponse, tftypes.Value, *tfprotov6.DynamicValue) {
	t.Helper()
	ty := sch.ValueType().(tftypes.Object)
	priorV, err := prior.Unmarshal(ty)
	if err != nil {
		t.Fatal(err)
	}
	cfgDV, _ := tfprotov6.NewDynamicValue(ty, config)
	propDV, _ := tfprotov6.NewDynamicValue(ty, proposedNewState(config, priorV, sch.Block.Attributes))
	plan, err := srv.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
		TypeName: policyType, PriorState: prior, Config: &cfgDV, ProposedNewState: &propDV,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan, priorV, &cfgDV
}

// TestFirewallPolicy_ImportPlansClean is the import property: import a live
// policy, refresh it, and plan its intent-only config — the plan must be empty.
func TestFirewallPolicy_ImportPlansClean(t *testing.T) {
	for name, live := range livePolicies {
		t.Run(name, func(t *testing.T) {
			var doc struct {
				ID string `json:"_id"`
			}
			if err := json.Unmarshal([]byte(live), &doc); err != nil {
				t.Fatal(err)
			}
			srv, ty, _, sch := policyServerSchema(t, map[string]string{"GET /firewall-policies/" + doc.ID: live})
			plan, prior, _ := planChange(t, srv, sch, importAndRead(t, srv, doc.ID), tfValue(ty, intents[name]))
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

// applyUpdate imports a live policy, plans config against it and applies,
// returning the PUT body. The fake echoes the PUT, so apply also runs the
// framework's plan-vs-result consistency check against what was sent.
func applyUpdate(t *testing.T, live string, id string, config map[string]any) map[string]any {
	t.Helper()
	srv, ty, fc, sch := policyServerSchema(t, map[string]string{
		"GET /firewall-policies/" + id: live,
		"PUT /firewall-policies/" + id: "<echo>",
	})
	prior := importAndRead(t, srv, id)
	plan, _, cfgDV := planChange(t, srv, sch, prior, tfValue(ty, config))
	assertNoErrorDiags(t, "plan", plan.Diagnostics)
	apply, err := srv.ApplyResourceChange(context.Background(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName: policyType, PriorState: prior, Config: cfgDV, PlannedState: plan.PlannedState,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "apply", apply.Diagnostics)
	call, ok := fc.lastCall(http.MethodPut, "/firewall-policies/"+id)
	if !ok {
		t.Fatal("no PUT")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(call.Body), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertEndpoint(t *testing.T, body map[string]any, side string, want map[string]any, absent ...string) {
	t.Helper()
	e, _ := body[side].(map[string]any)
	for k, v := range want {
		got, _ := json.Marshal(e[k])
		exp, _ := json.Marshal(v)
		if string(got) != string(exp) {
			t.Errorf("%s.%s = %s, want %s", side, k, got, exp)
		}
	}
	for _, k := range absent {
		if _, ok := e[k]; ok {
			t.Errorf("%s.%s = %v, want it removed", side, k, e[k])
		}
	}
}

// httpsIntent is the to-host-https policy's config with destination swapped.
func httpsIntent(dest map[string]any) map[string]any {
	return map[string]any{
		"name": "allow https to host", "action": "ALLOW", "protocol": "tcp", "create_allow_respond": true,
		"source":      map[string]any{"zone_id": zoneExternal, "matching_target": "ANY"},
		"destination": dest,
	}
}

func TestFirewallPolicyUpdate_IPSpecificToAny(t *testing.T) {
	body := applyUpdate(t, livePolicies["to-host-https"], "p-https",
		httpsIntent(map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"}))
	assertEndpoint(t, body, "destination", map[string]any{"matching_target": "ANY", "port_matching_type": "ANY"},
		"ips", "port", "matching_target_type")
	if body["index"] != float64(10002) {
		t.Errorf("index = %v, want the live 10002", body["index"])
	}
}

func TestFirewallPolicyUpdate_IPsToGroup(t *testing.T) {
	body := applyUpdate(t, livePolicies["to-host-https"], "p-https", httpsIntent(map[string]any{
		"zone_id": zoneInternal, "matching_target": "IP", "ip_group_id": ipGroup, "port": "443",
	}))
	assertEndpoint(t, body, "destination", map[string]any{
		"matching_target_type": "OBJECT", "ip_group_id": ipGroup, "port": "443", "port_matching_type": "SPECIFIC",
	}, "ips")
}

func TestFirewallPolicyUpdate_PortSpecificToGroup(t *testing.T) {
	body := applyUpdate(t, livePolicies["to-host-https"], "p-https", httpsIntent(map[string]any{
		"zone_id": zoneInternal, "matching_target": "IP", "ips": []string{"203.0.113.10"}, "port_group_id": "pg-web",
	}))
	assertEndpoint(t, body, "destination", map[string]any{
		"port_matching_type": "OBJECT", "port_group_id": "pg-web", "matching_target_type": "SPECIFIC",
	}, "port")
}

// Removing port (and the inverted-port flag) from the config clears both.
func TestFirewallPolicyUpdate_RemovingPortClearsIt(t *testing.T) {
	cfg := intents["multi-port"]
	cfg = map[string]any{
		"name": cfg["name"], "action": cfg["action"], "enabled": false, "ip_version": "BOTH", "protocol": "tcp_udp",
		"source":      cfg["source"],
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"},
	}
	body := applyUpdate(t, livePolicies["multi-port"], "p-ports", cfg)
	assertEndpoint(t, body, "destination", map[string]any{"port_matching_type": "ANY", "match_opposite_ports": false}, "port")
}

func TestFirewallPolicyRead_404RemovesFromState(t *testing.T) {
	srv, ty, _ := policyServer(t, nil) // every v2 path 404s
	state := tfValue(ty, map[string]any{"id": "p-gone", "name": "x", "action": "BLOCK",
		"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"},
		"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"}})
	dv, _ := tfprotov6.NewDynamicValue(ty, state)
	read, err := srv.ReadResource(context.Background(), &tfprotov6.ReadResourceRequest{TypeName: policyType, CurrentState: &dv})
	if err != nil {
		t.Fatal(err)
	}
	assertNoErrorDiags(t, "read", read.Diagnostics)
	if v, _ := read.NewState.Unmarshal(ty); !v.IsNull() {
		t.Errorf("state = %v, want removed", v)
	}
}

func TestFirewallPolicy_ValidateConfigRejectsContradictions(t *testing.T) {
	srv, ty, _ := policyServer(t, nil)
	cases := map[string]map[string]any{
		"port and port group": {"zone_id": zoneInternal, "matching_target": "ANY", "port": "443", "port_group_id": "pg"},
		"specific with group": {"zone_id": zoneInternal, "matching_target": "ANY", "port_matching_type": "SPECIFIC", "port_group_id": "pg"},
		"any target with ips": {"zone_id": zoneInternal, "matching_target": "ANY", "ips": []string{"203.0.113.1"}},
		"ips and ip group":    {"zone_id": zoneInternal, "matching_target": "IP", "ips": []string{"203.0.113.1"}, "ip_group_id": ipGroup},
	}
	for name, dest := range cases {
		cfg := tfValue(ty, map[string]any{"name": "x", "action": "BLOCK",
			"source": map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"}, "destination": dest})
		dv, _ := tfprotov6.NewDynamicValue(ty, cfg)
		resp, err := srv.ValidateResourceConfig(context.Background(), &tfprotov6.ValidateResourceConfigRequest{TypeName: policyType, Config: &dv})
		if err != nil {
			t.Fatal(err)
		}
		hasErr := false
		for _, d := range resp.Diagnostics {
			hasErr = hasErr || d.Severity == tfprotov6.DiagnosticSeverityError
		}
		if !hasErr {
			t.Errorf("%s: want a validation error", name)
		}
	}
}

func TestFirewallPolicy_CreateGatedOnClassicSite(t *testing.T) {
	for name, tc := range map[string]struct {
		routes  map[string]string
		wantErr bool
	}{
		"classic":     {routes: map[string]string{"GET /site-feature-migration": `[]`, "GET /firewall/zone": `[]`}, wantErr: true},
		"probe fails": {routes: map[string]string{"GET /site-feature-migration": "<400>", "GET /firewall/zone": "<400>"}},
	} {
		t.Run(name, func(t *testing.T) {
			srv, ty, _ := policyServer(t, tc.routes)
			cfg := tfValue(ty, map[string]any{"name": "x", "action": "BLOCK",
				"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"},
				"destination": map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"}})
			cfgDV, _ := tfprotov6.NewDynamicValue(ty, cfg)
			priorDV, _ := tfprotov6.NewDynamicValue(ty, tftypes.NewValue(ty, nil))
			plan, err := srv.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
				TypeName: policyType, PriorState: &priorDV, Config: &cfgDV, ProposedNewState: &cfgDV,
			})
			if err != nil {
				t.Fatal(err)
			}
			hasErr, hasWarn := false, false
			for _, d := range plan.Diagnostics {
				hasErr = hasErr || d.Severity == tfprotov6.DiagnosticSeverityError
				hasWarn = hasWarn || d.Severity == tfprotov6.DiagnosticSeverityWarning
			}
			if hasErr != tc.wantErr || (!tc.wantErr && !hasWarn) {
				t.Errorf("error=%v warning=%v, want error=%v (else a warning)", hasErr, hasWarn, tc.wantErr)
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
	srv, ty, fc := policyServer(t, map[string]string{
		"POST /firewall-policies": created,
		"GET /firewall/zone":      `[{"_id":"` + zoneInternal + `","zone_key":"internal"}]`,
	})

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

// TestFirewallPolicy_CreateAllowRespondDefaultsFalse: omitted, the controller
// 400s a create into the Gateway zone (#8), so create sends false there unless
// set. Into any other zone an unset attribute stays omitted.
func TestFirewallPolicy_CreateAllowRespondDefaultsFalse(t *testing.T) {
	const zoneGateway = "0a1b2c3d-1111-4111-8111-000000000003"
	for name, tc := range map[string]struct {
		dst  string
		set  any // nil leaves the attribute unset
		want any // nil expects the key absent
	}{
		"unset gateway":  {dst: zoneGateway, want: false},
		"unset internal": {dst: zoneInternal},
		"false gateway":  {dst: zoneGateway, set: false, want: false},
		"true gateway":   {dst: zoneGateway, set: true, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			created := `{"_id":"p-gw","action":"ALLOW","connection_state_type":"ALL","enabled":true,"index":10020,
				"ip_version":"IPV4","name":"to gateway","predefined":false,"protocol":"all",
				"create_allow_respond":` + strconv.FormatBool(tc.want == true) + `,
				"source":{"zone_id":"` + zoneInternal + `","matching_target":"ANY","port_matching_type":"ANY"},
				"destination":{"zone_id":"` + tc.dst + `","matching_target":"ANY","port_matching_type":"ANY"}}`
			srv, ty, fc := policyServer(t, map[string]string{
				"POST /firewall-policies": created,
				"GET /firewall/zone": `[{"_id":"` + zoneInternal + `","zone_key":"internal"},
					{"_id":"` + zoneGateway + `","zone_key":"gateway"}]`,
			})
			cfgMap := map[string]any{"name": "to gateway", "action": "ALLOW",
				"source":      map[string]any{"zone_id": zoneInternal, "matching_target": "ANY"},
				"destination": map[string]any{"zone_id": tc.dst, "matching_target": "ANY"}}
			if tc.set != nil {
				cfgMap["create_allow_respond"] = tc.set
			}
			cfgDV, _ := tfprotov6.NewDynamicValue(ty, tfValue(ty, cfgMap))
			priorDV, _ := tfprotov6.NewDynamicValue(ty, tftypes.NewValue(ty, nil))
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
			if got, ok := body["create_allow_respond"]; ok != (tc.want != nil) || got != tc.want {
				t.Errorf("create_allow_respond = %v (present=%v), want %v", got, ok, tc.want)
			}
		})
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

// proposedNewState mimics Terraform core: a null config attribute takes the
// prior value when it is Computed, and stays null otherwise.
func proposedNewState(config, prior tftypes.Value, attrs []*tfprotov6.SchemaAttribute) tftypes.Value {
	var cfg, pri map[string]tftypes.Value
	_ = config.As(&cfg)
	if !prior.IsNull() {
		_ = prior.As(&pri)
	}
	out := map[string]tftypes.Value{}
	for _, a := range attrs {
		c := cfg[a.Name]
		p, ok := pri[a.Name]
		if !ok {
			p = tftypes.NewValue(c.Type(), nil)
		}
		switch {
		case a.NestedType != nil && !c.IsNull():
			out[a.Name] = proposedNewState(c, p, a.NestedType.Attributes)
		case c.IsNull() && a.Computed:
			out[a.Name] = p
		default:
			out[a.Name] = c
		}
	}
	return tftypes.NewValue(config.Type(), out)
}
