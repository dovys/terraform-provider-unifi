package provider

import (
	"context"
	"strings"
	"time"

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
	_ resource.Resource                = (*accessHolidayGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*accessHolidayGroupResource)(nil)
	_ resource.ResourceWithImportState = (*accessHolidayGroupResource)(nil)
)

// NewAccessHolidayGroupResource constructs the managed holiday-group resource. A
// holiday group is a named set of holidays used to scope unlock schedules.
// Membership is managed via holiday_ids (the unique_ids of existing holidays),
// mirroring the door_group door_ids model; the individual holidays themselves are
// not modeled here. HIL-unverified (no live Access console) — see TODO.md.
func NewAccessHolidayGroupResource() resource.Resource { return &accessHolidayGroupResource{} }

type accessHolidayGroupResource struct {
	access *access.Client
}

type accessHolidayGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	IsPrivate    types.Bool   `tfsdk:"is_private"`
	TemplateName types.String `tfsdk:"template_name"`
	HolidayIDs   types.Set    `tfsdk:"holiday_ids"`

	HolidayCount types.Int64  `tfsdk:"holiday_count"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	CreateTime   types.String `tfsdk:"create_time"`
}

func (r *accessHolidayGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_holiday_group"
}

func (r *accessHolidayGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Access holiday group — a named set of holidays used to scope unlock schedules. Managed over " +
			"the private Access API (`/proxy/access/api/v2/holiday_group`). Membership is the set of holiday `unique_id`s in " +
			"`holiday_ids`; the holidays themselves are not modeled by this resource. **Modeled from the undocumented " +
			"Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Holiday group display name.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Holiday group description.",
			},
			"is_private": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the group is private to its creator.",
			},
			"template_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Template the group was seeded from, if any.",
			},
			"holiday_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Member holiday ids (`unique_id` of each holiday). Reconciled from the group's holiday list on refresh.",
			},
			"holiday_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of member holidays (read-only)."},
			"is_default":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the default holiday group (read-only)."},
			"create_time":   schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp, RFC3339 (read-only)."},
		},
	}
}

func (r *accessHolidayGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.access = c.Access()
	}
}

func (r *accessHolidayGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessHolidayGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.access.HolidayGroups().Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create holiday group", err.Error())
		return
	}
	id := deref(created.UniqueId)
	if id == "" {
		resp.Diagnostics.AddError("Create returned no id", "the Access API did not return a unique_id for the new holiday group")
		return
	}
	if r.readInto(ctx, id, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Holiday group missing after create", "the created holiday group could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessHolidayGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessHolidayGroupResourceModel
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

func (r *accessHolidayGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state accessHolidayGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := r.params(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.access.HolidayGroups().Update(ctx, state.ID.ValueString(), params); err != nil {
		resp.Diagnostics.AddError("Failed to update holiday group", err.Error())
		return
	}
	if r.readInto(ctx, state.ID.ValueString(), &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Holiday group missing after update", "the holiday group could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessHolidayGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accessHolidayGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.access.HolidayGroups().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete holiday group", err.Error())
	}
}

func (r *accessHolidayGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strings.TrimSpace(req.ID))...)
}

// params builds the write DTO from the plan. holiday_ids is sent only when
// configured; an unset (computed) holiday_ids leaves membership untouched.
func (r *accessHolidayGroupResource) params(ctx context.Context, plan *accessHolidayGroupResourceModel, diags *diag.Diagnostics) wire.HolidayBoHolidayGroupParams {
	return wire.HolidayBoHolidayGroupParams{
		Name:         strPtrOrNil(plan.Name),
		Description:  strPtrOrNil(plan.Description),
		IsPrivate:    boolPtrOrNil(plan.IsPrivate),
		TemplateName: strPtrOrNil(plan.TemplateName),
		HolidayIds:   stringSetToSlicePtr(ctx, plan.HolidayIDs, diags),
	}
}

// readInto refreshes the model from the holiday-group detail view. holiday_ids is
// derived from the member holiday list. Returns true if the group is gone.
func (r *accessHolidayGroupResource) readInto(ctx context.Context, id string, m *accessHolidayGroupResourceModel, diags *diag.Diagnostics) (gone bool) {
	g, err := r.access.HolidayGroups().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			return true
		}
		diags.AddError("Failed to read holiday group", err.Error())
		return false
	}

	m.ID = types.StringValue(id)
	m.Name = strToStateOptional(deref(g.Name))
	m.Description = strToStateOptional(deref(g.Description))
	m.IsPrivate = boolToState(g.IsPrivate)
	m.TemplateName = strToStateOptional(deref(g.TemplateName))
	m.IsDefault = boolToState(g.IsDefault)
	m.CreateTime = accessTimeToState(g.CreateTime)

	ids := holidayMemberIDs(g.Holidays)
	m.HolidayIDs = sliceToStringSet(ids)
	m.HolidayCount = types.Int64Value(int64(len(ids)))
	return false
}

// holidayMemberIDs extracts the member holiday unique_ids from a detail view's
// holiday list, falling back to the numeric id when unique_id is empty.
func holidayMemberIDs(holidays *[]wire.HolidayBoHolidayParams) []string {
	if holidays == nil {
		return []string{}
	}
	ids := make([]string, 0, len(*holidays))
	for _, h := range *holidays {
		if id := deref(h.UniqueId); id != "" {
			ids = append(ids, id)
		} else if hid := deref(h.Id); hid != "" {
			ids = append(ids, hid)
		}
	}
	return ids
}

// accessTimeToState renders an optional timestamp as an RFC3339 string (null when
// the pointer is nil).
func accessTimeToState(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}
