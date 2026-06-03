package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                = (*accessPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*accessPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*accessPolicyResource)(nil)
)

// NewAccessPolicyResource constructs the managed access-policy resource.
//
// The write body (AccessPolicyBo: name, schedule, configs, resource grants) and
// the read body (AccessPolicy: an embedded Policy plus access_resources/subjects)
// are different controller shapes, so the resource manages the clearly-writable
// surface — name, schedule_id, enabled, and resource grants ({type,id}) — and
// reconciles name + computed status from the read view. The deeper subject/rule
// graph is controller/UI-managed and intentionally not modeled here. All of this
// is HIL-unverified (no live Access console) — see TODO.md.
func NewAccessPolicyResource() resource.Resource { return &accessPolicyResource{} }

type accessPolicyResource struct {
	access *access.Client
}

type accessPolicyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ScheduleID types.String `tfsdk:"schedule_id"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Resources  types.Set    `tfsdk:"resources"`

	// Computed read-only status (from the access-policy read view).
	Description    types.String `tfsdk:"description"`
	CreatorName    types.String `tfsdk:"creator_name"`
	UserCount      types.Int64  `tfsdk:"user_count"`
	DoorCount      types.Int64  `tfsdk:"door_count"`
	ElevatorCount  types.Int64  `tfsdk:"elevator_count"`
	GroupCount     types.Int64  `tfsdk:"group_count"`
	ValidStartTime types.String `tfsdk:"valid_start_time"`
	ValidEndTime   types.String `tfsdk:"valid_end_time"`
}

type accessPolicyResourceObjectModel struct {
	Type types.String `tfsdk:"type"`
	ID   types.String `tfsdk:"id"`
}

var accessPolicyResourceObjectAttrTypes = map[string]attr.Type{
	"type": types.StringType,
	"id":   types.StringType,
}

var accessPolicyResourceSetType = types.SetType{ElemType: types.ObjectType{AttrTypes: accessPolicyResourceObjectAttrTypes}}

func (r *accessPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_policy"
}

func (r *accessPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Access policy — grants a set of resources (doors / door groups) on an optional schedule. " +
			"Managed over the private Access API (`/proxy/access/api/v2/access_policy`). The policy's subject/rule graph is " +
			"controller-managed and not modeled here; this resource manages the policy identity, schedule, enabled state, and " +
			"resource grants. **Modeled from the reverse-engineered Access API and not yet verified against a live console — see " +
			"the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Access policy display name.",
			},
			"schedule_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Schedule (`unique_id`) that gates when the policy is in effect. The read view does not " +
					"echo this field, so it is not reconciled on refresh (no drift detection) — HIL-unverified.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the policy is active. Maps to the policy `is_active` config; reconciled from the read view's `active`.",
			},
			"resources": schema.SetNestedAttribute{
				Optional: true,
				MarkdownDescription: "Resources the policy grants access to. Each entry is a `{type, id}` reference (e.g. " +
					"`type = \"door\"` with a door `unique_id`, or `type = \"door_group\"`). The read view returns a richer resource " +
					"shape, so this set is sent on write but not reconciled on refresh — HIL-unverified.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{Required: true, MarkdownDescription: "Resource type, e.g. `door` or `door_group`."},
						"id":   schema.StringAttribute{Required: true, MarkdownDescription: "Resource `unique_id`."},
					},
				},
			},
			"description":      schema.StringAttribute{Computed: true, MarkdownDescription: "Policy description (read-only)."},
			"creator_name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the policy creator (read-only)."},
			"user_count":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of users the policy applies to (read-only)."},
			"door_count":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of doors granted (read-only)."},
			"elevator_count":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of elevators granted (read-only)."},
			"group_count":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of door groups granted (read-only)."},
			"valid_start_time": schema.StringAttribute{Computed: true, MarkdownDescription: "Policy validity window start (read-only)."},
			"valid_end_time":   schema.StringAttribute{Computed: true, MarkdownDescription: "Policy validity window end (read-only)."},
		},
	}
}

func (r *accessPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.access = c.Access()
	}
}

func (r *accessPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.access.AccessPolicies().Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create access policy", err.Error())
		return
	}
	id := deref(created.UniqueId)
	if id == "" {
		resp.Diagnostics.AddError("Create returned no id", "the Access API did not return a unique_id for the new access policy")
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Access policy missing after create", "the created policy could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.readInto(ctx, state.ID.ValueString(), &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state accessPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.access.AccessPolicies().Update(ctx, state.ID.ValueString(), params); err != nil {
		resp.Diagnostics.AddError("Failed to update access policy", err.Error())
		return
	}
	if r.readInto(ctx, state.ID.ValueString(), &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Access policy missing after update", "the policy could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accessPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.access.AccessPolicies().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete access policy", err.Error())
	}
}

func (r *accessPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strings.TrimSpace(req.ID))...)
}

// params builds the write DTO from the plan. schedule_id and the resource grants
// are sent only when configured; enabled maps to the policy is_active config.
func (r *accessPolicyResource) params(ctx context.Context, plan *accessPolicyResourceModel, diags *diag.Diagnostics) wire.AccessPolicyBoAccessPolicyBo {
	p := wire.AccessPolicyBoAccessPolicyBo{
		Name:       strPtrOrNil(plan.Name),
		ScheduleId: strPtrOrNil(plan.ScheduleID),
		Resource:   r.expandResources(ctx, plan.Resources, diags),
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		p.Configs = &wire.AccessPolicyPoConfigs{IsActive: boolPtr(plan.Enabled)}
	}
	return p
}

func (r *accessPolicyResource) expandResources(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]wire.AccessPolicyBoResourceObject {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var objs []accessPolicyResourceObjectModel
	diags.Append(s.ElementsAs(ctx, &objs, false)...)
	out := make([]wire.AccessPolicyBoResourceObject, 0, len(objs))
	for _, o := range objs {
		out = append(out, wire.AccessPolicyBoResourceObject{
			Type: strPtrOrNil(o.Type),
			Id:   strPtrOrNil(o.ID),
		})
	}
	return &out
}

// readInto refreshes the model from the access-policy read view. name, enabled,
// and the computed status fields are reconciled from the controller; schedule_id
// and resources are preserved from the prior model (the read view does not echo
// them). Returns true if the policy is gone (caller removes it from state).
func (r *accessPolicyResource) readInto(ctx context.Context, id string, m *accessPolicyResourceModel, diags *diag.Diagnostics) (gone bool) {
	p, err := r.access.AccessPolicies().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read access policy", err.Error())
		return false
	}

	m.ID = types.StringValue(id)
	if p.Policy != nil {
		m.Name = strToStateOptional(deref(p.Policy.Name))
		m.Description = strToStateOptional(deref(p.Policy.Description))
	}
	m.Enabled = boolToState(p.Active)
	m.CreatorName = strToStateOptional(deref(p.CreatorName))
	m.UserCount = intToState(p.UserCount)

	var dc, ec, gc *int
	if p.ResourceCount != nil {
		dc, ec, gc = p.ResourceCount.DoorCount, p.ResourceCount.ElevatorCount, p.ResourceCount.GroupCount
	}
	m.DoorCount = intToState(dc)
	m.ElevatorCount = intToState(ec)
	m.GroupCount = intToState(gc)

	m.ValidStartTime = strToStateOptional(deref(p.ValidStartTime))
	m.ValidEndTime = strToStateOptional(deref(p.ValidEndTime))
	return false
}
