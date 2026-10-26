package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

var (
	_ resource.Resource                = (*accessDoorGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*accessDoorGroupResource)(nil)
	_ resource.ResourceWithImportState = (*accessDoorGroupResource)(nil)
)

// NewAccessDoorGroupResource constructs the managed door-group resource. door_group
// is the one Access object the spec provides a write DTO for, so
// it is the Access app's first managed resource. Its CRUD paths and the
// {code,msg,data} envelope contract are HIL-unverified (no live Access console) —
// the schema and TODO.md flag this; the verification items in TODO.md should be
// confirmed against a live console.
func NewAccessDoorGroupResource() resource.Resource { return &accessDoorGroupResource{} }

type accessDoorGroupResource struct {
	access *access.Client
}

type accessDoorGroupResourceModel struct {
	ID        types.String `tfsdk:"id"`         // Access unique_id (computed)
	Name      types.String `tfsdk:"name"`       // display name
	GroupType types.String `tfsdk:"group_type"` // controller-assigned grouping kind
	DoorIDs   types.Set    `tfsdk:"door_ids"`   // member door unique_ids
}

func (r *accessDoorGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door_group"
}

func (r *accessDoorGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Access door group — a named set of doors used to scope access policies. Managed over the " +
			"private Access API (`/proxy/access/api/v2/access/door_group`). Members are read back from the dedicated " +
			"`/door_ids` endpoint. Floor groups (member floors) are out of scope for now (no read path is modeled). " +
			"**Modeled from the undocumented Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Door group display name.",
			},
			"group_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				// group_type is controller-assigned, so we deliberately leave it
				// unpinned: an unset group_type plans as "known after apply" and Read
				// surfaces the controller's choice each cycle. (A user-set knob would
				// take UseStateForUnknown to avoid post-import churn; this isn't one.)
				MarkdownDescription: "Grouping kind (controller-assigned, e.g. `door`). Left unset, the controller chooses the " +
					"default for a door group.",
			},
			"door_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Member door ids (`unique_id` of each door). Use the `unifi_access_door` data source (or the Access UI) to obtain them.",
			},
		},
	}
}

func (r *accessDoorGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.access = c.Access()
	}
}

func (r *accessDoorGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessDoorGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.access.DoorGroups().Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create door group", err.Error())
		return
	}
	id := deref(created.UniqueId)
	if id == "" {
		resp.Diagnostics.AddError("Create returned no id", "the Access API did not return a unique_id for the new door group")
		return
	}
	r.readInto(ctx, id, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessDoorGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessDoorGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	gone := r.readInto(ctx, state.ID.ValueString(), &state, &resp.Diagnostics)
	if gone {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessDoorGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state accessDoorGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.access.DoorGroups().Update(ctx, state.ID.ValueString(), params); err != nil {
		resp.Diagnostics.AddError("Failed to update door group", err.Error())
		return
	}
	r.readInto(ctx, state.ID.ValueString(), &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessDoorGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accessDoorGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.access.DoorGroups().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete door group", err.Error())
	}
}

func (r *accessDoorGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strings.TrimSpace(req.ID))...)
}

// params builds the write DTO from the plan. door_ids is sent only when
// configured; an unset (computed) door_ids leaves membership untouched.
func (r *accessDoorGroupResource) params(ctx context.Context, plan *accessDoorGroupResourceModel, diags *diag.Diagnostics) wire.DoorGroupControllerDoorGroupParams {
	params := wire.DoorGroupControllerDoorGroupParams{
		Name:      strPtrOrNil(plan.Name),
		GroupType: strPtrOrNil(plan.GroupType),
	}
	if !plan.DoorIDs.IsNull() && !plan.DoorIDs.IsUnknown() {
		ids := stringSetToSlice(ctx, plan.DoorIDs, diags)
		if ids == nil {
			ids = []string{} // explicit empty set clears membership ([]), not null
		}
		params.DoorIds = &ids
	}
	return params
}

// readInto refreshes the model from the controller: the group object (name,
// group_type) plus its door membership from the dedicated endpoint. Returns true
// if the group is gone (caller removes it from state).
func (r *accessDoorGroupResource) readInto(ctx context.Context, id string, m *accessDoorGroupResourceModel, diags *diag.Diagnostics) (gone bool) {
	g, err := r.access.DoorGroups().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read door group", err.Error())
		return false
	}
	ids, err := r.access.DoorGroups().DoorIDs(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read door group members", err.Error())
		return false
	}

	m.ID = types.StringValue(id)
	m.Name = strToStateOptional(deref(g.Name))
	m.GroupType = strToStateOptional(deref(g.GroupType))
	m.DoorIDs = sliceToStringSet(ids)
	return false
}
