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
	_ resource.Resource                = (*trafficRouteResource)(nil)
	_ resource.ResourceWithConfigure   = (*trafficRouteResource)(nil)
	_ resource.ResourceWithImportState = (*trafficRouteResource)(nil)
)

func NewTrafficRouteResource() resource.Resource { return &trafficRouteResource{} }

type trafficRouteResource struct {
	client unifi.Client
}

type trafficRouteResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Description       types.String `tfsdk:"description"`
	MatchingTarget    types.String `tfsdk:"matching_target"`
	Domains           types.Set    `tfsdk:"domains"`
	IPAddresses       types.Set    `tfsdk:"ip_addresses"`
	IPRanges          types.Set    `tfsdk:"ip_ranges"`
	Regions           types.Set    `tfsdk:"regions"`
	NetworkID         types.String `tfsdk:"network_id"`
	TargetDevices     types.List   `tfsdk:"target_devices"`
	KillSwitchEnabled types.Bool   `tfsdk:"kill_switch_enabled"`
}

func (r *trafficRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_traffic_route"
}

func (r *trafficRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi traffic route (v2 `trafficroutes`) — policy routing: send traffic matched by " +
			"destination (domains, IPs, regions, or the whole internet) out a specific interface/VPN (`network_id`) " +
			"for a set of target devices, with an optional kill switch.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the route is active. Defaults to `true`.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description.",
			},
			"matching_target": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What the route matches on: `INTERNET` (all), `DOMAIN`, `IP`, or `REGION`.",
				Validators:          []validator.String{stringvalidator.OneOf("INTERNET", "DOMAIN", "IP", "REGION")},
			},
			"domains":      stringSetAttr("Domains to match when `matching_target = DOMAIN`."),
			"ip_addresses": stringSetAttr("IP addresses to match when `matching_target = IP`."),
			"ip_ranges":    stringSetAttr("IP ranges/CIDRs to match when `matching_target = IP`."),
			"regions":      stringSetAttr("Region codes to match when `matching_target = REGION`."),
			"network_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The interface or VPN network the matched traffic is routed through (the route's " +
					"egress).",
			},
			"target_devices": targetDevicesSchema(),
			"kill_switch_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "When the egress is down, drop matched traffic instead of falling back to the " +
					"default route. Defaults to `false`.",
			},
		},
	}
}

func (r *trafficRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *trafficRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trafficRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateTrafficRoute(ctx, plan.toTrafficRoute(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create traffic route", err.Error())
		return
	}
	plan.fromTrafficRoute(ctx, created, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *trafficRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trafficRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	route, err := r.client.GetTrafficRoute(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read traffic route", err.Error())
		return
	}
	state.fromTrafficRoute(ctx, route, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *trafficRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trafficRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateTrafficRoute(ctx, plan.toTrafficRoute(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update traffic route", err.Error())
		return
	}
	plan.fromTrafficRoute(ctx, updated, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *trafficRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trafficRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTrafficRoute(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete traffic route", err.Error())
	}
}

func (r *trafficRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *trafficRouteResourceModel) toTrafficRoute(ctx context.Context, diags *diag.Diagnostics) *unifi.TrafficRoute {
	return &unifi.TrafficRoute{
		ID:                m.ID.ValueString(),
		Enabled:           boolPtr(m.Enabled),
		Description:       strOrEmpty(m.Description),
		MatchingTarget:    strOrEmpty(m.MatchingTarget),
		Domains:           stringSetToSlice(ctx, m.Domains, diags),
		IPAddresses:       stringSetToSlice(ctx, m.IPAddresses, diags),
		IPRanges:          stringSetToSlice(ctx, m.IPRanges, diags),
		Regions:           stringSetToSlice(ctx, m.Regions, diags),
		NetworkID:         strOrEmpty(m.NetworkID),
		TargetDevices:     listToTargetDevices(ctx, m.TargetDevices, diags),
		KillSwitchEnabled: boolPtr(m.KillSwitchEnabled),
	}
}

func (m *trafficRouteResourceModel) fromTrafficRoute(ctx context.Context, t *unifi.TrafficRoute, diags *diag.Diagnostics) {
	m.ID = types.StringValue(t.ID)
	m.Enabled = boolToState(t.Enabled)
	m.Description = strToStateOptional(t.Description)
	m.MatchingTarget = strToStateOptional(t.MatchingTarget)
	m.Domains = sliceToStringSet(t.Domains)
	m.IPAddresses = sliceToStringSet(t.IPAddresses)
	m.IPRanges = sliceToStringSet(t.IPRanges)
	m.Regions = sliceToStringSet(t.Regions)
	m.NetworkID = strToStateOptional(t.NetworkID)
	m.TargetDevices = targetDevicesToList(ctx, t.TargetDevices, diags)
	m.KillSwitchEnabled = boolToState(t.KillSwitchEnabled)
}
