package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
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
	_ resource.Resource                = (*protectSensorResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectSensorResource)(nil)
	_ resource.ResourceWithImportState = (*protectSensorResource)(nil)
)

func NewProtectSensorResource() resource.Resource { return &protectSensorResource{} }

type protectSensorResource struct {
	protect *protect.Client
}

type protectSensorResourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name      types.String `tfsdk:"name"`
	MountType types.String `tfsdk:"mount_type"`
	Camera    types.String `tfsdk:"camera"`

	MotionEnabled     types.Bool    `tfsdk:"motion_enabled"`
	MotionSensitivity types.Float64 `tfsdk:"motion_sensitivity"`

	TemperatureEnabled types.Bool    `tfsdk:"temperature_enabled"`
	TemperatureLow     types.Float64 `tfsdk:"temperature_low"`
	TemperatureHigh    types.Float64 `tfsdk:"temperature_high"`
	TemperatureMargin  types.Float64 `tfsdk:"temperature_margin"`

	HumidityEnabled types.Bool    `tfsdk:"humidity_enabled"`
	HumidityLow     types.Float64 `tfsdk:"humidity_low"`
	HumidityHigh    types.Float64 `tfsdk:"humidity_high"`
	HumidityMargin  types.Float64 `tfsdk:"humidity_margin"`

	LightEnabled types.Bool    `tfsdk:"light_enabled"`
	LightLow     types.Float64 `tfsdk:"light_low"`
	LightHigh    types.Float64 `tfsdk:"light_high"`
	LightMargin  types.Float64 `tfsdk:"light_margin"`

	AlarmEnabled        types.Bool `tfsdk:"alarm_enabled"`
	LeakInternalEnabled types.Bool `tfsdk:"leak_internal_enabled"`
	LeakExternalEnabled types.Bool `tfsdk:"leak_external_enabled"`
	LedEnabled          types.Bool `tfsdk:"led_enabled"`

	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	Model            types.String `tfsdk:"model"`
	Type             types.String `tfsdk:"type"`
	State            types.String `tfsdk:"state"`
	Host             types.String `tfsdk:"host"`
	IsAdopted        types.Bool   `tfsdk:"is_adopted"`
	IsOpened         types.Bool   `tfsdk:"is_opened"`
	IsMotionDetected types.Bool   `tfsdk:"is_motion_detected"`
}

func (r *protectSensorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_sensor"
}

func (r *protectSensorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	pct := []validator.Float64{float64validator.Between(0, 100)}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect UP-Sense multi-sensor, identified by MAC. Configures the existing, adopted device over " +
			"the private Protect API (`/proxy/protect/api/sensors/<id>`): name, mount type, paired camera, and the per-capability " +
			"settings (motion, temperature, humidity, ambient light, smoke/CO alarm, leak, and status LED). `terraform destroy` removes " +
			"the sensor from state and leaves it configured; set `forget_on_destroy = true` to unadopt it. " +
			"**Config writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Sensor MAC address (the natural key); any separator/case is accepted and normalized. The device must be adopted. Immutable — changing it replaces the resource.",
				PlanModifiers:       []planmodifier.String{normalizeMAC(), stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Sensor display name."},
			"mount_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Physical mount type: `door`, `window`, `garage`, `leak`, or `none`.",
				Validators:          []validator.String{stringvalidator.OneOf("door", "window", "garage", "leak", "none")},
			},
			"camera": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Object id of the camera paired to this sensor."},

			"motion_enabled":     schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether motion detection is enabled."},
			"motion_sensitivity": schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Motion sensitivity (0–100).", Validators: pct},

			"temperature_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether temperature alerting is enabled."},
			"temperature_low":     schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Low temperature alert threshold (°C)."},
			"temperature_high":    schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "High temperature alert threshold (°C)."},
			"temperature_margin":  schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Temperature hysteresis margin (°C)."},

			"humidity_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether humidity alerting is enabled."},
			"humidity_low":     schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Low relative-humidity alert threshold (%)."},
			"humidity_high":    schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "High relative-humidity alert threshold (%)."},
			"humidity_margin":  schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Humidity hysteresis margin (%)."},

			"light_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether ambient-light alerting is enabled."},
			"light_low":     schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Low ambient-light alert threshold (lux)."},
			"light_high":    schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "High ambient-light alert threshold (lux)."},
			"light_margin":  schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Ambient-light hysteresis margin (lux)."},

			"alarm_enabled":         schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the smoke/CO alarm listener is enabled."},
			"leak_internal_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the internal (onboard) leak sensor is enabled."},
			"leak_external_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the external leak-probe input is enabled."},
			"led_enabled":           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the status LED is enabled."},

			"forget_on_destroy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When `true`, `terraform destroy` unadopts (forgets) the sensor instead of just removing it from state. Defaults to `false`.",
			},

			"model":              schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model."},
			"type":               schema.StringAttribute{Computed: true, MarkdownDescription: "Device type identifier."},
			"state":              schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state."},
			"host":               schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the sensor is adopted by this console."},
			"is_opened":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the door/window/garage contact currently reads open."},
			"is_motion_detected": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether motion is currently detected."},
		},
	}
}

func (r *protectSensorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

func (r *protectSensorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectSensorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac := strings.TrimSpace(plan.MAC.ValueString())
	current, err := r.protect.Sensors().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Sensor not found",
				fmt.Sprintf("No sensor with MAC %q is adopted on this Protect console. Adopt it first.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up sensor", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectSensorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectSensorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.protect.Sensors().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read sensor", err.Error())
		return
	}
	state.fromSensor(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectSensorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectSensorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.protect.Sensors().Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read sensor before update", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectSensorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectSensorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.protect.Sensors().Delete(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
			resp.Diagnostics.AddError("Failed to unadopt sensor", err.Error())
		}
	}
}

func (r *protectSensorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if strings.Contains(id, ":") {
		s, err := r.protect.Sensors().GetByMAC(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no sensor with MAC %q: %s", id, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), s.Id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), macCanonical(s.Mac))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *protectSensorResource) applyConfig(ctx context.Context, plan *protectSensorResourceModel, current *wire.Sensor, diags *diag.Diagnostics) {
	cfg := sensorConfig{
		Name:      mergeStr(plan.Name, current.Name),
		MountType: mergeStr(plan.MountType, (*string)(current.MountType)),
		Camera:    mergeStr(plan.Camera, current.Camera),
	}
	if m := (&sensorToggleSensitivity{
		IsEnabled:   mergeBool(plan.MotionEnabled, current.MotionSettings.IsEnabled),
		Sensitivity: mergeF64(plan.MotionSensitivity, current.MotionSettings.Sensitivity),
	}); m.IsEnabled != nil || m.Sensitivity != nil {
		cfg.MotionSettings = m
	}
	cfg.TemperatureSettings = mergeThreshold(plan.TemperatureEnabled, plan.TemperatureLow, plan.TemperatureHigh, plan.TemperatureMargin,
		current.TemperatureSettings.IsEnabled, current.TemperatureSettings.LowThreshold, current.TemperatureSettings.HighThreshold, current.TemperatureSettings.Margin)
	cfg.HumiditySettings = mergeThreshold(plan.HumidityEnabled, plan.HumidityLow, plan.HumidityHigh, plan.HumidityMargin,
		current.HumiditySettings.IsEnabled, current.HumiditySettings.LowThreshold, current.HumiditySettings.HighThreshold, current.HumiditySettings.Margin)
	cfg.LightSettings = mergeThreshold(plan.LightEnabled, plan.LightLow, plan.LightHigh, plan.LightMargin,
		current.LightSettings.IsEnabled, current.LightSettings.LowThreshold, current.LightSettings.HighThreshold, current.LightSettings.Margin)
	if a := mergeBool(plan.AlarmEnabled, current.AlarmSettings.IsEnabled); a != nil {
		cfg.AlarmSettings = &sensorEnabled{IsEnabled: a}
	}
	if li := (&sensorLeakConfig{
		IsInternalEnabled: mergeBool(plan.LeakInternalEnabled, current.LeakSettings.IsInternalEnabled),
		IsExternalEnabled: mergeBool(plan.LeakExternalEnabled, current.LeakSettings.IsExternalEnabled),
	}); li.IsInternalEnabled != nil || li.IsExternalEnabled != nil {
		cfg.LeakSettings = li
	}
	if l := mergeBool(plan.LedEnabled, current.LedSettings.IsEnabled); l != nil {
		cfg.LedSettings = &sensorEnabled{IsEnabled: l}
	}

	updated, err := r.protect.Sensors().Update(ctx, current.Id, cfg)
	if err != nil {
		diags.AddError("Failed to apply sensor config", err.Error())
		return
	}
	plan.fromSensor(updated)
}

func (m *protectSensorResourceModel) fromSensor(s *wire.Sensor) {
	m.ID = types.StringValue(s.Id)
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(macCanonical(s.Mac))
	}
	m.Name = strToStateOptional(deref(s.Name))
	m.MountType = strToStateOptional(string(derefEnum(s.MountType)))
	m.Camera = strToStateOptional(deref(s.Camera))

	m.MotionEnabled = boolToState(s.MotionSettings.IsEnabled)
	m.MotionSensitivity = f64PtrToState(s.MotionSettings.Sensitivity)
	m.TemperatureEnabled = boolToState(s.TemperatureSettings.IsEnabled)
	m.TemperatureLow = f64PtrToState(s.TemperatureSettings.LowThreshold)
	m.TemperatureHigh = f64PtrToState(s.TemperatureSettings.HighThreshold)
	m.TemperatureMargin = f64PtrToState(s.TemperatureSettings.Margin)
	m.HumidityEnabled = boolToState(s.HumiditySettings.IsEnabled)
	m.HumidityLow = f64PtrToState(s.HumiditySettings.LowThreshold)
	m.HumidityHigh = f64PtrToState(s.HumiditySettings.HighThreshold)
	m.HumidityMargin = f64PtrToState(s.HumiditySettings.Margin)
	m.LightEnabled = boolToState(s.LightSettings.IsEnabled)
	m.LightLow = f64PtrToState(s.LightSettings.LowThreshold)
	m.LightHigh = f64PtrToState(s.LightSettings.HighThreshold)
	m.LightMargin = f64PtrToState(s.LightSettings.Margin)
	m.AlarmEnabled = boolToState(s.AlarmSettings.IsEnabled)
	m.LeakInternalEnabled = boolToState(s.LeakSettings.IsInternalEnabled)
	m.LeakExternalEnabled = boolToState(s.LeakSettings.IsExternalEnabled)
	m.LedEnabled = boolToState(s.LedSettings.IsEnabled)

	m.Model = strToStateOptional(deref(s.MarketName))
	m.Type = strToStateOptional(deref(s.Type))
	m.State = types.StringValue(string(derefEnum(s.State)))
	m.Host = strToStateOptional(deref(s.Host))
	m.IsAdopted = boolToState(s.IsAdopted)
	m.IsOpened = boolToState(s.IsOpened)
	m.IsMotionDetected = boolToState(s.IsMotionDetected)
}

// mergeThreshold builds a threshold sub-config (enabled + low/high/margin),
// returning nil when nothing in the group is set so the PATCH omits it entirely.
func mergeThreshold(en types.Bool, low, high, margin types.Float64, curEn *bool, curLow, curHigh, curMargin *float64) *sensorThresholdConfig {
	c := &sensorThresholdConfig{
		IsEnabled:     mergeBool(en, curEn),
		LowThreshold:  mergeF64(low, curLow),
		HighThreshold: mergeF64(high, curHigh),
		Margin:        mergeF64(margin, curMargin),
	}
	if c.IsEnabled == nil && c.LowThreshold == nil && c.HighThreshold == nil && c.Margin == nil {
		return nil
	}
	return c
}

// --- owned-field overlay (omitempty) sent on PATCH ---

type sensorConfig struct {
	Name                *string                  `json:"name,omitempty"`
	MountType           *string                  `json:"mountType,omitempty"`
	Camera              *string                  `json:"camera,omitempty"`
	MotionSettings      *sensorToggleSensitivity `json:"motionSettings,omitempty"`
	TemperatureSettings *sensorThresholdConfig   `json:"temperatureSettings,omitempty"`
	HumiditySettings    *sensorThresholdConfig   `json:"humiditySettings,omitempty"`
	LightSettings       *sensorThresholdConfig   `json:"lightSettings,omitempty"`
	AlarmSettings       *sensorEnabled           `json:"alarmSettings,omitempty"`
	LeakSettings        *sensorLeakConfig        `json:"leakSettings,omitempty"`
	LedSettings         *sensorEnabled           `json:"ledSettings,omitempty"`
}

type sensorEnabled struct {
	IsEnabled *bool `json:"isEnabled,omitempty"`
}

type sensorToggleSensitivity struct {
	IsEnabled   *bool    `json:"isEnabled,omitempty"`
	Sensitivity *float64 `json:"sensitivity,omitempty"`
}

type sensorThresholdConfig struct {
	IsEnabled     *bool    `json:"isEnabled,omitempty"`
	LowThreshold  *float64 `json:"lowThreshold,omitempty"`
	HighThreshold *float64 `json:"highThreshold,omitempty"`
	Margin        *float64 `json:"margin,omitempty"`
}

type sensorLeakConfig struct {
	IsInternalEnabled *bool `json:"isInternalEnabled,omitempty"`
	IsExternalEnabled *bool `json:"isExternalEnabled,omitempty"`
}
