package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

var (
	_ datasource.DataSource              = (*protectCameraDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*protectCameraDataSource)(nil)
)

func NewProtectCameraDataSource() datasource.DataSource { return &protectCameraDataSource{} }

type protectCameraDataSource struct {
	protect *protect.Client
}

type protectCameraDataSourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name          types.String `tfsdk:"name"`
	Model         types.String `tfsdk:"model"`
	Type          types.String `tfsdk:"type"`
	State         types.String `tfsdk:"state"`
	Host          types.String `tfsdk:"host"`
	IsAdopted     types.Bool   `tfsdk:"is_adopted"`
	HasSpeaker    types.Bool   `tfsdk:"has_speaker"`
	MicEnabled    types.Bool   `tfsdk:"mic_enabled"`
	VideoMode     types.String `tfsdk:"video_mode"`
	RecordingMode types.String `tfsdk:"recording_mode"`
}

func (d *protectCameraDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_camera"
}

func (d *protectCameraDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi Protect camera by `mac` or `id` (read-only). Use it to reference a camera " +
			"that Terraform does not manage (e.g. as the target of a liveview).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Protect object id. Provide `id` or `mac`.",
			},
			"mac": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Camera MAC address. Provide `mac` or `id`.",
			},
			"name":           schema.StringAttribute{Computed: true, MarkdownDescription: "Camera display name."},
			"model":          schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model / type."},
			"type":           schema.StringAttribute{Computed: true, MarkdownDescription: "Device type identifier."},
			"state":          schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state."},
			"host":           schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the camera is adopted."},
			"has_speaker":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the camera has a speaker."},
			"mic_enabled":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the microphone is enabled."},
			"video_mode":     schema.StringAttribute{Computed: true, MarkdownDescription: "Current video mode."},
			"recording_mode": schema.StringAttribute{Computed: true, MarkdownDescription: "Current recording mode."},
		},
	}
}

func (d *protectCameraDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.protect = c.Protect()
	}
}

func (d *protectCameraDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg protectCameraDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	mac := strings.TrimSpace(cfg.MAC.ValueString())
	if id == "" && mac == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `mac` to look up a camera.")
		return
	}

	var (
		cam *wire.Camera
		err error
	)
	if id != "" {
		cam, err = d.protect.Cameras().Get(ctx, id)
	} else {
		cam, err = d.protect.Cameras().GetByMAC(ctx, mac)
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to find camera", err.Error())
		return
	}

	cfg.ID = types.StringValue(cam.Id)
	cfg.MAC = types.StringValue(macCanonical(cam.Mac))
	cfg.Name = strToStateOptional(deref(cam.Name))
	cfg.Model = strToStateOptional(deref(cam.MarketName))
	cfg.Type = strToStateOptional(deref(cam.Type))
	cfg.State = types.StringValue(string(derefEnum(cam.State)))
	cfg.Host = strToStateOptional(deref(cam.Host))
	cfg.IsAdopted = boolToState(cam.IsAdopted)
	cfg.HasSpeaker = boolToState(cam.HasSpeaker)
	cfg.MicEnabled = boolToState(cam.IsMicEnabled)
	cfg.VideoMode = strToStateOptional(string(derefEnum(cam.VideoMode)))
	cfg.RecordingMode = strToStateOptional(string(derefEnum(cam.RecordingSettings.Mode)))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
