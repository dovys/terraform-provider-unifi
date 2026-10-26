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
	_ resource.Resource                = (*accessDoorResource)(nil)
	_ resource.ResourceWithConfigure   = (*accessDoorResource)(nil)
	_ resource.ResourceWithImportState = (*accessDoorResource)(nil)
)

// NewAccessDoorResource constructs the managed door resource.
//
// Doors are discovered location objects, not created through the API: this is an
// update/import-only resource that configures an existing door (selected by
// door_id). There is no API to create or delete a door, so "create" configures
// the existing door and "destroy" is a no-op state removal — the door keeps its
// last-applied configuration (the safe-by-default, never-forget posture). All
// HIL-unverified (no live Access console) — see TODO.md.
func NewAccessDoorResource() resource.Resource { return &accessDoorResource{} }

type accessDoorResource struct {
	access *access.Client
}

type accessDoorResourceModel struct {
	ID        types.String `tfsdk:"id"`
	DoorID    types.String `tfsdk:"door_id"`
	Name      types.String `tfsdk:"name"`
	FloorName types.String `tfsdk:"floor_name"`
	ExtraType types.String `tfsdk:"extra_type"`

	FullName     types.String `tfsdk:"full_name"`
	LocationType types.String `tfsdk:"location_type"`
	Timezone     types.String `tfsdk:"timezone"`
	IsOpen       types.Bool   `tfsdk:"is_open"`
	DpsConnected types.Bool   `tfsdk:"dps_connected"`
}

func (r *accessDoorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door"
}

func (r *accessDoorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configures an existing UniFi Access door (`/proxy/access/api/v2/locations/door/{id}`). Doors are " +
			"**discovered**, not created — this resource is update/import-only: it adopts an existing door by `door_id` and " +
			"manages its name, floor, and type; destroying it stops managing the door without deleting it. **Modeled from the " +
			"undocumented Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`); equals `door_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"door_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The discovered door's `unique_id`. Changing it targets a different door (forces replacement).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Door display name.",
			},
			"floor_name": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Move the door under the floor with this name. This is a write-time association; the door object " +
					"does not echo its floor name, so it is not reconciled on refresh (no drift detection) — HIL-unverified.",
			},
			"extra_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Door type tag (e.g. `standard`).",
			},
			"full_name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Full location path of the door (read-only)."},
			"location_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Location type of the door (read-only)."},
			"timezone":      schema.StringAttribute{Computed: true, MarkdownDescription: "Door timezone (read-only)."},
			"is_open":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the door is currently open (read-only, live state)."},
			"dps_connected": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a door-position sensor is connected (read-only, live state)."},
		},
	}
}

func (r *accessDoorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.access = c.Access()
	}
}

// Create configures the existing door selected by door_id. There is no API to
// create a door, so a missing door_id is a configuration error.
func (r *accessDoorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessDoorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.DoorID.ValueString()
	if _, err := r.access.Doors().Update(ctx, id, r.params(&plan)); err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Door not found", "no door with door_id "+id+" exists. Doors are discovered, not created — import or reference an existing door.")
			return
		}
		resp.Diagnostics.AddError("Failed to configure door", err.Error())
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Door missing after configure", "the door could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessDoorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessDoorResourceModel
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

func (r *accessDoorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accessDoorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.DoorID.ValueString()
	if _, err := r.access.Doors().Update(ctx, id, r.params(&plan)); err != nil {
		resp.Diagnostics.AddError("Failed to configure door", err.Error())
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Door missing after configure", "the door could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: a door cannot be deleted through the API (it is discovered
// hardware). Destroying the resource stops managing the door without changing it;
// Terraform removes it from state automatically.
func (r *accessDoorResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *accessDoorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("door_id"), strings.TrimSpace(req.ID))...)
}

func (r *accessDoorResource) params(plan *accessDoorResourceModel) wire.LocationBoUpdateDoorParams {
	return wire.LocationBoUpdateDoorParams{
		DoorId:    strPtrOrNil(plan.DoorID),
		DoorName:  strPtrOrNil(plan.Name),
		FloorName: strPtrOrNil(plan.FloorName),
		ExtraType: strPtrOrNil(plan.ExtraType),
	}
}

// readInto refreshes the model from the door's location embed. floor_name is a
// write-time association the door object doesn't echo, so it is preserved from
// the prior model. Returns true if the door is gone.
func (r *accessDoorResource) readInto(ctx context.Context, id string, m *accessDoorResourceModel, diags *diag.Diagnostics) (gone bool) {
	d, err := r.access.Doors().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read door", err.Error())
		return false
	}

	m.ID = types.StringValue(id)
	m.DoorID = types.StringValue(id)
	if loc := d.Location; loc != nil {
		m.Name = strToStateOptional(deref(loc.Name))
		m.ExtraType = strToStateOptional(deref(loc.ExtraType))
		m.FullName = strToStateOptional(deref(loc.FullName))
		m.LocationType = strToStateOptional(deref(loc.LocationType))
		m.Timezone = strToStateOptional(deref(loc.Timezone))
	}
	m.IsOpen = boolToState(d.IsOpen)
	m.DpsConnected = boolToState(d.DpsConnected)
	return false
}
