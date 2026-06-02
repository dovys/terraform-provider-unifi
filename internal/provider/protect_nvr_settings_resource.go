package provider

import (
	"context"

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
	_ resource.Resource                = (*protectNVRSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectNVRSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*protectNVRSettingsResource)(nil)
)

func NewProtectNVRSettingsResource() resource.Resource { return &protectNVRSettingsResource{} }

type protectNVRSettingsResource struct {
	protect *protect.Client
}

type protectNVRSettingsModel struct {
	ID types.String `tfsdk:"id"`

	Name                 types.String `tfsdk:"name"`
	Timezone             types.String `tfsdk:"timezone"`
	TimeFormat           types.String `tfsdk:"time_format"`
	TemperatureUnit      types.String `tfsdk:"temperature_unit"`
	SSHEnabled           types.Bool   `tfsdk:"ssh_enabled"`
	AutomaticBackups     types.Bool   `tfsdk:"enable_automatic_backups"`
	AutoRetentionEnabled types.Bool   `tfsdk:"auto_retention_enabled"`
	RecordingRetentionMs types.Int64  `tfsdk:"recording_retention_duration_ms"`
	DisableAudio         types.Bool   `tfsdk:"disable_audio"`

	// Computed telemetry.
	Version types.String `tfsdk:"version"`
	Host    types.String `tfsdk:"host"`
}

func (r *protectNVRSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_nvr_settings"
}

func (r *protectNVRSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Console-wide UniFi Protect NVR settings — a **singleton** at `/proxy/protect/api/nvr`. " +
			"Import/update-only: `terraform apply` configures the existing NVR, and `terraform destroy` removes it from state " +
			"without changing the console (the NVR always exists). **Writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect NVR object id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "NVR / console display name."},
			"timezone": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IANA timezone (e.g. `America/New_York`)."},
			"time_format": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Clock format: `12h` or `24h`.",
				Validators:          []validator.String{stringvalidator.OneOf("12h", "24h")},
			},
			"temperature_unit": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Temperature unit: `C` or `F`.",
				Validators:          []validator.String{stringvalidator.OneOf("C", "F")},
			},
			"ssh_enabled":              schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether SSH access to the console is enabled."},
			"enable_automatic_backups": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether automatic configuration backups are enabled."},
			"auto_retention_enabled":   schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the automatic recording-retention balancer is enabled."},
			"recording_retention_duration_ms": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Recording retention duration in milliseconds (used when `auto_retention_enabled` is false).",
			},
			"disable_audio": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether audio recording is disabled console-wide."},

			"version": schema.StringAttribute{Computed: true, MarkdownDescription: "Running UniFi Protect application version."},
			"host":    schema.StringAttribute{Computed: true, MarkdownDescription: "Console management host/IP."},
		},
	}
}

func (r *protectNVRSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.protect = c.Protect()
	}
}

// Create configures the existing (always-present) NVR singleton and records it
// in state — there is no API create.
func (r *protectNVRSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectNVRSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.applyConfig(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectNVRSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectNVRSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nvr, err := r.protect.NVR().Get(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read NVR settings", err.Error())
		return
	}
	state.fromNVR(nvr)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectNVRSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan protectNVRSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.applyConfig(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond state removal: the NVR singleton is never deleted.
func (r *protectNVRSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *protectNVRSettingsResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nvr, err := r.protect.NVR().Get(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Import failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), nvr.Id)...)
}

func (r *protectNVRSettingsResource) applyConfig(ctx context.Context, plan *protectNVRSettingsModel, diags *diag.Diagnostics) {
	current, err := r.protect.NVR().Get(ctx)
	if err != nil {
		diags.AddError("Failed to read NVR settings before update", err.Error())
		return
	}
	// The NVR has a generated write DTO (wire.NvrUpdate, omitempty-tagged), so
	// the owned overlay is the codegen type — not a hand-rolled struct. Fields
	// are merged plan-over-current so an unconfigured-but-modeled field is kept.
	cfg := wire.NvrUpdate{
		Name:                         mergeStr(plan.Name, current.Name),
		Timezone:                     mergeStr(plan.Timezone, current.Timezone),
		TimeFormat:                   (*wire.NvrUpdateTimeFormat)(mergeStr(plan.TimeFormat, (*string)(current.TimeFormat))),
		TemperatureUnit:              (*wire.NvrUpdateTemperatureUnit)(mergeStr(plan.TemperatureUnit, (*string)(current.TemperatureUnit))),
		IsSshEnabled:                 mergeBool(plan.SSHEnabled, current.IsSshEnabled),
		EnableAutomaticBackups:       mergeBool(plan.AutomaticBackups, current.EnableAutomaticBackups),
		AutoRetentionEnabled:         mergeBool(plan.AutoRetentionEnabled, &current.AutoRetentionEnabled),
		RecordingRetentionDurationMs: mergeFloat(plan.RecordingRetentionMs, &current.RecordingRetentionDurationMs),
		DisableAudio:                 mergeBool(plan.DisableAudio, current.DisableAudio),
	}
	updated, err := r.protect.NVR().Update(ctx, &cfg)
	if err != nil {
		diags.AddError("Failed to apply NVR settings", err.Error())
		return
	}
	plan.fromNVR(updated)
}

func (m *protectNVRSettingsModel) fromNVR(n *wire.Nvr) {
	m.ID = types.StringValue(n.Id)
	m.Name = strToStateOptional(deref(n.Name))
	m.Timezone = strToStateOptional(deref(n.Timezone))
	m.TimeFormat = strToStateOptional(string(derefEnum(n.TimeFormat)))
	m.TemperatureUnit = strToStateOptional(string(derefEnum(n.TemperatureUnit)))
	m.SSHEnabled = boolToState(n.IsSshEnabled)
	m.AutomaticBackups = boolToState(n.EnableAutomaticBackups)
	m.AutoRetentionEnabled = types.BoolValue(n.AutoRetentionEnabled)
	m.RecordingRetentionMs = types.Int64Value(int64(n.RecordingRetentionDurationMs))
	m.DisableAudio = boolToState(n.DisableAudio)
	m.Version = strToStateOptional(deref(n.Version))
	m.Host = strToStateOptional(deref(n.Host))
}
