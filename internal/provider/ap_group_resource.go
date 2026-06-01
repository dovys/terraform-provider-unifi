package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*apGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*apGroupResource)(nil)
	_ resource.ResourceWithImportState = (*apGroupResource)(nil)
)

func NewAPGroupResource() resource.Resource { return &apGroupResource{} }

type apGroupResource struct {
	client unifi.Client
}

type apGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`             // controller _id; import key (computed)
	Name         types.String `tfsdk:"name"`           // display name (required)
	DeviceMACs   types.Set    `tfsdk:"device_macs"`    // APs in the group (MAC addresses)
	AttrNoDelete types.Bool   `tfsdk:"attr_no_delete"` // controller-protected (the default group); computed
}

func (r *apGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_ap_group"
}

func (r *apGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi AP group — a named set of access points a `unifi_network_wlan` can broadcast on " +
			"(v2 `/apgroups`). The controller ships a protected default group covering every adopted AP.",
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
			"device_macs": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "MAC addresses of the access points in this group. Empty groups are allowed.",
			},
			"attr_no_delete": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the controller protects this group from deletion (the built-in default " +
					"group covering every adopted AP). When `true`, `terraform destroy` removes the resource from " +
					"state without deleting the group on the controller.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *apGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *apGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateAPGroup(ctx, plan.toAPGroup(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create AP group", err.Error())
		return
	}
	plan.fromAPGroup(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetAPGroup(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read AP group", err.Error())
		return
	}
	state.fromAPGroup(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan apGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateAPGroup(ctx, plan.toAPGroup(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update AP group", err.Error())
		return
	}
	plan.fromAPGroup(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Safe by default: the controller-protected default group can't be deleted —
	// drop it from state and leave it on the controller rather than issuing a
	// DELETE the controller would reject.
	if state.AttrNoDelete.ValueBool() {
		return
	}
	if err := r.client.DeleteAPGroup(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete AP group", err.Error())
	}
}

func (r *apGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *apGroupResourceModel) toAPGroup(ctx context.Context, diags *diag.Diagnostics) *unifi.APGroup {
	macs := stringSetToSlice(ctx, m.DeviceMACs, diags)
	if macs == nil {
		macs = []string{} // device_macs is non-omitempty: send an explicit empty array
	}
	return &unifi.APGroup{
		ID:         m.ID.ValueString(),
		Name:       m.Name.ValueString(),
		DeviceMACs: macs,
	}
}

func (m *apGroupResourceModel) fromAPGroup(g *unifi.APGroup) {
	m.ID = types.StringValue(g.ID)
	m.Name = types.StringValue(g.Name)
	m.DeviceMACs = sliceToStringSet(g.DeviceMACs)
	m.AttrNoDelete = types.BoolValue(g.AttrNoDelete != nil && *g.AttrNoDelete)
}
