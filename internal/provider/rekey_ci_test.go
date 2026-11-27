package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The zone-based firewall migration re-keys every object from a 24-hex
// ObjectID to a UUID. List-and-filter reads find nothing under the old id and
// must recover by a stable key instead of dropping the object from state.

const (
	oldHexID = "5f0a1b2c3d4e5f6a7b8c9d0e"
	newUUID  = "7d3e9a10-4444-4444-8444-000000000001"
)

func strVal(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

func stateID(t *testing.T, s tfsdk.State) string {
	t.Helper()
	var id types.String
	if d := s.GetAttribute(context.Background(), path.Root("id"), &id); d.HasError() {
		t.Fatal(d)
	}
	return id.ValueString()
}

func TestDeviceRead_AdoptsRekeyedIDByMAC(t *testing.T) {
	fc := newFakeController(t, map[string]string{
		"GET /stat/device": `{"meta":{"rc":"ok"},"data":[{"_id":"` + newUUID + `","mac":"aa:bb:cc:00:00:01","name":"core switch","type":"usw","adopted":true}]}`,
	})
	r := &deviceResource{net: fc.network(t)}
	sch := resourceSchema(t, r)

	out, removed := readResource(t, r, stateWith(sch, map[string]tftypes.Value{"id": strVal(oldHexID), "mac": strVal("aa:bb:cc:00:00:01")}))
	if removed {
		t.Fatal("device removed from state; want the re-keyed id adopted")
	}
	if got := stateID(t, out); got != newUUID {
		t.Errorf("id = %q, want %q", got, newUUID)
	}

	_, removed = readResource(t, r, stateWith(sch, map[string]tftypes.Value{"id": strVal(oldHexID), "mac": strVal("aa:bb:cc:00:00:99")}))
	if !removed {
		t.Error("device with no MAC match should be removed")
	}
}

func TestDNSRecordRead_AdoptsUniqueRekeyedMatch(t *testing.T) {
	cases := map[string]struct {
		list    string
		removed bool
	}{
		"unique": {`[{"_id":"` + newUUID + `","key":"nas.example.com","record_type":"A","value":"198.51.100.20","enabled":true},
			{"_id":"other","key":"nas.example.com","record_type":"AAAA","value":"2001:db8::20","enabled":true}]`, false},
		"ambiguous": {`[{"_id":"a","key":"nas.example.com","record_type":"A","value":"198.51.100.20"},
			{"_id":"b","key":"nas.example.com","record_type":"A","value":"198.51.100.21"}]`, true},
		"gone": {`[{"_id":"c","key":"other.example.com","record_type":"A","value":"198.51.100.22"}]`, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fc := newFakeController(t, map[string]string{"GET /static-dns": tc.list})
			r := &dnsRecordResource{net: fc.network(t)}
			out, removed := readResource(t, r, stateWith(resourceSchema(t, r), map[string]tftypes.Value{
				"id": strVal(oldHexID), "name": strVal("nas.example.com"), "type": strVal("A"), "value": strVal("198.51.100.20"),
			}))
			if removed != tc.removed {
				t.Fatalf("removed = %v, want %v", removed, tc.removed)
			}
			if !removed && stateID(t, out) != newUUID {
				t.Errorf("id = %q, want %q", stateID(t, out), newUUID)
			}
		})
	}
}

func TestWANSLARead_AdoptsRekeyedIDByName(t *testing.T) {
	fc := newFakeController(t, map[string]string{
		"GET /wan-slas": `[{"_id":"` + newUUID + `","name":"primary uplink","monitor_health_type":"ALL"}]`,
	})
	r := &wanSLAResource{net: fc.network(t)}
	out, removed := readResource(t, r, stateWith(resourceSchema(t, r), map[string]tftypes.Value{
		"id": strVal(oldHexID), "name": strVal("primary uplink"),
	}))
	if removed {
		t.Fatal("SLA removed from state; want the re-keyed id adopted")
	}
	if got := stateID(t, out); got != newUUID {
		t.Errorf("id = %q, want %q", got, newUUID)
	}
}

func TestFirewallRuleCreate_RefusedOnZoneFirewall(t *testing.T) {
	fc := newFakeController(t, map[string]string{
		"GET /site-feature-migration": `[{"feature":"ZONE_BASED_FIREWALL","timestamp":1763164800000}]`,
	})
	r := &firewallRuleResource{net: fc.network(t)}
	sch := resourceSchema(t, r)
	planned := stateWith(sch, map[string]tftypes.Value{"name": strVal("drop iot"), "ruleset": strVal("LAN_IN"), "action": strVal("drop")})
	plan := tfsdk.Plan{Schema: sch, Raw: planned.Raw}

	mp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(context.Background()), nil)}}, mp)
	if !mp.Diagnostics.HasError() || !strings.Contains(mp.Diagnostics.Errors()[0].Detail(), "unifi_network_firewall_policy") {
		t.Errorf("plan: want a zone-firewall error pointing at unifi_network_firewall_policy, got %v", mp.Diagnostics)
	}

	cr := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, cr)
	if !cr.Diagnostics.HasError() {
		t.Error("create: want an error")
	}
	if _, posted := fc.lastCall(http.MethodPost, "/rest/firewallrule"); posted {
		t.Error("create POSTed a legacy rule to a zone-based site")
	}
}

func TestFirewallRulePlan_AllowedOnClassicSite(t *testing.T) {
	fc := newFakeController(t, map[string]string{"GET /site-feature-migration": `[]`})
	r := &firewallRuleResource{net: fc.network(t)}
	sch := resourceSchema(t, r)
	planned := stateWith(sch, map[string]tftypes.Value{"name": strVal("drop iot")})
	plan := tfsdk.Plan{Schema: sch, Raw: planned.Raw}
	mp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(context.Background()), nil)}}, mp)
	if mp.Diagnostics.HasError() {
		t.Errorf("classic site: %v", mp.Diagnostics)
	}
}

func TestFirewallZoneDataSource(t *testing.T) {
	zones := `[{"_id":"` + zoneInternal + `","name":"Internal","zone_key":"internal","default_zone":true,"network_ids":["` + netTrusted + `"]},
		{"_id":"` + zoneExternal + `","name":"External","zone_key":"external","default_zone":true,"network_ids":[]},
		{"_id":"z-c1","name":"Cameras","default_zone":false,"network_ids":["` + netIoT + `"]},
		{"_id":"z-c2","name":"Cameras","zone_key":"","default_zone":false,"network_ids":[]}]`
	fc := newFakeController(t, map[string]string{"GET /firewall/zone": zones})
	d := &firewallZoneDataSource{net: fc.network(t)}
	var sr datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &sr)
	ty := sr.Schema.Type().TerraformType(context.Background()).(tftypes.Object)

	read := func(key, name string) (*datasource.ReadResponse, firewallZoneDataSourceModel) {
		vals := map[string]tftypes.Value{}
		for n, at := range ty.AttributeTypes {
			vals[n] = tftypes.NewValue(at, nil)
		}
		if key != "" {
			vals["zone_key"] = strVal(key)
		}
		if name != "" {
			vals["name"] = strVal(name)
		}
		raw := tftypes.NewValue(ty, vals)
		resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sr.Schema, Raw: raw}}
		d.Read(context.Background(), datasource.ReadRequest{Config: tfsdk.Config{Schema: sr.Schema, Raw: raw}}, resp)
		var m firewallZoneDataSourceModel
		if !resp.Diagnostics.HasError() {
			resp.State.Get(context.Background(), &m)
		}
		return resp, m
	}

	if resp, m := read("external", ""); resp.Diagnostics.HasError() || m.ID.ValueString() != zoneExternal || !m.DefaultZone.ValueBool() {
		t.Errorf("by zone_key: %v %+v", resp.Diagnostics, m)
	}
	if resp, m := read("internal", "Internal"); resp.Diagnostics.HasError() || len(m.NetworkIDs.Elements()) != 1 {
		t.Errorf("by both: %v %+v", resp.Diagnostics, m)
	}
	if resp, _ := read("", "Cameras"); !resp.Diagnostics.HasError() {
		t.Error("duplicate name: want an error")
	}
	if resp, _ := read("dmz", ""); !resp.Diagnostics.HasError() {
		t.Error("absent zone: want an error")
	}
}
