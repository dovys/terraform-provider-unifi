package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

// stringSetAttr / int64SetAttr are small builders for the Optional element-typed
// set attributes used by the traffic resources' matching criteria.
func stringSetAttr(desc string) schema.SetAttribute {
	return schema.SetAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: desc}
}

func int64SetAttr(desc string) schema.SetAttribute {
	return schema.SetAttribute{Optional: true, ElementType: types.Int64Type, MarkdownDescription: desc}
}

var (
	_ resource.Resource                = (*trafficRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*trafficRuleResource)(nil)
	_ resource.ResourceWithImportState = (*trafficRuleResource)(nil)
)

func NewTrafficRuleResource() resource.Resource { return &trafficRuleResource{} }

type trafficRuleResource struct {
	client unifi.Client
}

type trafficRuleResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Action         types.String `tfsdk:"action"`
	MatchingTarget types.String `tfsdk:"matching_target"`
	Description    types.String `tfsdk:"description"`
	Domains        types.Set    `tfsdk:"domains"`
	IPAddresses    types.Set    `tfsdk:"ip_addresses"`
	IPRanges       types.Set    `tfsdk:"ip_ranges"`
	Regions        types.Set    `tfsdk:"regions"`
	AppIDs         types.Set    `tfsdk:"app_ids"`
	AppCategoryIDs types.Set    `tfsdk:"app_category_ids"`
	NetworkIDs     types.Set    `tfsdk:"network_ids"`
	TargetDevices  types.List   `tfsdk:"target_devices"`
	BandwidthLimit types.Object `tfsdk:"bandwidth_limit"`
}

// trafficTargetDeviceObject mirrors one target_devices entry for List decoding.
// Shared with the traffic_route resource (same package).
type trafficTargetDeviceObject struct {
	Type      types.String `tfsdk:"type"`
	ClientMAC types.String `tfsdk:"client_mac"`
	NetworkID types.String `tfsdk:"network_id"`
}

var trafficTargetDeviceAttrTypes = map[string]attr.Type{
	"type":       types.StringType,
	"client_mac": types.StringType,
	"network_id": types.StringType,
}

type bandwidthLimitObject struct {
	Enabled           types.Bool  `tfsdk:"enabled"`
	DownloadLimitKbps types.Int64 `tfsdk:"download_limit_kbps"`
	UploadLimitKbps   types.Int64 `tfsdk:"upload_limit_kbps"`
}

var bandwidthLimitAttrTypes = map[string]attr.Type{
	"enabled":             types.BoolType,
	"download_limit_kbps": types.Int64Type,
	"upload_limit_kbps":   types.Int64Type,
}

func (r *trafficRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_traffic_rule"
}

// targetDevicesSchema is the shared nested-attribute schema for target_devices.
func targetDevicesSchema() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		MarkdownDescription: "Which devices the rule applies to. Each entry is all clients, a specific client " +
			"(by MAC), or a whole network.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Target kind: `ALL_CLIENTS`, `CLIENT` (set `client_mac`), or `NETWORK` (set `network_id`).",
					Validators:          []validator.String{stringvalidator.OneOf("ALL_CLIENTS", "CLIENT", "NETWORK")},
				},
				"client_mac": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Client MAC, when `type = CLIENT`.",
				},
				"network_id": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Network id, when `type = NETWORK`.",
				},
			},
		},
	}
}

func (r *trafficRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi traffic rule (v2 `trafficrules`) — block, allow, or rate-limit traffic matched by " +
			"destination (the whole internet, domains, IPs, regions, or applications/categories) for a set of target " +
			"devices.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the rule is active. Defaults to `true`.",
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What to do with matched traffic: `BLOCK`, `ALLOW`, or `SPEED_LIMIT` (set `bandwidth_limit`).",
				Validators:          []validator.String{stringvalidator.OneOf("BLOCK", "ALLOW", "SPEED_LIMIT")},
			},
			"matching_target": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "What the rule matches on: `INTERNET` (all), `DOMAIN`, `IP`, `REGION`, `APP`, or " +
					"`APP_CATEGORY`. Populate the corresponding list below.",
				Validators: []validator.String{stringvalidator.OneOf("INTERNET", "DOMAIN", "IP", "REGION", "APP", "APP_CATEGORY")},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable description.",
			},
			"domains":          stringSetAttr("Domains to match when `matching_target = DOMAIN`."),
			"ip_addresses":     stringSetAttr("IP addresses to match when `matching_target = IP`."),
			"ip_ranges":        stringSetAttr("IP ranges/CIDRs to match when `matching_target = IP`."),
			"regions":          stringSetAttr("Region codes to match when `matching_target = REGION`."),
			"network_ids":      stringSetAttr("Network ids the rule is scoped to."),
			"app_ids":          int64SetAttr("Application ids to match when `matching_target = APP`."),
			"app_category_ids": int64SetAttr("Application-category ids to match when `matching_target = APP_CATEGORY`."),
			"target_devices":   targetDevicesSchema(),
			"bandwidth_limit": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Rate cap applied when `action = SPEED_LIMIT`.",
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Whether the limit is enforced.",
					},
					"download_limit_kbps": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Download cap in kbps.",
					},
					"upload_limit_kbps": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Upload cap in kbps.",
					},
				},
			},
		},
	}
}

func (r *trafficRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *trafficRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan trafficRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateTrafficRule(ctx, plan.toTrafficRule(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create traffic rule", err.Error())
		return
	}
	plan.fromTrafficRule(ctx, created, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *trafficRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state trafficRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.GetTrafficRule(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read traffic rule", err.Error())
		return
	}
	state.fromTrafficRule(ctx, rule, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *trafficRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan trafficRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateTrafficRule(ctx, plan.toTrafficRule(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update traffic rule", err.Error())
		return
	}
	plan.fromTrafficRule(ctx, updated, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *trafficRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state trafficRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTrafficRule(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete traffic rule", err.Error())
	}
}

func (r *trafficRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *trafficRuleResourceModel) toTrafficRule(ctx context.Context, diags *diag.Diagnostics) *unifi.TrafficRule {
	return &unifi.TrafficRule{
		ID:             m.ID.ValueString(),
		Enabled:        boolPtr(m.Enabled),
		Action:         strOrEmpty(m.Action),
		MatchingTarget: strOrEmpty(m.MatchingTarget),
		Description:    strOrEmpty(m.Description),
		Domains:        stringSetToSlice(ctx, m.Domains, diags),
		IPAddresses:    stringSetToSlice(ctx, m.IPAddresses, diags),
		IPRanges:       stringSetToSlice(ctx, m.IPRanges, diags),
		Regions:        stringSetToSlice(ctx, m.Regions, diags),
		AppIDs:         intSetToSlice(ctx, m.AppIDs, diags),
		AppCategoryIDs: intSetToSlice(ctx, m.AppCategoryIDs, diags),
		NetworkIDs:     stringSetToSlice(ctx, m.NetworkIDs, diags),
		TargetDevices:  listToTargetDevices(ctx, m.TargetDevices, diags),
		BandwidthLimit: objectToBandwidthLimit(ctx, m.BandwidthLimit, diags),
	}
}

func (m *trafficRuleResourceModel) fromTrafficRule(ctx context.Context, t *unifi.TrafficRule, diags *diag.Diagnostics) {
	m.ID = types.StringValue(t.ID)
	m.Enabled = boolToState(t.Enabled)
	m.Action = strToStateOptional(t.Action)
	m.MatchingTarget = strToStateOptional(t.MatchingTarget)
	m.Description = strToStateOptional(t.Description)
	m.Domains = sliceToStringSet(t.Domains)
	m.IPAddresses = sliceToStringSet(t.IPAddresses)
	m.IPRanges = sliceToStringSet(t.IPRanges)
	m.Regions = sliceToStringSet(t.Regions)
	m.AppIDs = sliceToIntSet(t.AppIDs)
	m.AppCategoryIDs = sliceToIntSet(t.AppCategoryIDs)
	m.NetworkIDs = sliceToStringSet(t.NetworkIDs)
	m.TargetDevices = targetDevicesToList(ctx, t.TargetDevices, diags)
	m.BandwidthLimit = bandwidthLimitToObject(ctx, t.BandwidthLimit, diags)
}

// --- shared target_devices conversion (used by traffic_rule and traffic_route) ---

func listToTargetDevices(ctx context.Context, l types.List, diags *diag.Diagnostics) []unifi.TrafficTargetDevice {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var objs []trafficTargetDeviceObject
	diags.Append(l.ElementsAs(ctx, &objs, false)...)
	out := make([]unifi.TrafficTargetDevice, 0, len(objs))
	for _, o := range objs {
		out = append(out, unifi.TrafficTargetDevice{
			Type:      o.Type.ValueString(),
			ClientMAC: strOrEmpty(o.ClientMAC),
			NetworkID: strOrEmpty(o.NetworkID),
		})
	}
	return out
}

func targetDevicesToList(ctx context.Context, devices []unifi.TrafficTargetDevice, diags *diag.Diagnostics) types.List {
	objType := types.ObjectType{AttrTypes: trafficTargetDeviceAttrTypes}
	if devices == nil {
		return types.ListNull(objType)
	}
	objs := make([]trafficTargetDeviceObject, 0, len(devices))
	for _, d := range devices {
		objs = append(objs, trafficTargetDeviceObject{
			Type:      types.StringValue(d.Type),
			ClientMAC: strToStateOptional(d.ClientMAC),
			NetworkID: strToStateOptional(d.NetworkID),
		})
	}
	l, d := types.ListValueFrom(ctx, objType, objs)
	diags.Append(d...)
	return l
}

func objectToBandwidthLimit(ctx context.Context, o types.Object, diags *diag.Diagnostics) *unifi.TrafficBandwidthLimit {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var obj bandwidthLimitObject
	diags.Append(o.As(ctx, &obj, basetypes.ObjectAsOptions{})...)
	return &unifi.TrafficBandwidthLimit{
		Enabled:           boolPtr(obj.Enabled),
		DownloadLimitKbps: intPtr(obj.DownloadLimitKbps),
		UploadLimitKbps:   intPtr(obj.UploadLimitKbps),
	}
}

func bandwidthLimitToObject(ctx context.Context, b *unifi.TrafficBandwidthLimit, diags *diag.Diagnostics) types.Object {
	objType := types.ObjectType{AttrTypes: bandwidthLimitAttrTypes}
	if b == nil {
		return types.ObjectNull(bandwidthLimitAttrTypes)
	}
	obj := bandwidthLimitObject{
		Enabled:           boolToState(b.Enabled),
		DownloadLimitKbps: intToState(b.DownloadLimitKbps),
		UploadLimitKbps:   intToState(b.UploadLimitKbps),
	}
	o, d := types.ObjectValueFrom(ctx, objType.AttrTypes, obj)
	diags.Append(d...)
	return o
}
