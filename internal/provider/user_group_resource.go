package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*userGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*userGroupResource)(nil)
	_ resource.ResourceWithImportState = (*userGroupResource)(nil)
)

func NewUserGroupResource() resource.Resource { return &userGroupResource{} }

type userGroupResource struct {
	client unifi.Client
}

type userGroupResourceModel struct {
	ID             types.String `tfsdk:"id"`                // controller _id; import key (computed)
	Name           types.String `tfsdk:"name"`              // display name (required)
	QOSRateMaxUp   types.Int64  `tfsdk:"qos_rate_max_up"`   // upload cap (kbps); unset = unlimited
	QOSRateMaxDown types.Int64  `tfsdk:"qos_rate_max_down"` // download cap (kbps); unset = unlimited
	AttrNoDelete   types.Bool   `tfsdk:"attr_no_delete"`    // controller-protected (the default group); computed
}

func (r *userGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_user_group"
}

func (r *userGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi user group — a bandwidth/QoS group (legacy `/rest/usergroup`) that caps the " +
			"up/down rate of the clients assigned to it. The controller ships a protected default group.",
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
			"qos_rate_max_up": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Maximum upload rate in **kbps**. Omit (or set to a non-positive value, " +
					"the controller's unlimited sentinel) for no limit.",
				Validators: []validator.Int64{int64validator.AtLeast(-1)},
			},
			"qos_rate_max_down": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Maximum download rate in **kbps**. Omit (or set to a non-positive value, " +
					"the controller's unlimited sentinel) for no limit.",
				Validators: []validator.Int64{int64validator.AtLeast(-1)},
			},
			"attr_no_delete": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the controller protects this group from deletion (the built-in default " +
					"group). When `true`, `terraform destroy` removes the resource from state without deleting the " +
					"group on the controller.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *userGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *userGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateUserGroup(ctx, plan.toUserGroup())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user group", err.Error())
		return
	}
	plan.fromUserGroup(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetUserGroup(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user group", err.Error())
		return
	}
	state.fromUserGroup(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateUserGroup(ctx, plan.toUserGroup())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update user group", err.Error())
		return
	}
	plan.fromUserGroup(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Safe by default: the controller-protected default group can't be deleted —
	// drop it from state rather than issuing a DELETE the controller would reject.
	if state.AttrNoDelete.ValueBool() {
		return
	}
	if err := r.client.DeleteUserGroup(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete user group", err.Error())
	}
}

func (r *userGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *userGroupResourceModel) toUserGroup() *unifi.UserGroup {
	return &unifi.UserGroup{
		ID:             m.ID.ValueString(),
		Name:           m.Name.ValueString(),
		QOSRateMaxUp:   intPtr(m.QOSRateMaxUp),
		QOSRateMaxDown: intPtr(m.QOSRateMaxDown),
	}
}

func (m *userGroupResourceModel) fromUserGroup(g *unifi.UserGroup) {
	m.ID = types.StringValue(g.ID)
	m.Name = types.StringValue(g.Name)
	m.QOSRateMaxUp = intToState(g.QOSRateMaxUp)
	m.QOSRateMaxDown = intToState(g.QOSRateMaxDown)
	m.AttrNoDelete = types.BoolValue(g.AttrNoDelete != nil && *g.AttrNoDelete)
}
