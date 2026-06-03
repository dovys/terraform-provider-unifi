package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

var (
	_ resource.Resource                = (*protectLiveviewResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectLiveviewResource)(nil)
	_ resource.ResourceWithImportState = (*protectLiveviewResource)(nil)
)

func NewProtectLiveviewResource() resource.Resource { return &protectLiveviewResource{} }

type protectLiveviewResource struct {
	protect *protect.Client
}

type protectLiveviewResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Layout    types.Int64  `tfsdk:"layout"`
	IsDefault types.Bool   `tfsdk:"is_default"`
	IsGlobal  types.Bool   `tfsdk:"is_global"`
	Slots     types.List   `tfsdk:"slots"`
}

// liveview slot object attribute types (nested block element).
var liveviewSlotAttrTypes = map[string]attr.Type{
	"cameras":        types.ListType{ElemType: types.StringType},
	"cycle_mode":     types.StringType,
	"cycle_interval": types.Int64Type,
}

func (r *protectLiveviewResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_liveview"
}

func (r *protectLiveviewResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect live view — a saved multi-camera grid layout, managed over the private Protect API " +
			"(`/proxy/protect/api/liveviews`). A full CRUD object (created, not configure-existing). Each `slots` entry lists the " +
			"cameras shown in one grid cell and optional cycling. **The slot shape is reverse-engineered (untyped in the spec) and " +
			"HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "Live view name (also accepted as an import key)."},
			"layout": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of slots / grid layout (e.g. 1, 4, 9, 16). Should match the number of `slots` entries.",
			},
			"is_default": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether this is the default live view for all viewers."},
			"is_global":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether this live view is global (available system-wide to all users)."},
			"slots": schema.ListNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Ordered grid cells. Each lists the camera ids shown in that cell and optional auto-cycling.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cameras": schema.ListAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Camera object ids displayed in this cell (cycled when more than one).",
						},
						"cycle_mode":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Cycling mode when the cell holds multiple cameras (e.g. `motion`)."},
						"cycle_interval": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Seconds between camera cycles."},
					},
				},
			},
		},
	}
}

func (r *protectLiveviewResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

func (r *protectLiveviewResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectLiveviewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := r.config(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.protect.Liveviews().Create(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create live view", err.Error())
		return
	}
	plan.fromLiveview(ctx, created, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectLiveviewResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectLiveviewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	lv, err := r.protect.Liveviews().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read live view", err.Error())
		return
	}
	state.fromLiveview(ctx, lv, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectLiveviewResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectLiveviewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := r.config(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.protect.Liveviews().Update(ctx, state.ID.ValueString(), cfg)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update live view", err.Error())
		return
	}
	plan.fromLiveview(ctx, updated, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectLiveviewResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectLiveviewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.protect.Liveviews().Delete(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
		resp.Diagnostics.AddError("Failed to delete live view", err.Error())
	}
}

func (r *protectLiveviewResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	// A 24-hex id imports directly; anything else is treated as a name lookup.
	if len(id) == 24 && !strings.ContainsAny(id, " /") {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	lv, err := r.protect.Liveviews().GetByName(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no live view with id/name %q: %s", id, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), lv.Id)...)
}

// config builds the owned-field overlay from the plan. Slots are sent only when
// configured (an unset slots leaves the controller's untouched).
func (r *protectLiveviewResource) config(ctx context.Context, plan *protectLiveviewResourceModel, diags *diag.Diagnostics) liveviewConfig {
	cfg := liveviewConfig{
		Name:      strPtrOrNil(plan.Name),
		Layout:    intToFloatPtr(plan.Layout),
		IsDefault: boolPtrOrNil(plan.IsDefault),
		IsGlobal:  boolPtrOrNil(plan.IsGlobal),
	}
	if !plan.Slots.IsNull() && !plan.Slots.IsUnknown() {
		var slots []liveviewSlotModel
		diags.Append(plan.Slots.ElementsAs(ctx, &slots, false)...)
		raw := make([]interface{}, 0, len(slots))
		for _, s := range slots {
			cams := stringListToSlice(ctx, s.Cameras, diags)
			cells := make([]interface{}, len(cams))
			for i, c := range cams {
				cells[i] = c
			}
			m := map[string]interface{}{"cameras": cells}
			if !s.CycleMode.IsNull() && !s.CycleMode.IsUnknown() {
				m["cycleMode"] = s.CycleMode.ValueString()
			}
			if !s.CycleInterval.IsNull() && !s.CycleInterval.IsUnknown() {
				m["cycleInterval"] = float64(s.CycleInterval.ValueInt64())
			}
			raw = append(raw, m)
		}
		cfg.Slots = &raw
	}
	return cfg
}

type liveviewSlotModel struct {
	Cameras       types.List   `tfsdk:"cameras"`
	CycleMode     types.String `tfsdk:"cycle_mode"`
	CycleInterval types.Int64  `tfsdk:"cycle_interval"`
}

func (m *protectLiveviewResourceModel) fromLiveview(ctx context.Context, lv *wire.Liveview, diags *diag.Diagnostics) {
	m.ID = types.StringValue(lv.Id)
	m.Name = strToStateOptional(deref(lv.Name))
	m.Layout = float64ToState(lv.Layout)
	m.IsDefault = boolToState(lv.IsDefault)
	m.IsGlobal = boolToState(lv.IsGlobal)
	m.Slots = slotsToState(ctx, lv.Slots, diags)
}

// slotsToState parses the untyped wire slots ([]interface{} of maps) into the
// nested list. Unrecognized shapes degrade to an empty/partial slot rather than
// erroring, so a controller-side field the provider doesn't model can't break Read.
func slotsToState(ctx context.Context, raw *[]interface{}, diags *diag.Diagnostics) types.List {
	objType := types.ObjectType{AttrTypes: liveviewSlotAttrTypes}
	if raw == nil {
		return types.ListNull(objType)
	}
	elems := make([]attr.Value, 0, len(*raw))
	for _, item := range *raw {
		m, _ := item.(map[string]interface{})
		var cams []string
		if cs, ok := m["cameras"].([]interface{}); ok {
			for _, c := range cs {
				if s, ok := c.(string); ok {
					cams = append(cams, s)
				}
			}
		}
		camList := sliceToStringList(cams)
		mode := types.StringNull()
		if s, ok := m["cycleMode"].(string); ok && s != "" {
			mode = types.StringValue(s)
		}
		interval := types.Int64Null()
		if f, ok := m["cycleInterval"].(float64); ok {
			interval = types.Int64Value(int64(f))
		}
		obj, d := types.ObjectValue(liveviewSlotAttrTypes, map[string]attr.Value{
			"cameras":        camList,
			"cycle_mode":     mode,
			"cycle_interval": interval,
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	l, d := types.ListValue(objType, elems)
	diags.Append(d...)
	return l
}

// --- owned-field overlay (omitempty) sent on POST/PATCH ---

type liveviewConfig struct {
	Name      *string        `json:"name,omitempty"`
	Layout    *float64       `json:"layout,omitempty"`
	IsDefault *bool          `json:"isDefault,omitempty"`
	IsGlobal  *bool          `json:"isGlobal,omitempty"`
	Slots     *[]interface{} `json:"slots,omitempty"`
}
