package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*wlanGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*wlanGroupResource)(nil)
	_ resource.ResourceWithImportState = (*wlanGroupResource)(nil)
)

func NewWLANGroupResource() resource.Resource { return &wlanGroupResource{} }

type wlanGroupResource struct {
	net *network.Client
}

type wlanGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	AttrNoDelete types.Bool   `tfsdk:"attr_no_delete"`
}

func (r *wlanGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wlan_group"
}

func (r *wlanGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi WLAN group (`wlangroup`, legacy `/rest`) — a named grouping of WLANs (the older " +
			"organizational unit that predates AP groups). Only `name` is configurable; the controller ships a " +
			"protected default group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group name.",
			},
			"attr_no_delete": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the controller protects this group from deletion (the built-in default " +
					"group). When `true`, `terraform destroy` removes the resource from state without deleting it.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *wlanGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *wlanGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wlanGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.WiFi().WLANGroups().Create(ctx, plan.toWLANGroup())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create WLAN group", err.Error())
		return
	}
	plan.fromWLANGroup(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wlanGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.net.WiFi().WLANGroups().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read WLAN group", err.Error())
		return
	}
	state.fromWLANGroup(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wlanGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wlanGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.WiFi().WLANGroups().Update(ctx, plan.ID.ValueString(), plan.toWLANGroup())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update WLAN group", err.Error())
		return
	}
	plan.fromWLANGroup(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wlanGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The controller-protected default group can't be deleted — drop it from
	// state rather than issuing a DELETE the controller would reject.
	if state.AttrNoDelete.ValueBool() {
		return
	}
	if err := r.net.WiFi().WLANGroups().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete WLAN group", err.Error())
	}
}

func (r *wlanGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *wlanGroupResourceModel) toWLANGroup() *wire.Wlangroup {
	return &wire.Wlangroup{
		UnderscoreId: strPtrOrNil(m.ID),
		Name:         strPtrOrNil(m.Name),
	}
}

func (m *wlanGroupResourceModel) fromWLANGroup(g *wire.Wlangroup) {
	m.ID = types.StringValue(deref(g.UnderscoreId))
	m.Name = types.StringValue(deref(g.Name))
	m.AttrNoDelete = types.BoolValue(g.AttrNoDelete != nil && *g.AttrNoDelete)
}
