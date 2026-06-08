package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*deviceResource)(nil)
	_ resource.ResourceWithConfigure   = (*deviceResource)(nil)
	_ resource.ResourceWithImportState = (*deviceResource)(nil)
)

func NewDeviceResource() resource.Resource { return &deviceResource{} }

type deviceResource struct {
	net *network.Client
}

type deviceResourceModel struct {
	ID  types.String `tfsdk:"id"`  // controller _id (computed)
	MAC types.String `tfsdk:"mac"` // natural key (required, immutable)

	Name                       types.String `tfsdk:"name"`
	LEDOverride                types.String `tfsdk:"led_override"`
	LEDOverrideColor           types.String `tfsdk:"led_override_color"`
	LEDOverrideColorBrightness types.Int64  `tfsdk:"led_override_color_brightness"`
	MgmtNetworkID              types.String `tfsdk:"mgmt_network_id"`

	PortOverrides   types.Map  `tfsdk:"port_overrides"` // map[port number] => override object
	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	// Computed telemetry.
	Model   types.String `tfsdk:"model"`
	Type    types.String `tfsdk:"type"`
	Version types.String `tfsdk:"version"`
	IP      types.String `tfsdk:"ip"`
	State   types.Int64  `tfsdk:"state"`
	Adopted types.Bool   `tfsdk:"adopted"`
}

func (r *deviceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_device"
}

func (r *deviceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi network device (access point, switch, or gateway) identified by MAC. The single " +
			"resource adopts the device if it's pending, then manages its config (name, LED, management network, and " +
			"per-port `port_overrides`) and exposes read-only telemetry. By default `terraform destroy` reverts the " +
			"Terraform-managed port overrides and drops the device from state; set `forget_on_destroy = true` to instead " +
			"forget (unadopt) the device. **Adoption and config-write are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Device MAC address (the natural key). The device must be visible to the controller " +
					"(adopted, or pending adoption). Immutable — changing it replaces the resource.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Device display name.",
			},
			"led_override": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Status-LED behavior: `on`, `off`, or `default` (follow the site setting).",
			},
			"led_override_color": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "LED color as a hex string (when the model supports a colored LED).",
			},
			"led_override_color_brightness": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "LED brightness (0–100), when the model supports it.",
			},
			"mgmt_network_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The device's **management VLAN** (`unifi_network_network` id). The device 802.1Q-tags its own " +
					"control-plane traffic (inform, adoption) with this network's VLAN, so it reaches the controller and gets " +
					"its IP on that subnet **regardless of the upstream switch port's untagged/native VLAN** — an AP on a port " +
					"whose `native_network_id` is `iot` still lands on the management subnet. Distinct from `config_network` " +
					"(the device's IP-addressing mode: dhcp/static), which this provider does not model.",
			},
			"port_overrides": schema.MapNestedAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Per-port configuration, keyed by physical port number (e.g. `\"1\"`). Terraform manages " +
					"only the ports listed here: each is merged onto the controller's existing override (unmodeled keys are " +
					"preserved), undeclared ports are left untouched, and removing a port reverts it to defaults. **On import, " +
					"every port with a live override is captured** so `terraform plan -generate-config-out` reflects the switch — " +
					"keep the ports you want to manage in config (dropping one reverts it, as does `terraform destroy`).",
				NestedObject: schema.NestedAttributeObject{Attributes: portOverrideSchemaAttrs(false)},
			},
			"forget_on_destroy": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "When `true`, `terraform destroy` forgets (unadopts) the device instead of just " +
					"reverting the managed port overrides. Defaults to `false`. Guard production devices with `prevent_destroy`.",
			},

			"model":   schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model code."},
			"type":    schema.StringAttribute{Computed: true, MarkdownDescription: "Device type (`uap`, `usw`, `ugw`, `udm`, …)."},
			"version": schema.StringAttribute{Computed: true, MarkdownDescription: "Running firmware version."},
			"ip":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP."},
			"state":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Controller state code (`1` = connected)."},
			"adopted": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the device is adopted by this controller."},
		},
	}
}

func (r *deviceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *deviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mac := strings.ToLower(strings.TrimSpace(plan.MAC.ValueString()))
	dev, err := r.net.Devices().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Device not found",
				fmt.Sprintf("No device with MAC %q is visible to the controller. The device must be adopted or pending "+
					"adoption (broadcasting) before Terraform can manage it.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up device", err.Error())
		return
	}

	// Adopt if the controller sees the device but hasn't adopted it yet.
	if dev.Adopted == nil || !*dev.Adopted {
		if err := r.net.Devices().Adopt(ctx, mac); err != nil {
			resp.Diagnostics.AddError("Failed to adopt device", err.Error())
			return
		}
		if re, err := r.net.Devices().GetByMAC(ctx, mac); err == nil {
			dev = re
		}
	}

	r.applyConfig(ctx, &plan, deref(dev.UnderscoreId), nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dev, err := r.net.Devices().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read device", err.Error())
		return
	}
	// Reconcile only the ports Terraform tracks (prior-state keys); never leak
	// the controller's other ports into state. nil fallback: a tracked port the
	// controller lost is dropped (drift the next plan reconciles).
	state.fromDevice(ctx, dev, portKeys(ctx, state.PortOverrides, &resp.Diagnostics), nil, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *deviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state deviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Ports owned this cycle = config ∪ prior state, so a port dropped from config
	// gets reverted rather than orphaned.
	priorKeys := portKeys(ctx, state.PortOverrides, &resp.Diagnostics)
	r.applyConfig(ctx, &plan, state.ID.ValueString(), priorKeys, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.net.Devices().Forget(ctx, state.MAC.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to forget device", err.Error())
		}
		return
	}
	// Revert the Terraform-managed port overrides (drop them); leave telemetry and
	// scalar overrides as last-applied. No-op when no ports were managed.
	priorKeys := portKeys(ctx, state.PortOverrides, &resp.Diagnostics)
	if len(priorKeys) == 0 {
		return
	}
	// Empty config + the prior owned ports → the merge drops them (reverts).
	if _, err := r.net.Devices().UpdateConfig(ctx, state.ID.ValueString(), network.DeviceConfig{}, priorKeys); err != nil && !isNotFoundErr(err) {
		resp.Diagnostics.AddError("Failed to revert device port overrides", err.Error())
	}
}

func (r *deviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	// Accept either a controller _id or a MAC (containing ':').
	var dev *wire.Device
	var err error
	if strings.Contains(id, ":") {
		dev, err = r.net.Devices().GetByMAC(ctx, id)
	} else {
		dev, err = r.net.Devices().Get(ctx, id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no device for %q: %s", id, err))
		return
	}
	// Capture the whole device — including *every* live port override — so
	// `plan -generate-config-out` reflects the switch's real per-port config
	// instead of an empty map. Every captured port becomes Terraform-managed;
	// keep the ones you want in config (dropping a port reverts it, as does
	// destroy). MAC is null on the fresh model, so fromDevice fills it.
	var state deviceResourceModel
	state.fromDevice(ctx, dev, allPortKeys(dev, &resp.Diagnostics), nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applyConfig pushes the plan's owned config to the device and refreshes the
// model in place (telemetry + the planned port overrides). ownedPortIdx is the
// prior-state port set (nil on create).
func (r *deviceResource) applyConfig(ctx context.Context, plan *deviceResourceModel, id string, priorKeys []int, diags *diag.Diagnostics) {
	desired, planKeys := desiredPortOverrides(ctx, plan.PortOverrides, diags)
	if diags.HasError() {
		return
	}
	owned := unionInts(priorKeys, planKeys)

	cfg := network.DeviceConfig{
		Name:                       strOrEmpty(plan.Name),
		LEDOverride:                strOrEmpty(plan.LEDOverride),
		LEDOverrideColor:           strOrEmpty(plan.LEDOverrideColor),
		LEDOverrideColorBrightness: intPtr(plan.LEDOverrideColorBrightness),
		MgmtNetworkID:              strOrEmpty(plan.MgmtNetworkID),
		PortOverrides:              desired,
	}

	updated, err := r.net.Devices().UpdateConfig(ctx, id, cfg, owned)
	if err != nil {
		diags.AddError("Failed to apply device config", err.Error())
		return
	}
	plan.fromDevice(ctx, updated, planKeys, desired, diags)
}

// fromDevice maps a wire Device onto the model. keys selects which ports to
// surface in port_overrides (the Terraform-managed set); others are ignored.
// fallback (the just-written desired overrides, nil on Read) keeps a declared
// port in state when the controller hasn't echoed it back yet.
func (m *deviceResourceModel) fromDevice(ctx context.Context, d *wire.Device, keys []int, fallback []network.PortOverride, diags *diag.Diagnostics) {
	m.ID = types.StringValue(deref(d.UnderscoreId))
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(deref(d.Mac)) // import path; otherwise keep the configured value
	}
	m.Name = strToStateOptional(deref(d.Name))
	m.LEDOverride = strToStateOptional(string(derefEnum(d.LedOverride)))
	m.LEDOverrideColor = strToStateOptional(deref(d.LedOverrideColor))
	m.LEDOverrideColorBrightness = intToState(d.LedOverrideColorBrightness)
	m.MgmtNetworkID = strToStateOptional(deref(d.MgmtNetworkId))
	m.Model = types.StringValue(deref(d.Model))
	m.Type = types.StringValue(deref(d.Type))
	m.Version = types.StringValue(deref(d.Version))
	m.IP = types.StringValue(deref(d.Ip))
	m.State = intToState(d.State)
	m.Adopted = boolToState(d.Adopted)

	overrides, err := network.PortOverridesOf(d)
	if err != nil {
		diags.AddError("Failed to decode device port overrides", err.Error())
		return
	}
	m.PortOverrides = buildPortOverridesMap(ctx, overrides, keys, fallback, diags)
}

// desiredPortOverrides decodes the config map into client port overrides and
// returns them with the sorted list of port numbers.
func desiredPortOverrides(ctx context.Context, m types.Map, diags *diag.Diagnostics) ([]network.PortOverride, []int) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	elems := map[string]portOverrideObject{}
	diags.Append(m.ElementsAs(ctx, &elems, false)...)
	if diags.HasError() {
		return nil, nil
	}
	out := make([]network.PortOverride, 0, len(elems))
	keys := make([]int, 0, len(elems))
	for k, o := range elems {
		idx, err := strconv.Atoi(k)
		if err != nil {
			diags.AddError("Invalid port number", fmt.Sprintf("port_overrides key %q must be an integer port number", k))
			continue
		}
		out = append(out, portOverrideToNet(ctx, idx, o, diags))
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	return out, keys
}

// buildPortOverridesMap builds the framework map for the given (managed) ports
// from the device's current overrides. fallback carries the just-written desired
// overrides (nil on Read) and serves two purposes for a managed port:
//   - presence: a declared port the authoritative document omits entirely (the
//     controller hasn't echoed a freshly-written override yet) is sourced from
//     fallback, so its key doesn't vanish and trip the post-apply consistency
//     check;
//   - binding round-trip: when the document echoes the port but drops the
//     port-profile binding (portconf_id is reflected in /rest/device but not
//     always the /stat/device list the read uses), the configured port_profile_id
//     is restored from fallback so a known, user-set value round-trips instead of
//     reading back null (which would be "inconsistent result after apply").
//
// On Read fallback is nil, so a managed port the controller genuinely lost is
// dropped (drift the next plan reconciles).
func buildPortOverridesMap(ctx context.Context, overrides []network.PortOverride, keys []int, fallback []network.PortOverride, diags *diag.Diagnostics) types.Map {
	objType := types.ObjectType{AttrTypes: portOverrideAttrTypes}
	if len(keys) == 0 {
		return types.MapValueMust(objType, map[string]attr.Value{})
	}
	byIdx := make(map[int]network.PortOverride, len(overrides))
	for _, po := range overrides {
		byIdx[po.PortIdx] = po
	}
	fbIdx := make(map[int]network.PortOverride, len(fallback))
	for _, po := range fallback {
		fbIdx[po.PortIdx] = po
	}
	elems := map[string]attr.Value{}
	for _, k := range keys {
		po, ok := byIdx[k]
		if !ok {
			if po, ok = fbIdx[k]; !ok {
				continue
			}
		} else if fb, fbOK := fbIdx[k]; fbOK && po.PortconfID == "" && fb.PortconfID != "" {
			// Echoed override but the document omitted the binding the user set;
			// restore the configured portconf_id so it round-trips.
			po.PortconfID = fb.PortconfID
		}
		obj, d := types.ObjectValueFrom(ctx, portOverrideAttrTypes, portOverrideFromNet(po))
		diags.Append(d...)
		elems[strconv.Itoa(k)] = obj
	}
	return types.MapValueMust(objType, elems)
}

// allPortKeys returns every port index that has a live override on the device,
// sorted. Import captures all of them so the imported state mirrors the switch.
func allPortKeys(d *wire.Device, diags *diag.Diagnostics) []int {
	overrides, err := network.PortOverridesOf(d)
	if err != nil {
		diags.AddError("Failed to decode device port overrides", err.Error())
		return nil
	}
	keys := make([]int, 0, len(overrides))
	for _, po := range overrides {
		keys = append(keys, po.PortIdx)
	}
	sort.Ints(keys)
	return keys
}

// portKeys returns the integer port numbers present in a port_overrides map.
func portKeys(ctx context.Context, m types.Map, diags *diag.Diagnostics) []int {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	keys := make([]int, 0, len(m.Elements()))
	for k := range m.Elements() {
		idx, err := strconv.Atoi(k)
		if err != nil {
			diags.AddError("Invalid port number", fmt.Sprintf("port_overrides key %q must be an integer", k))
			continue
		}
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	return keys
}

func unionInts(a, b []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(a)+len(b))
	for _, v := range append(append([]int{}, a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}
