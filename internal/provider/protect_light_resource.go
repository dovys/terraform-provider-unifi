package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = (*protectLightResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectLightResource)(nil)
	_ resource.ResourceWithImportState = (*protectLightResource)(nil)
)

func NewProtectLightResource() resource.Resource { return &protectLightResource{} }

type protectLightResource struct {
	protect *protect.Client
}

type protectLightResourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name             types.String `tfsdk:"name"`
	Camera           types.String `tfsdk:"camera"`
	Mode             types.String `tfsdk:"mode"`
	EnableAt         types.String `tfsdk:"enable_at"`
	LedLevel         types.Int64  `tfsdk:"led_level"`
	IndicatorEnabled types.Bool   `tfsdk:"indicator_enabled"`
	PirDuration      types.Int64  `tfsdk:"pir_duration"`
	PirSensitivity   types.Int64  `tfsdk:"pir_sensitivity"`
	LuxSensitivity   types.String `tfsdk:"lux_sensitivity"`
	ForceLedOn       types.Bool   `tfsdk:"force_led_on"`

	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	Model     types.String `tfsdk:"model"`
	Type      types.String `tfsdk:"type"`
	State     types.String `tfsdk:"state"`
	Host      types.String `tfsdk:"host"`
	IsAdopted types.Bool   `tfsdk:"is_adopted"`
	IsLightOn types.Bool   `tfsdk:"is_light_on"`
}

func (r *protectLightResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_light"
}

func (r *protectLightResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect floodlight (UP-FloodLight), identified by MAC. Configures the existing, adopted " +
			"device over the private Protect API (`/proxy/protect/api/lights/<id>`): name, paired camera, activation mode/schedule, " +
			"LED brightness, PIR (motion) duration/sensitivity, and ambient-light (lux) sensitivity. `terraform destroy` removes the " +
			"light from state and leaves it configured; set `forget_on_destroy = true` to unadopt it. " +
			"**Config writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Floodlight MAC address (the natural key); any separator/case is accepted and normalized to " +
					"lowercase colon-separated form. The device must be adopted and visible to Protect. Immutable — changing it replaces the resource.",
				PlanModifiers: []planmodifier.String{normalizeMAC(), stringplanmodifier.RequiresReplace()},
			},
			"name":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Light display name."},
			"camera": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Object id of the camera paired to this light (motion from that camera can trigger it)."},
			"mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Activation mode: `off`, `motion`, `always`, or `schedule`.",
				Validators:          []validator.String{stringvalidator.OneOf("off", "motion", "always", "schedule")},
			},
			"enable_at": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "When the light is allowed to activate: `dark`, `night`, or `fulltime`.",
				Validators:          []validator.String{stringvalidator.OneOf("dark", "night", "fulltime")},
			},
			"led_level": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "LED brightness level (model-dependent scale, typically 1–6).",
			},
			"indicator_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the status indicator LED is enabled."},
			"pir_duration": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How long the light stays on after motion, in milliseconds.",
			},
			"pir_sensitivity": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "PIR motion sensitivity (0–100).",
				Validators:          []validator.Int64{int64validator.Between(0, 100)},
			},
			"lux_sensitivity": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Ambient-light (lux) sensitivity threshold: `low`, `medium`, or `high`.",
				Validators:          []validator.String{stringvalidator.OneOf("low", "medium", "high")},
			},
			"force_led_on": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Force the main LED on regardless of mode."},
			"forget_on_destroy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When `true`, `terraform destroy` unadopts (forgets) the light instead of just removing it from state. Defaults to `false`.",
			},

			"model":       schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model (e.g. `UP FloodLight`)."},
			"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Device type identifier."},
			"state":       schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state (`CONNECTED`, `DISCONNECTED`, …)."},
			"host":        schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the light is adopted by this console."},
			"is_light_on": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the main LED is currently on."},
		},
	}
}

func (r *protectLightResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

func (r *protectLightResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectLightResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac := strings.TrimSpace(plan.MAC.ValueString())
	current, err := r.protect.Lights().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Light not found",
				fmt.Sprintf("No floodlight with MAC %q is adopted on this Protect console. Adopt it first.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up light", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectLightResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectLightResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	light, err := r.protect.Lights().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read light", err.Error())
		return
	}
	state.fromLight(light)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectLightResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectLightResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.protect.Lights().Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read light before update", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectLightResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectLightResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.protect.Lights().Delete(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
			resp.Diagnostics.AddError("Failed to unadopt light", err.Error())
		}
	}
}

func (r *protectLightResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if strings.Contains(id, ":") {
		light, err := r.protect.Lights().GetByMAC(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no light with MAC %q: %s", id, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), light.Id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), macCanonical(light.Mac))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// applyConfig PATCHes the plan's owned fields onto the light, merging plan over
// current so a sibling field the provider models but the practitioner didn't set
// is preserved (the Protect owned-overlay discipline).
func (r *protectLightResource) applyConfig(ctx context.Context, plan *protectLightResourceModel, current *wire.Light, diags *diag.Diagnostics) {
	cfg := lightConfig{
		Name:   mergeStr(plan.Name, current.Name),
		Camera: mergeStr(plan.Camera, current.Camera),
	}
	dev := &lightDeviceConfig{
		IsIndicatorEnabled: mergeBool(plan.IndicatorEnabled, current.LightDeviceSettings.IsIndicatorEnabled),
		LedLevel:           mergeFloat(plan.LedLevel, current.LightDeviceSettings.LedLevel),
		LuxSensitivity:     mergeStr(plan.LuxSensitivity, (*string)(current.LightDeviceSettings.LuxSensitivity)),
		PirDuration:        mergeFloat(plan.PirDuration, current.LightDeviceSettings.PirDuration),
		PirSensitivity:     mergeFloat(plan.PirSensitivity, current.LightDeviceSettings.PirSensitivity),
	}
	if dev.IsIndicatorEnabled != nil || dev.LedLevel != nil || dev.LuxSensitivity != nil || dev.PirDuration != nil || dev.PirSensitivity != nil {
		cfg.LightDeviceSettings = dev
	}
	mode := &lightModeConfig{
		Mode:     mergeStr(plan.Mode, (*string)(current.LightModeSettings.Mode)),
		EnableAt: mergeStr(plan.EnableAt, (*string)(current.LightModeSettings.EnableAt)),
	}
	if mode.Mode != nil || mode.EnableAt != nil {
		cfg.LightModeSettings = mode
	}
	if on := mergeBool(plan.ForceLedOn, current.LightOnSettings.IsLedForceOn); on != nil {
		cfg.LightOnSettings = &lightOnConfig{IsLedForceOn: on}
	}

	updated, err := r.protect.Lights().Update(ctx, current.Id, cfg)
	if err != nil {
		diags.AddError("Failed to apply light config", err.Error())
		return
	}
	plan.fromLight(updated)
}

func (m *protectLightResourceModel) fromLight(l *wire.Light) {
	m.ID = types.StringValue(l.Id)
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(macCanonical(l.Mac))
	}
	m.Name = strToStateOptional(deref(l.Name))
	m.Camera = strToStateOptional(deref(l.Camera))
	m.Mode = strToStateOptional(string(derefEnum(l.LightModeSettings.Mode)))
	m.EnableAt = strToStateOptional(string(derefEnum(l.LightModeSettings.EnableAt)))
	m.LedLevel = float64ToState(l.LightDeviceSettings.LedLevel)
	m.IndicatorEnabled = boolToState(l.LightDeviceSettings.IsIndicatorEnabled)
	m.PirDuration = float64ToState(l.LightDeviceSettings.PirDuration)
	m.PirSensitivity = float64ToState(l.LightDeviceSettings.PirSensitivity)
	m.LuxSensitivity = strToStateOptional(string(derefEnum(l.LightDeviceSettings.LuxSensitivity)))
	m.ForceLedOn = boolToState(l.LightOnSettings.IsLedForceOn)

	m.Model = strToStateOptional(deref(l.MarketName))
	m.Type = strToStateOptional(deref(l.Type))
	m.State = types.StringValue(string(derefEnum(l.State)))
	m.Host = strToStateOptional(deref(l.Host))
	m.IsAdopted = boolToState(l.IsAdopted)
	m.IsLightOn = boolToState(l.IsLightOn)
}

// --- owned-field overlay (omitempty) sent on PATCH ---

type lightConfig struct {
	Name                *string            `json:"name,omitempty"`
	Camera              *string            `json:"camera,omitempty"`
	LightDeviceSettings *lightDeviceConfig `json:"lightDeviceSettings,omitempty"`
	LightModeSettings   *lightModeConfig   `json:"lightModeSettings,omitempty"`
	LightOnSettings     *lightOnConfig     `json:"lightOnSettings,omitempty"`
}

type lightDeviceConfig struct {
	IsIndicatorEnabled *bool    `json:"isIndicatorEnabled,omitempty"`
	LedLevel           *float64 `json:"ledLevel,omitempty"`
	LuxSensitivity     *string  `json:"luxSensitivity,omitempty"`
	PirDuration        *float64 `json:"pirDuration,omitempty"`
	PirSensitivity     *float64 `json:"pirSensitivity,omitempty"`
}

type lightModeConfig struct {
	EnableAt *string `json:"enableAt,omitempty"`
	Mode     *string `json:"mode,omitempty"`
}

type lightOnConfig struct {
	IsLedForceOn *bool `json:"isLedForceOn,omitempty"`
}
