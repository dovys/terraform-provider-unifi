package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

var (
	_ resource.Resource                = (*protectChimeResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectChimeResource)(nil)
	_ resource.ResourceWithImportState = (*protectChimeResource)(nil)
)

func NewProtectChimeResource() resource.Resource { return &protectChimeResource{} }

type protectChimeResource struct {
	protect *protect.Client
}

type protectChimeResourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name        types.String `tfsdk:"name"`
	Volume      types.Int64  `tfsdk:"volume"`
	RepeatTimes types.Int64  `tfsdk:"repeat_times"`
	CameraIDs   types.Set    `tfsdk:"camera_ids"`

	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	Model     types.String `tfsdk:"model"`
	State     types.String `tfsdk:"state"`
	Host      types.String `tfsdk:"host"`
	IsAdopted types.Bool   `tfsdk:"is_adopted"`
}

func (r *protectChimeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_chime"
}

func (r *protectChimeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect Chime, identified by MAC. Configures the existing, adopted device over the private Protect " +
			"API (`/proxy/protect/api/chimes/<id>`): name, speaker volume, ring repeat count, and the doorbell cameras it rings for. " +
			"`terraform destroy` removes the chime from state and leaves it configured; set `forget_on_destroy = true` to unadopt it. " +
			"Per-doorbell ringtone overrides (`ringSettings`) are not modeled — they reference uploaded ringtone objects the provider does not model. " +
			"**Config writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Chime MAC address (the natural key); any separator/case is accepted and normalized. The device must be adopted. Immutable — changing it replaces the resource.",
				PlanModifiers:       []planmodifier.String{normalizeMAC(), stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Chime display name."},
			"volume": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Speaker volume (0–100).",
				Validators:          []validator.Int64{int64validator.Between(0, 100)},
			},
			"repeat_times": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How many times the chime repeats per ring.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"camera_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Object ids of the doorbell cameras this chime rings for.",
			},
			"forget_on_destroy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When `true`, `terraform destroy` unadopts (forgets) the chime instead of just removing it from state. Defaults to `false`.",
			},

			"model":      schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model."},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state."},
			"host":       schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the chime is adopted by this console."},
		},
	}
}

func (r *protectChimeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

func (r *protectChimeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectChimeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac := strings.TrimSpace(plan.MAC.ValueString())
	current, err := r.protect.Chimes().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Chime not found",
				fmt.Sprintf("No chime with MAC %q is adopted on this Protect console. Adopt it first.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up chime", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectChimeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectChimeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ch, err := r.protect.Chimes().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read chime", err.Error())
		return
	}
	state.fromChime(ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectChimeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectChimeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.protect.Chimes().Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read chime before update", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectChimeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectChimeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.protect.Chimes().Delete(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
			resp.Diagnostics.AddError("Failed to unadopt chime", err.Error())
		}
	}
}

func (r *protectChimeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if strings.Contains(id, ":") {
		ch, err := r.protect.Chimes().GetByMAC(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no chime with MAC %q: %s", id, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ch.Id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), macCanonical(ch.Mac))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *protectChimeResource) applyConfig(ctx context.Context, plan *protectChimeResourceModel, current *wire.Chime, diags *diag.Diagnostics) {
	cfg := chimeConfig{
		Name:        mergeStr(plan.Name, current.Name),
		Volume:      mergeFloat(plan.Volume, current.Volume),
		RepeatTimes: mergeFloat(plan.RepeatTimes, current.RepeatTimes),
		CameraIds:   mergeStrSet(ctx, plan.CameraIDs, current.CameraIds, diags),
	}
	if diags.HasError() {
		return
	}
	updated, err := r.protect.Chimes().Update(ctx, current.Id, cfg)
	if err != nil {
		diags.AddError("Failed to apply chime config", err.Error())
		return
	}
	plan.fromChime(updated)
}

func (m *protectChimeResourceModel) fromChime(c *wire.Chime) {
	m.ID = types.StringValue(c.Id)
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(macCanonical(c.Mac))
	}
	m.Name = strToStateOptional(deref(c.Name))
	m.Volume = float64ToState(c.Volume)
	m.RepeatTimes = float64ToState(c.RepeatTimes)
	m.CameraIDs = sliceToStringSet(c.CameraIds)

	m.Model = strToStateOptional(deref(c.MarketName))
	m.State = types.StringValue(string(derefEnum(c.State)))
	m.Host = strToStateOptional(deref(c.Host))
	m.IsAdopted = boolToState(c.IsAdopted)
}

// --- owned-field overlay (omitempty) sent on PATCH ---

type chimeConfig struct {
	Name        *string  `json:"name,omitempty"`
	Volume      *float64 `json:"volume,omitempty"`
	RepeatTimes *float64 `json:"repeatTimes,omitempty"`
	CameraIds   []string `json:"cameraIds,omitempty"`
}
