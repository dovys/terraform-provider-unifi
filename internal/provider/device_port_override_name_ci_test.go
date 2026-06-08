package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// regression test for #101 — renaming a port via port_overrides.<n>.name failed
// with "Provider produced inconsistent result after apply: .port_overrides["1"].name:
// was "uplink-port", but now "lan1-port"". The /rest/device PUT lands the
// rename, but the /stat/device list the read maps from can lag a refresh and echo
// the *pre-rename* name. Trusting that stale read for a known, configured value
// contradicts the plan, so the apply errors and every later plan re-proposes the
// rename forever. The configured name (a known plan value) must round-trip into
// apply-time state regardless of what the lagging read echoes.

// renameLaggedDeviceJSON is a /stat/device list where port 1 is echoed with the
// OLD name ("lan1-port") — the eventual-consistency lag from #101. The rename
// to "trunk-rename" was accepted by /rest/device but the read hasn't caught up.
const renameLaggedDeviceJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"UCGMAX","type":"udm","adopted":true,"port_overrides":[
	{"port_idx":1,"forward":"customize","name":"lan1-port","native_networkconf_id":"netA"}
]}]}`

// renamePlan builds a plan that renames port 1 to want (a known value). Every
// other override leaf is unknown — the shape the framework hands the provider for
// an unconfigured Optional+Computed nested attribute. id "" leaves id null (create).
func renamePlan(t *testing.T, sch rschema.Schema, id, want string) tfsdk.Plan {
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
	ovals["name"] = tftypes.NewValue(tftypes.String, want)
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"1": po})

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

// portOverrideName reads port_overrides[key].name from a model, failing if the
// port is absent.
func portOverrideName(t *testing.T, m deviceResourceModel, key string) types.String {
	t.Helper()
	el, ok := m.PortOverrides.Elements()[key]
	if !ok {
		t.Fatalf("port %q absent from post-apply state — a declared port must survive", key)
	}
	return el.(types.Object).Attributes()["name"].(types.String)
}

// TestDevicePortOverrideRename_RoundTripsWhenReadLags is the specific regression
// for #101: the user renames a port (a known plan value), the controller accepts
// the write, but the /stat/device read still echoes the pre-rename name. The
// configured name must round-trip into apply-time state, not the stale read.
func TestDevicePortOverrideRename_RoundTripsWhenReadLags(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResourceServing(t, renameLaggedDeviceJSON)
	sch := deviceResourceSchema(t)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: renamePlan(t, sch, "", "trunk-rename")}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)

	got := portOverrideName(t, m, "1")
	if got.IsNull() || got.ValueString() != "trunk-rename" {
		t.Errorf("port_overrides[1].name = %v, want \"trunk-rename\" — a configured rename must round-trip even when the read echoes the old name (#101)", got)
	}
}

// TestDevicePortOverrideRename_UpdateRoundTripsWhenReadLags is the update path of
// #101: prior state holds the old name, the plan renames it, and the lagging read
// still echoes the old name. Post-apply state must hold the configured new name.
func TestDevicePortOverrideRename_UpdateRoundTripsWhenReadLags(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResourceServing(t, renameLaggedDeviceJSON)
	sch := deviceResourceSchema(t)

	prior := renameStateOldName(t, sch, "lan1-port")
	plan := renamePlan(t, sch, "d1", "trunk-rename")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: prior}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)

	got := portOverrideName(t, m, "1")
	if got.IsNull() || got.ValueString() != "trunk-rename" {
		t.Errorf("port_overrides[1].name = %v, want \"trunk-rename\" (#101)", got)
	}
}

// renameStateOldName builds prior state where port 1 carries name — the
// pre-rename shape. name is a parameter so a test can diverge prior state from
// what the controller serves (the Read test relies on that divergence).
func renameStateOldName(t *testing.T, sch rschema.Schema, name string) tfsdk.State {
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
	ovals["name"] = tftypes.NewValue(tftypes.String, name)
	ovals["forward"] = tftypes.NewValue(tftypes.String, "customize")
	ovals["native_network_id"] = tftypes.NewValue(tftypes.String, "netA")
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"1": po})

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

// TestDevicePortOverrideRead_KeepsControllerName guards the Read path: with no
// fallback (Read passes nil), the provider must reflect the controller's
// authoritative name, not invent one. This is the counterpart to the round-trip
// restore — the restore only fires on apply (fallback set), never on Read, so
// genuine controller-side drift still reconciles.
//
// Prior state deliberately diverges from what the controller serves: state holds
// "stale-prior-name" while renameLaggedDeviceJSON serves "lan1-port". Asserting
// the controller's value survives proves Read reflects the controller — not the
// prior state, and not a fallback restore. If the two matched, the assertion
// couldn't tell "kept controller value" from "kept prior state".
func TestDevicePortOverrideRead_KeepsControllerName(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResourceServing(t, renameLaggedDeviceJSON)
	sch := deviceResourceSchema(t)

	state := renameStateOldName(t, sch, "stale-prior-name")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	// Seed prior state so Read knows which port to reconcile.
	resp.State.Raw = state.Raw
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)

	got := portOverrideName(t, m, "1")
	if got.ValueString() != "lan1-port" {
		t.Errorf("Read port_overrides[1].name = %q, want controller value \"lan1-port\" — Read must reflect the controller, not prior state %q or a fallback", got.ValueString(), "stale-prior-name")
	}
}
