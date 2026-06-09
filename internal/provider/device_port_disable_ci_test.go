package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

// Coverage for issue #108 — administratively disabling a switch port. The
// controller has no direct disable field: forward=disabled is DERIVED from
// tagged_vlan_mgmt=block_all with an empty native VLAN. The provider exposes a
// `disabled` bool that writes that pair and reads back from the derived forward.

// TestDevicePortDisable_FromNetDerivesDisabled: a controller override with
// forward=disabled reads back disabled=true; any other forward reads false.
func TestDevicePortDisable_FromNetDerivesDisabled(t *testing.T) {
	if !portOverrideFromNet(network.PortOverride{PortIdx: 7, Forward: network.ForwardDisabled}).Disabled.ValueBool() {
		t.Error("forward=disabled must read back disabled=true (#108)")
	}
	if portOverrideFromNet(network.PortOverride{PortIdx: 7, Forward: "all"}).Disabled.ValueBool() {
		t.Error("forward=all must read back disabled=false")
	}
	// Write read-back intent: a freshly-written port whose document hasn't echoed
	// yet rides in on PortOverride.Disabled even with forward still stale — both ways.
	disable, enable := true, false
	if !portOverrideFromNet(network.PortOverride{PortIdx: 7, Disabled: &disable, Forward: "native"}).Disabled.ValueBool() {
		t.Error("configured disabled=true intent must survive a lagging forward read (#108)")
	}
	if portOverrideFromNet(network.PortOverride{PortIdx: 7, Disabled: &enable, Forward: network.ForwardDisabled}).Disabled.ValueBool() {
		t.Error("configured disabled=false intent must survive a lagging forward=disabled read (#108)")
	}
}

// TestDevicePortDisable_ToNetSetsMarker: disabled=true in config sets the
// PortOverride.Disabled write marker the merge expands.
func TestDevicePortDisable_ToNetSetsMarker(t *testing.T) {
	var diags diag.Diagnostics
	o := portOverrideObject{
		Disabled:                 types.BoolValue(true),
		ExcludedNetworkIDs:       types.SetNull(types.StringType),
		PortSecurityMACAddresses: types.SetNull(types.StringType),
	}
	po := portOverrideToNet(context.Background(), 7, o, &diags)
	if diags.HasError() {
		t.Fatalf("toNet diags: %v", diags)
	}
	if po.Disabled == nil || !*po.Disabled {
		t.Errorf("disabled=true must set PortOverride.Disabled (#108), got %v", po.Disabled)
	}
}

// disablePortConfig builds a device config whose port "7" object carries the
// given leaf overrides (all other leaves null), for ValidateConfig.
func disablePortConfig(t *testing.T, sch rschema.Schema, attrs map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], nil) // null = unset in config
	}
	for k, v := range attrs {
		ovals[k] = v
	}
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"7": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	top := map[string]tftypes.Value{}
	for name, ty := range schemaType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "port_overrides":
			top[name] = poMap
		default:
			top[name] = tftypes.NewValue(ty, nil)
		}
	}
	return tfsdk.Config{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}

func validateDeviceConfig(t *testing.T, cfg tfsdk.Config) diag.Diagnostics {
	t.Helper()
	resp := &resource.ValidateConfigResponse{}
	(&deviceResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: cfg}, resp)
	return resp.Diagnostics
}

// TestDevicePortDisable_ValidateConfigRejectsVLANConflict: disabled=true paired
// with a VLAN/profile field is a contradiction (the disable would silently win),
// so it must error at plan time.
func TestDevicePortDisable_ValidateConfigRejectsVLANConflict(t *testing.T) {
	sch := deviceResourceSchema(t)
	cases := map[string]map[string]tftypes.Value{
		"native_network_id": {
			"disabled":          tftypes.NewValue(tftypes.Bool, true),
			"native_network_id": tftypes.NewValue(tftypes.String, "netA"),
		},
		"port_profile_id": {
			"disabled":        tftypes.NewValue(tftypes.Bool, true),
			"port_profile_id": tftypes.NewValue(tftypes.String, "prof1"),
		},
		"tagged_vlan_mgmt=auto": {
			"disabled":         tftypes.NewValue(tftypes.Bool, true),
			"tagged_vlan_mgmt": tftypes.NewValue(tftypes.String, "auto"),
		},
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			if d := validateDeviceConfig(t, disablePortConfig(t, sch, attrs)); !d.HasError() {
				t.Errorf("disabled=true with %s must produce a config error (#108)", name)
			}
		})
	}
}

// TestDevicePortDisable_ValidateConfigAllowsCleanDisable: disabled alone (or with
// the redundant tagged_vlan_mgmt=block_all) is valid.
func TestDevicePortDisable_ValidateConfigAllowsCleanDisable(t *testing.T) {
	sch := deviceResourceSchema(t)
	cases := map[string]map[string]tftypes.Value{
		"disabled only": {
			"disabled": tftypes.NewValue(tftypes.Bool, true),
		},
		"disabled + block_all": {
			"disabled":         tftypes.NewValue(tftypes.Bool, true),
			"tagged_vlan_mgmt": tftypes.NewValue(tftypes.String, "block_all"),
		},
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			if d := validateDeviceConfig(t, disablePortConfig(t, sch, attrs)); d.HasError() {
				t.Errorf("clean disable (%s) must validate, got: %v", name, d)
			}
		})
	}
}

// disabledDeviceJSON is a /stat/device list where port 7 reads back as the
// controller's derived disabled shape: forward=disabled, tagged_vlan_mgmt=
// block_all, native empty.
const disabledDeviceJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"US8","type":"usw","adopted":true,"port_overrides":[
	{"port_idx":7,"forward":"disabled","tagged_vlan_mgmt":"block_all","native_networkconf_id":""}
]}]}`

// disabledPlan builds a plan with port "7" = {disabled = true}; every other leaf
// unknown (the Optional+Computed shape the framework hands the provider).
func disabledPlan(t *testing.T, sch rschema.Schema, id string) tfsdk.Plan {
	t.Helper()
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], tftypes.UnknownValue)
	}
	ovals["disabled"] = tftypes.NewValue(tftypes.Bool, true)
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"7": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	top := map[string]tftypes.Value{}
	for name, ty := range schemaType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "id":
			if id != "" {
				top[name] = tftypes.NewValue(ty, id)
			} else {
				top[name] = tftypes.NewValue(ty, tftypes.UnknownValue)
			}
		case "port_overrides":
			top[name] = poMap
		case "forget_on_destroy":
			top[name] = tftypes.NewValue(ty, nil)
		default:
			top[name] = tftypes.NewValue(ty, tftypes.UnknownValue)
		}
	}
	return tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}

// TestDevicePortDisable_CreateConsistent: creating a device that disables port 7
// yields a consistent result — disabled=true round-trips and every Optional+
// Computed leaf resolves to a concrete value (no "inconsistent result after apply").
func TestDevicePortDisable_CreateConsistent(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResourceServing(t, disabledDeviceJSON)
	sch := deviceResourceSchema(t)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: disabledPlan(t, sch, "")}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)

	el, ok := m.PortOverrides.Elements()["7"]
	if !ok {
		t.Fatal("port 7 absent from post-apply state")
	}
	obj := el.(types.Object)
	if d := obj.Attributes()["disabled"].(types.Bool); !d.ValueBool() {
		t.Errorf("disabled = %v, want true — a configured disable must round-trip", d)
	}
	if f := obj.Attributes()["forward"].(types.String); f.ValueString() != network.ForwardDisabled {
		t.Errorf("forward = %q, want %q", f.ValueString(), network.ForwardDisabled)
	}
	for name, v := range obj.Attributes() {
		if v.IsUnknown() {
			t.Errorf("leaf %q is unknown in post-apply state — every Optional+Computed leaf must resolve", name)
		}
	}
}

// TestDevicePortDisable_CreateConsistentWhenReadLags: the same create, but the
// /stat/device read still echoes the port's OLD enabled shape (forward=native).
// The known disabled=true plan value must still round-trip via the carried intent,
// not flip to false off the stale forward.
func TestDevicePortDisable_CreateConsistentWhenReadLags(t *testing.T) {
	ctx := context.Background()
	const laggingJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"US8","type":"usw","adopted":true,"port_overrides":[
		{"port_idx":7,"forward":"native","tagged_vlan_mgmt":"auto","native_networkconf_id":"netA"}
	]}]}`
	r := bindDeviceResourceServing(t, laggingJSON)
	sch := deviceResourceSchema(t)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: disabledPlan(t, sch, "")}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)
	obj := m.PortOverrides.Elements()["7"].(types.Object)
	if d := obj.Attributes()["disabled"].(types.Bool); !d.ValueBool() {
		t.Errorf("disabled = %v, want true — must survive a lagging forward read (#108)", d)
	}
}

// TestDevicePortDisable_ReenableConsistent: flipping a disabled port to
// disabled=false re-enables it; the known false plan value must round-trip
// consistently (the controller serves the re-enabled trunk back).
func TestDevicePortDisable_ReenableConsistent(t *testing.T) {
	ctx := context.Background()
	// Controller serves port 7 re-enabled as a default trunk after the write.
	const reenabledJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"US8","type":"usw","adopted":true,"port_overrides":[
		{"port_idx":7,"forward":"all","tagged_vlan_mgmt":"auto","native_networkconf_id":"netDefault"}
	]}]}`
	r := bindDeviceResourceServing(t, reenabledJSON)
	sch := deviceResourceSchema(t)

	// Prior state: port 7 is disabled.
	prior := stateWithDisabledPort7(t, sch)
	// Plan: explicit disabled = false (a known plan value), no VLAN intent.
	plan := disabledPlanWithValue(t, sch, "d1", false)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: prior}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)
	obj := m.PortOverrides.Elements()["7"].(types.Object)
	if d := obj.Attributes()["disabled"].(types.Bool); d.ValueBool() {
		t.Errorf("disabled = %v, want false — a known disabled=false must round-trip (#108)", d)
	}
}

// disabledPlanWithValue is disabledPlan with an explicit disabled value (true/false).
func disabledPlanWithValue(t *testing.T, sch rschema.Schema, id string, disabled bool) tfsdk.Plan {
	t.Helper()
	p := disabledPlan(t, sch, id)
	// disabledPlan sets disabled=true; rewrite just that leaf.
	raw := p.Raw
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], tftypes.UnknownValue)
	}
	ovals["disabled"] = tftypes.NewValue(tftypes.Bool, disabled)
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"7": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	var top map[string]tftypes.Value
	_ = raw.As(&top)
	top["port_overrides"] = poMap
	return tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}

// stateWithDisabledPort7 builds prior state where port 7 reads as disabled.
func stateWithDisabledPort7(t *testing.T, sch rschema.Schema) tfsdk.State {
	t.Helper()
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], nil)
	}
	ovals["disabled"] = tftypes.NewValue(tftypes.Bool, true)
	ovals["forward"] = tftypes.NewValue(tftypes.String, "disabled")
	ovals["tagged_vlan_mgmt"] = tftypes.NewValue(tftypes.String, "block_all")
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"7": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	top := map[string]tftypes.Value{}
	for name, ty := range schemaType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "id":
			top[name] = tftypes.NewValue(ty, "d1")
		case "port_overrides":
			top[name] = poMap
		default:
			top[name] = tftypes.NewValue(ty, nil)
		}
	}
	return tfsdk.State{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}
