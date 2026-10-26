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
	_ resource.Resource                = (*accessDoorGuardResource)(nil)
	_ resource.ResourceWithConfigure   = (*accessDoorGuardResource)(nil)
	_ resource.ResourceWithImportState = (*accessDoorGuardResource)(nil)
)

// NewAccessDoorGuardResource constructs the managed door-guard resource.
//
// A door guard is a per-door configuration: the set of users authorized to act
// as guards for one door. It is not a standalone object — it always exists as
// part of a (discovered) door and is replaced in place via PUT, so this is an
// update/import-only sub-resource keyed by door_id. Like unifi_access_door, it
// configures an aspect of discovered hardware: "create" replaces the guard set
// of the existing door and "destroy" is a no-op state removal — the guard set
// keeps its last-applied value (safe-by-default, never-forget); set user_ids =
// [] to clear it. The user_ids are opaque references the operator supplies; the
// provider does not model Access users. All HIL-unverified (no live Access
// console) — see TODO.md.
func NewAccessDoorGuardResource() resource.Resource { return &accessDoorGuardResource{} }

type accessDoorGuardResource struct {
	access *access.Client
}

type accessDoorGuardResourceModel struct {
	ID      types.String `tfsdk:"id"`
	DoorID  types.String `tfsdk:"door_id"`
	UserIDs types.Set    `tfsdk:"user_ids"`
}

func (r *accessDoorGuardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door_guard"
}

func (r *accessDoorGuardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the guard user set of a UniFi Access door (`/proxy/access/api/v2/locations/door/{id}/door_guard`). " +
			"A door guard is the set of users authorized to act as guards for one door. This is an update/import-only sub-resource " +
			"keyed by `door_id`: it replaces the door's guard set and destroying it stops managing the set without changing it " +
			"(set `user_ids = []` to clear). The `user_ids` are opaque references to existing Access users — the provider does not " +
			"model users. **Modeled from the undocumented Access API and not yet verified against a live console — see the " +
			"provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id; equals `door_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"door_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The door's `unique_id`. Changing it targets a different door (forces replacement).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Guard user ids (the `unique_id` of each Access user). Set to `[]` to clear the guard set. Reconciled from the door's guard list on refresh.",
			},
		},
	}
}

func (r *accessDoorGuardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.access = c.Access()
	}
}

// Create replaces the guard set of the door selected by door_id. There is no API
// to create a door, so a missing door_id is a configuration error.
func (r *accessDoorGuardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessDoorGuardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.DoorID.ValueString()
	if _, err := r.access.DoorGuards().Update(ctx, id, params); err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Door not found", "no door with door_id "+id+" exists. Doors are discovered, not created — reference an existing door.")
			return
		}
		resp.Diagnostics.AddError("Failed to configure door guard", err.Error())
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Door guard missing after configure", "the door guard could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessDoorGuardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessDoorGuardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.readInto(ctx, state.DoorID.ValueString(), &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessDoorGuardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accessDoorGuardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.DoorID.ValueString()
	if _, err := r.access.DoorGuards().Update(ctx, id, params); err != nil {
		resp.Diagnostics.AddError("Failed to configure door guard", err.Error())
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Door guard missing after configure", "the door guard could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: a door guard cannot be deleted through the API (it belongs
// to discovered hardware). Destroying the resource stops managing the guard set
// without changing it; set user_ids = [] to clear it instead. Terraform removes
// it from state automatically.
func (r *accessDoorGuardResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *accessDoorGuardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("door_id"), strings.TrimSpace(req.ID))...)
}

func (r *accessDoorGuardResource) params(ctx context.Context, plan *accessDoorGuardResourceModel, diags *diag.Diagnostics) wire.DoorGuardBoUpdateDoorGuard {
	return wire.DoorGuardBoUpdateDoorGuard{
		DoorId:  strPtrOrNil(plan.DoorID),
		UserIds: stringSetToSlicePtr(ctx, plan.UserIDs, diags),
	}
}

// readInto refreshes the model from the door's guard configuration. user_ids is
// normalized to an always-non-null set (empty when there are no guards) so an
// explicit user_ids = [] config plans cleanly. Returns true if the door is gone.
func (r *accessDoorGuardResource) readInto(ctx context.Context, id string, m *accessDoorGuardResourceModel, diags *diag.Diagnostics) (gone bool) {
	g, err := r.access.DoorGuards().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read door guard", err.Error())
		return false
	}

	m.ID = types.StringValue(id)
	m.DoorID = types.StringValue(id)
	m.UserIDs = sliceToStringSet(derefSlice(g.UserIds))
	return false
}
