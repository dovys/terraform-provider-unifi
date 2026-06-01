package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*portProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*portProfileResource)(nil)
	_ resource.ResourceWithImportState = (*portProfileResource)(nil)
)

func NewPortProfileResource() resource.Resource { return &portProfileResource{} }

type portProfileResource struct {
	client unifi.Client
}

// portProfileResourceModel covers the configurable portconf surface. Match the
// established Optional+Computed pattern so unset fields track the controller.
type portProfileResourceModel struct {
	ID   types.String `tfsdk:"id"`   // controller _id; import key (computed)
	Name types.String `tfsdk:"name"` // profile name (required)

	Forward            types.String `tfsdk:"forward"`              // VLAN behavior
	NativeNetworkID    types.String `tfsdk:"native_network_id"`    // untagged/native network
	TaggedVLANMgmt     types.String `tfsdk:"tagged_vlan_mgmt"`     // tagged-VLAN handling
	ExcludedNetworkIDs types.Set    `tfsdk:"excluded_network_ids"` // tagged networks to exclude
	VoiceNetworkID     types.String `tfsdk:"voice_network_id"`     // voice VLAN

	POEMode types.String `tfsdk:"poe_mode"` // PoE behavior

	Autoneg    types.Bool   `tfsdk:"autoneg"`
	Speed      types.Int64  `tfsdk:"speed"`
	FullDuplex types.Bool   `tfsdk:"full_duplex"`
	OpMode     types.String `tfsdk:"op_mode"`

	PortSecurityEnabled      types.Bool   `tfsdk:"port_security_enabled"`
	PortSecurityMACAddresses types.Set    `tfsdk:"port_security_mac_addresses"`
	Dot1XCtrl                types.String `tfsdk:"dot1x_ctrl"`
	Dot1XIdleTimeout         types.Int64  `tfsdk:"dot1x_idle_timeout"`
	Isolation                types.Bool   `tfsdk:"isolation"`
	EgressRateLimitEnabled   types.Bool   `tfsdk:"egress_rate_limit_enabled"`
	EgressRateLimitKbps      types.Int64  `tfsdk:"egress_rate_limit_kbps"`

	STPPortMode          types.Bool   `tfsdk:"stp_port_mode"`
	STPBPDUGuard         types.Bool   `tfsdk:"stp_bpdu_guard"`
	STPEdgeState         types.String `tfsdk:"stp_edge_state"`
	LLDPMEDEnabled       types.Bool   `tfsdk:"lldpmed_enabled"`
	PortKeepaliveEnabled types.Bool   `tfsdk:"port_keepalive_enabled"`
	MulticastRouterMode  types.String `tfsdk:"multicast_router_mode"`
}

func (r *portProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_port_profile"
}

func (r *portProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi switch-port profile (`portconf`) — a reusable port configuration (VLAN behavior, " +
			"PoE, port security, STP, 802.1X, rate limiting) you assign to switch ports instead of editing each one. " +
			"Port profiles predate the controller's v2 API and are served on the legacy `/rest/portconf` surface (there " +
			"is no v2 equivalent on current firmware).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "Profile name (shown in the UniFi Port Manager)."},

			"forward": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "VLAN forwarding mode for the port: `all` (trunk — carries every VLAN), `native` " +
					"(access port — the untagged `native_network_id` only), `customize` (trunk carrying the native VLAN " +
					"plus a chosen set of tagged VLANs, refined by `tagged_vlan_mgmt`/`excluded_network_ids`), or " +
					"`disabled` (the port is administratively shut).",
				Validators: []validator.String{stringvalidator.OneOf("all", "native", "customize", "disabled")},
			},
			"native_network_id": optStr("Native (untagged) network (`unifi_network_network` id): frames arriving without a " +
				"VLAN tag are placed on this network, and this network egresses untagged. On an access port it's the port's VLAN."),
			"tagged_vlan_mgmt": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Which tagged VLANs the port trunks: `auto` (Allow All — every VLAN created in UniFi), " +
					"`block_all` (Access Port — only the untagged `native_network_id` passes), or `custom` (trunk all VLANs " +
					"*except* those listed in `excluded_network_ids`).",
				Validators: []validator.String{stringvalidator.OneOf("auto", "block_all", "custom")},
			},
			"excluded_network_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tagged networks (`unifi_network_network` ids) to **block** on the trunk when `tagged_vlan_mgmt = custom`; every other VLAN is allowed through.",
			},
			"voice_network_id": optStr("Voice VLAN network (`unifi_network_network` id): a second tagged VLAN for VoIP phones, " +
				"advertised to them via LLDP-MED so voice traffic is separated from the data VLAN."),

			// No OneOf: the accepted PoE strings depend on the switch model/generation
			// (older switches expose passive 24V, newer ones don't; passthrough only on
			// passthrough-capable models) and aren't hardware-confirmed (TODO.md), so a
			// validator here would risk rejecting a value a real switch accepts.
			"poe_mode": optStr("How the port supplies Power over Ethernet: `auto` (negotiate active PoE — 802.3af/at/bt — " +
				"per the connected device), `pasv24` (always-on passive 24V, for legacy gear; only on switches that support " +
				"it), `passthrough` (re-output the switch's own input PoE, on passthrough-capable models), or `off` (no power). " +
				"Available values depend on the switch model."),

			"autoneg":     optBool("Auto-negotiate link speed and duplex (the normal default). Set `false` to force `speed`/`full_duplex`."),
			"speed":       optInt("Forced link speed in Mbps (e.g. `10`, `100`, `1000`) when `autoneg = false`."),
			"full_duplex": optBool("Force full-duplex (rather than half-duplex) when `autoneg = false`."),
			"op_mode": optStr("Port operation mode: `switch` (normal switching, the default), `mirror` (SPAN — copy traffic " +
				"to this port for packet capture), or `aggregate` (LACP link aggregation across multiple ports)."),

			"port_security_enabled": optBool("Lock the port to a fixed MAC allow-list (`port_security_mac_addresses`); frames from other source MACs are dropped."),
			"port_security_mac_addresses": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "MAC addresses allowed on the port when `port_security_enabled = true`.",
			},
			"dot1x_ctrl": optStr("802.1X port access control: `force_authorized` (always open — the default), `auto` (require " +
				"RADIUS authentication before the port forwards), `force_unauthorized` (always blocked), `mac_based` (authenticate " +
				"each MAC individually, for devices that can't speak 802.1X), or `multi_host` (authenticate once, then allow other " +
				"devices — e.g. PCs behind a VoIP phone)."),
			"dot1x_idle_timeout":        optInt("Seconds of inactivity before an authenticated 802.1X session is cleared."),
			"isolation":                 optBool("Port isolation: isolated ports can't talk to each other (only to uplinks). Use for untrusted guest/IoT ports."),
			"egress_rate_limit_enabled": optBool("Cap the port's outbound (egress) throughput at `egress_rate_limit_kbps`."),
			"egress_rate_limit_kbps":    optInt("Egress (outbound) rate limit in kbps, applied when `egress_rate_limit_enabled = true`."),

			"stp_port_mode":          optBool("Include the port in Spanning Tree (STP/RSTP) loop prevention."),
			"stp_bpdu_guard":         optBool("BPDU guard: shut the port if it receives a spanning-tree BPDU — protects an edge/access port against a rogue switch or accidental loop."),
			"stp_edge_state":         optStr("Edge-port (PortFast) behavior — skip STP listening/learning and forward immediately on link-up: `auto`, `enabled`, or `disabled`."),
			"lldpmed_enabled":        optBool("Advertise LLDP-MED so VoIP phones auto-discover their voice VLAN and QoS policy."),
			"port_keepalive_enabled": optBool("Send link keep-alives on the port (helps some powered devices stay linked)."),
			"multicast_router_mode":  optStr("How the port treats multicast-router (IGMP/MLD querier) traffic for multicast snooping."),
		},
	}
}

func (r *portProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *portProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreatePortProfile(ctx, plan.toPortProfile(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create port profile", err.Error())
		return
	}
	plan.fromPortProfile(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetPortProfile(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read port profile", err.Error())
		return
	}
	state.fromPortProfile(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdatePortProfile(ctx, plan.toPortProfile(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update port profile", err.Error())
		return
	}
	plan.fromPortProfile(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePortProfile(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete port profile", err.Error())
	}
}

func (r *portProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *portProfileResourceModel) toPortProfile(ctx context.Context, diags *diag.Diagnostics) *unifi.PortProfile {
	p := &unifi.PortProfile{
		ID:                     m.ID.ValueString(),
		Name:                   m.Name.ValueString(),
		Forward:                strOrEmpty(m.Forward),
		NativeNetworkConfID:    strOrEmpty(m.NativeNetworkID),
		TaggedVLANMgmt:         strOrEmpty(m.TaggedVLANMgmt),
		VoiceNetworkConfID:     strOrEmpty(m.VoiceNetworkID),
		POEMode:                strOrEmpty(m.POEMode),
		Autoneg:                boolPtr(m.Autoneg),
		Speed:                  intPtr(m.Speed),
		FullDuplex:             boolPtr(m.FullDuplex),
		OpMode:                 strOrEmpty(m.OpMode),
		PortSecurityEnabled:    boolPtr(m.PortSecurityEnabled),
		Dot1XCtrl:              strOrEmpty(m.Dot1XCtrl),
		Dot1XIdleTimeout:       intPtr(m.Dot1XIdleTimeout),
		Isolation:              boolPtr(m.Isolation),
		EgressRateLimitEnabled: boolPtr(m.EgressRateLimitEnabled),
		EgressRateLimitKbps:    intPtr(m.EgressRateLimitKbps),
		STPPortMode:            boolPtr(m.STPPortMode),
		STPBPDUGuard:           boolPtr(m.STPBPDUGuard),
		STPEdgeState:           strOrEmpty(m.STPEdgeState),
		LLDPMEDEnabled:         boolPtr(m.LLDPMEDEnabled),
		PortKeepaliveOn:        boolPtr(m.PortKeepaliveEnabled),
		MulticastRouterMode:    strOrEmpty(m.MulticastRouterMode),
	}
	// These two lists are owned (non-omitempty on the wire): always send a concrete
	// slice so an explicit empty set clears the value instead of reading back null
	// (which would trip the post-apply consistency check). Mirrors firewall_group.
	if e := stringSetToSlice(ctx, m.ExcludedNetworkIDs, diags); e != nil {
		p.ExcludedNetworkConfIDs = e
	} else {
		p.ExcludedNetworkConfIDs = []string{}
	}
	if s := stringSetToSlice(ctx, m.PortSecurityMACAddresses, diags); s != nil {
		p.PortSecurityMACAddress = s
	} else {
		p.PortSecurityMACAddress = []string{}
	}
	return p
}

func (m *portProfileResourceModel) fromPortProfile(p *unifi.PortProfile) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Forward = strToStateOptional(p.Forward)
	m.NativeNetworkID = strToStateOptional(p.NativeNetworkConfID)
	m.TaggedVLANMgmt = strToStateOptional(p.TaggedVLANMgmt)
	m.ExcludedNetworkIDs = sliceToStringSet(p.ExcludedNetworkConfIDs)
	m.VoiceNetworkID = strToStateOptional(p.VoiceNetworkConfID)
	m.POEMode = strToStateOptional(p.POEMode)
	m.Autoneg = boolToState(p.Autoneg)
	m.Speed = intToState(p.Speed)
	m.FullDuplex = boolToState(p.FullDuplex)
	m.OpMode = strToStateOptional(p.OpMode)
	m.PortSecurityEnabled = boolToState(p.PortSecurityEnabled)
	m.PortSecurityMACAddresses = sliceToStringSet(p.PortSecurityMACAddress)
	m.Dot1XCtrl = strToStateOptional(p.Dot1XCtrl)
	m.Dot1XIdleTimeout = intToState(p.Dot1XIdleTimeout)
	m.Isolation = boolToState(p.Isolation)
	m.EgressRateLimitEnabled = boolToState(p.EgressRateLimitEnabled)
	m.EgressRateLimitKbps = intToState(p.EgressRateLimitKbps)
	m.STPPortMode = boolToState(p.STPPortMode)
	m.STPBPDUGuard = boolToState(p.STPBPDUGuard)
	m.STPEdgeState = strToStateOptional(p.STPEdgeState)
	m.LLDPMEDEnabled = boolToState(p.LLDPMEDEnabled)
	m.PortKeepaliveEnabled = boolToState(p.PortKeepaliveOn)
	m.MulticastRouterMode = strToStateOptional(p.MulticastRouterMode)
}
