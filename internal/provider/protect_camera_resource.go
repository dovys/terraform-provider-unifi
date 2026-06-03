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
	_ resource.Resource                = (*protectCameraResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectCameraResource)(nil)
	_ resource.ResourceWithImportState = (*protectCameraResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*protectCameraResource)(nil)
)

func NewProtectCameraResource() resource.Resource { return &protectCameraResource{} }

type protectCameraResource struct {
	protect *protect.Client
}

type protectCameraResourceModel struct {
	ID  types.String `tfsdk:"id"`  // Protect 24-hex id (computed)
	MAC types.String `tfsdk:"mac"` // natural key (required, immutable)

	Name               types.String `tfsdk:"name"`
	MicEnabled         types.Bool   `tfsdk:"mic_enabled"`
	MicVolume          types.Int64  `tfsdk:"mic_volume"`
	StatusLightEnabled types.Bool   `tfsdk:"status_light_enabled"`
	HDRType            types.String `tfsdk:"hdr_type"`
	VideoMode          types.String `tfsdk:"video_mode"`
	RecordingMode      types.String `tfsdk:"recording_mode"`

	OSDNameEnabled  types.Bool `tfsdk:"osd_name_enabled"`
	OSDDateEnabled  types.Bool `tfsdk:"osd_date_enabled"`
	OSDLogoEnabled  types.Bool `tfsdk:"osd_logo_enabled"`
	OSDDebugEnabled types.Bool `tfsdk:"osd_debug_enabled"`

	SmartDetectObjectTypes types.Set `tfsdk:"smart_detect_object_types"`
	SmartDetectAudioTypes  types.Set `tfsdk:"smart_detect_audio_types"`

	// Doorbell-only LCD message (gated by the camera's capability at apply time).
	LCDMessageType types.String `tfsdk:"lcd_message_type"`
	LCDMessageText types.String `tfsdk:"lcd_message_text"`

	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	// Computed telemetry (read-only; never part of the plan).
	Model      types.String `tfsdk:"model"`
	Type       types.String `tfsdk:"type"`
	State      types.String `tfsdk:"state"`
	Host       types.String `tfsdk:"host"`
	IsAdopted  types.Bool   `tfsdk:"is_adopted"`
	HasSpeaker types.Bool   `tfsdk:"has_speaker"`
}

func (r *protectCameraResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_camera"
}

func (r *protectCameraResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect camera (doorbells are cameras), identified by MAC. Configures the existing, " +
			"adopted camera over the private Protect API (`/proxy/protect/api/cameras/<id>`): name, microphone, status LED, " +
			"on-screen-display overlays, HDR/video mode, smart-detection types, recording mode, and the doorbell LCD message. " +
			"Read-only telemetry (state, model, adoption) is exposed as computed. `terraform destroy` removes the camera from " +
			"state and leaves it configured as last-applied; set `forget_on_destroy = true` to unadopt it. " +
			"**Config writes and unadopt are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Camera MAC address (the natural key); any separator/case is accepted and normalized to " +
					"lowercase colon-separated form. The camera must be adopted and visible to Protect. " +
					"Immutable — changing it replaces the resource.",
				// Normalize before RequiresReplace so a colon/case variant doesn't force a replace.
				PlanModifiers: []planmodifier.String{normalizeMAC(), stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Camera display name.",
			},
			"mic_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the camera microphone is enabled.",
			},
			"mic_volume": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Microphone volume (0–100).",
				Validators:          []validator.Int64{int64validator.Between(0, 100)},
			},
			"status_light_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the status LED is enabled.",
			},
			"hdr_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "High Dynamic Range mode (e.g. `auto`, `off`); supported values vary by model.",
			},
			"video_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Video mode: `default`, `highFps`, `homekit`, `sport`, `slowShutter`, `lprReflex`, or `lprNoneReflex`.",
				Validators: []validator.String{stringvalidator.OneOf(
					"default", "highFps", "homekit", "sport", "slowShutter", "lprReflex", "lprNoneReflex")},
			},
			"recording_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Recording mode: `always`, `schedule`, `never`, `detections`, or `adaptive`.",
				Validators:          []validator.String{stringvalidator.OneOf("always", "schedule", "never", "detections", "adaptive")},
			},
			"osd_name_enabled":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Show the camera name in the on-screen display."},
			"osd_date_enabled":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Show the date in the on-screen display."},
			"osd_logo_enabled":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Show the logo in the on-screen display."},
			"osd_debug_enabled": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Show debug info in the on-screen display."},
			"smart_detect_object_types": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Smart-detection object types to detect (e.g. `person`, `vehicle`, `animal`, `package`). Model-dependent.",
			},
			"smart_detect_audio_types": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Smart-detection audio types to detect (e.g. `smoke_cmonx`, `alrmSmoke`). Model-dependent.",
			},
			"lcd_message_type": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Doorbell LCD message type: `LEAVE_PACKAGE_AT_DOOR`, `DO_NOT_DISTURB`, `CUSTOM_MESSAGE`, or `IMAGE`. " +
					"Only valid on doorbell cameras.",
				Validators: []validator.String{stringvalidator.OneOf(
					"LEAVE_PACKAGE_AT_DOOR", "DO_NOT_DISTURB", "CUSTOM_MESSAGE", "IMAGE")},
			},
			"lcd_message_text": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Doorbell LCD message text (for `CUSTOM_MESSAGE`).",
			},
			"forget_on_destroy": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "When `true`, `terraform destroy` unadopts (forgets) the camera instead of just removing it " +
					"from state. Defaults to `false`. Guard expensive cameras with `prevent_destroy`.",
			},

			"model":       schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model (e.g. `UVC G4 Doorbell`)."},
			"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Device type identifier."},
			"state":       schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state (`CONNECTED`, `DISCONNECTED`, …)."},
			"host":        schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the camera is adopted by this console."},
			"has_speaker": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the camera has a speaker."},
		},
	}
}

func (r *protectCameraResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

// ModifyPlan gates capability-dependent fields (HDR, mic, status LED, smart
// detection, doorbell LCD) against the camera's live FeatureFlags — invariant 3:
// writing an unsupported field becomes a plan-time diagnostic, not a raw 4xx.
// Those flags are only known after a read, so this can't be ValidateConfig; it
// reads the camera here (the documented ModifyPlan-with-read gate). It inspects
// the *config* (not the plan) so Optional+Computed values carried from state
// don't false-trigger. HIL-unverified — no camera lacking these flags on the
// recon console — but safe: a too-permissive gate is a missing diagnostic, not a
// crash.
func (r *protectCameraResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy (null plan) or an unconfigured provider (validate walk): nothing to gate.
	if req.Plan.Raw.IsNull() || r.protect == nil {
		return
	}
	var cfg protectCameraResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only read the camera when a capability-gated field is actually configured.
	if !cameraConfiguresGatedField(&cfg) {
		return
	}
	mac := strings.TrimSpace(cfg.MAC.ValueString())
	if mac == "" {
		return
	}
	cam, err := r.protect.Cameras().GetByMAC(ctx, mac)
	if err != nil {
		// Camera not readable yet (not adopted, Protect absent, transient): don't
		// block the plan on a check we can't perform — Create surfaces a precise
		// error and Configure already flags an absent Protect.
		return
	}
	for _, v := range cameraCapabilityViolations(&cfg, cam.FeatureFlags) {
		resp.Diagnostics.AddAttributeError(path.Root(v.Attr),
			"Camera does not support "+v.Label,
			fmt.Sprintf("Camera %q does not advertise %s, so %q cannot be set. Remove it or target a capable camera.",
				macCanonical(cam.Mac), v.Label, v.Attr))
	}
}

// capViolation pairs a configured-but-unsupported attribute with the capability
// label named in its diagnostic.
type capViolation struct{ Attr, Label string }

// cameraConfiguresGatedField reports whether the config sets any
// capability-gated field, so ModifyPlan can skip the camera read otherwise.
func cameraConfiguresGatedField(cfg *protectCameraResourceModel) bool {
	return !cfg.HDRType.IsNull() || !cfg.MicEnabled.IsNull() || !cfg.MicVolume.IsNull() ||
		!cfg.StatusLightEnabled.IsNull() ||
		!cfg.SmartDetectObjectTypes.IsNull() || !cfg.SmartDetectAudioTypes.IsNull() ||
		!cfg.LCDMessageType.IsNull() || !cfg.LCDMessageText.IsNull()
}

// cameraCapabilityViolations returns the configured fields the camera's live
// FeatureFlags say it doesn't support. cfg is read from the resource *config*,
// so Optional+Computed values carried from state never false-trigger.
func cameraCapabilityViolations(cfg *protectCameraResourceModel, ff wire.FeatureFlags) []capViolation {
	supports := func(b *bool) bool { return b != nil && *b }
	var v []capViolation
	if !cfg.HDRType.IsNull() && !supports(ff.HasHdr) {
		v = append(v, capViolation{"hdr_type", "HDR"})
	}
	if (!cfg.MicEnabled.IsNull() || !cfg.MicVolume.IsNull()) && !supports(ff.HasMic) {
		v = append(v, capViolation{"mic_enabled", "a microphone"})
	}
	if !cfg.StatusLightEnabled.IsNull() && !supports(ff.HasLedStatus) {
		v = append(v, capViolation{"status_light_enabled", "a status LED"})
	}
	if (!cfg.SmartDetectObjectTypes.IsNull() || !cfg.SmartDetectAudioTypes.IsNull()) && !supports(ff.HasSmartDetect) {
		v = append(v, capViolation{"smart_detect_object_types", "smart detection"})
	}
	if (!cfg.LCDMessageType.IsNull() || !cfg.LCDMessageText.IsNull()) && !supports(ff.HasLcdScreen) {
		v = append(v, capViolation{"lcd_message_type", "a doorbell LCD screen"})
	}
	return v
}

func (r *protectCameraResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectCameraResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac := strings.TrimSpace(plan.MAC.ValueString())
	current, err := r.protect.Cameras().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Camera not found",
				fmt.Sprintf("No camera with MAC %q is adopted on this Protect console. Adopt the camera first.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up camera", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectCameraResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectCameraResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cam, err := r.protect.Cameras().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read camera", err.Error())
		return
	}
	state.fromCamera(ctx, cam, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectCameraResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectCameraResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.protect.Cameras().Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read camera before update", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectCameraResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectCameraResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Safe by default: leave the (physical) camera adopted and configured unless
	// the practitioner opts into forgetting it.
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.protect.Cameras().Forget(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
			resp.Diagnostics.AddError("Failed to unadopt camera", err.Error())
		}
	}
}

func (r *protectCameraResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	// Accept a MAC (contains ':') or the Protect 24-hex id.
	if strings.Contains(id, ":") {
		cam, err := r.protect.Cameras().GetByMAC(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no camera with MAC %q: %s", id, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), cam.Id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), macCanonical(cam.Mac))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// applyConfig PATCHes the plan's owned fields onto the camera and refreshes the
// model in place. Each owned sub-object is built by merging the plan over the
// current camera's values, so a PATCH never clears a sibling field the provider
// also models (the Protect analogue of GET-merge-PUT).
func (r *protectCameraResource) applyConfig(ctx context.Context, plan *protectCameraResourceModel, current *wire.Camera, diags *diag.Diagnostics) {
	cfg := cameraConfig{
		Name:         mergeStr(plan.Name, current.Name),
		IsMicEnabled: mergeBool(plan.MicEnabled, current.IsMicEnabled),
		MicVolume:    mergeFloat(plan.MicVolume, current.MicVolume),
		HdrType:      mergeStr(plan.HDRType, current.HdrType),
		VideoMode:    mergeStr(plan.VideoMode, (*string)(current.VideoMode)),
	}
	// Attach an owned sub-object only when it carries a value, so a PATCH never
	// sends a bare "{}" the controller might read as "clear" (e.g. a camera with
	// no smart-detect support). Each field is merged plan-over-current so a
	// sibling the provider models but the practitioner didn't set is preserved.
	if led := (&ledConfig{IsEnabled: mergeBool(plan.StatusLightEnabled, current.LedSettings.IsEnabled)}); led.IsEnabled != nil {
		cfg.LedSettings = led
	}
	osd := &osdConfig{
		IsNameEnabled:  mergeBool(plan.OSDNameEnabled, current.OsdSettings.IsNameEnabled),
		IsDateEnabled:  mergeBool(plan.OSDDateEnabled, current.OsdSettings.IsDateEnabled),
		IsLogoEnabled:  mergeBool(plan.OSDLogoEnabled, current.OsdSettings.IsLogoEnabled),
		IsDebugEnabled: mergeBool(plan.OSDDebugEnabled, current.OsdSettings.IsDebugEnabled),
	}
	if osd.IsNameEnabled != nil || osd.IsDateEnabled != nil || osd.IsLogoEnabled != nil || osd.IsDebugEnabled != nil {
		cfg.OsdSettings = osd
	}
	if mode := mergeStr(plan.RecordingMode, (*string)(current.RecordingSettings.Mode)); mode != nil {
		cfg.RecordingSettings = &recordingConfig{Mode: mode}
	}
	objs := mergeStrSet(ctx, plan.SmartDetectObjectTypes, ifaceStrings(current.SmartDetectSettings.ObjectTypes), diags)
	auds := mergeStrSet(ctx, plan.SmartDetectAudioTypes, ifaceStrings(current.SmartDetectSettings.AudioTypes), diags)
	if objs != nil || auds != nil {
		cfg.SmartDetectSettings = &smartDetectConfig{ObjectTypes: objs, AudioTypes: auds}
	}
	// LCD message is doorbell-only; only send it when configured.
	if !plan.LCDMessageType.IsNull() || !plan.LCDMessageText.IsNull() {
		cfg.LcdMessage = &lcdConfig{
			Type: strPtrOrNil(plan.LCDMessageType),
			Text: strPtrOrNil(plan.LCDMessageText),
		}
	}
	if diags.HasError() {
		return
	}

	updated, err := r.protect.Cameras().Update(ctx, current.Id, cfg)
	if err != nil {
		diags.AddError("Failed to apply camera config", err.Error())
		return
	}
	plan.fromCamera(ctx, updated, diags)
}

// fromCamera maps a wire Camera onto the model (config reconciled + computed
// telemetry).
func (m *protectCameraResourceModel) fromCamera(ctx context.Context, c *wire.Camera, diags *diag.Diagnostics) {
	m.ID = types.StringValue(c.Id)
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(macCanonical(c.Mac)) // import path; otherwise keep the (already-canonical) configured value
	}
	m.Name = strToStateOptional(deref(c.Name))
	m.MicEnabled = boolToState(c.IsMicEnabled)
	m.MicVolume = float64ToState(c.MicVolume)
	m.StatusLightEnabled = boolToState(c.LedSettings.IsEnabled)
	m.HDRType = strToStateOptional(deref(c.HdrType))
	m.VideoMode = strToStateOptional(string(derefEnum(c.VideoMode)))
	m.RecordingMode = strToStateOptional(string(derefEnum(c.RecordingSettings.Mode)))
	m.OSDNameEnabled = boolToState(c.OsdSettings.IsNameEnabled)
	m.OSDDateEnabled = boolToState(c.OsdSettings.IsDateEnabled)
	m.OSDLogoEnabled = boolToState(c.OsdSettings.IsLogoEnabled)
	m.OSDDebugEnabled = boolToState(c.OsdSettings.IsDebugEnabled)
	m.SmartDetectObjectTypes = sliceToStringSet(ifaceStrings(c.SmartDetectSettings.ObjectTypes))
	m.SmartDetectAudioTypes = sliceToStringSet(ifaceStrings(c.SmartDetectSettings.AudioTypes))

	m.Model = strToStateOptional(deref(c.MarketName))
	m.Type = strToStateOptional(deref(c.Type))
	m.State = types.StringValue(string(derefEnum(c.State)))
	m.Host = strToStateOptional(deref(c.Host))
	m.IsAdopted = boolToState(c.IsAdopted)
	m.HasSpeaker = boolToState(c.HasSpeaker)
}

// --- owned-field overlay (omitempty) sent on PATCH ---

type cameraConfig struct {
	Name                *string            `json:"name,omitempty"`
	IsMicEnabled        *bool              `json:"isMicEnabled,omitempty"`
	MicVolume           *float64           `json:"micVolume,omitempty"`
	HdrType             *string            `json:"hdrType,omitempty"`
	VideoMode           *string            `json:"videoMode,omitempty"`
	LedSettings         *ledConfig         `json:"ledSettings,omitempty"`
	OsdSettings         *osdConfig         `json:"osdSettings,omitempty"`
	RecordingSettings   *recordingConfig   `json:"recordingSettings,omitempty"`
	SmartDetectSettings *smartDetectConfig `json:"smartDetectSettings,omitempty"`
	LcdMessage          *lcdConfig         `json:"lcdMessage,omitempty"`
}

type ledConfig struct {
	IsEnabled *bool `json:"isEnabled,omitempty"`
}

type osdConfig struct {
	IsNameEnabled  *bool `json:"isNameEnabled,omitempty"`
	IsDateEnabled  *bool `json:"isDateEnabled,omitempty"`
	IsLogoEnabled  *bool `json:"isLogoEnabled,omitempty"`
	IsDebugEnabled *bool `json:"isDebugEnabled,omitempty"`
}

type recordingConfig struct {
	Mode *string `json:"mode,omitempty"`
}

type smartDetectConfig struct {
	ObjectTypes []string `json:"objectTypes,omitempty"`
	AudioTypes  []string `json:"audioTypes,omitempty"`
}

type lcdConfig struct {
	Type *string `json:"type,omitempty"`
	Text *string `json:"text,omitempty"`
}
