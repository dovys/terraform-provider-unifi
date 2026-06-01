package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ datasource.DataSource              = (*deviceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*deviceDataSource)(nil)
)

func NewDeviceDataSource() datasource.DataSource { return &deviceDataSource{} }

type deviceDataSource struct {
	client unifi.Client
}

// deviceDataSourceModel is the read-only view: look up by mac or id, expose the
// full device including every port_overrides entry (not just managed ports).
type deviceDataSourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name                       types.String `tfsdk:"name"`
	LEDOverride                types.String `tfsdk:"led_override"`
	LEDOverrideColor           types.String `tfsdk:"led_override_color"`
	LEDOverrideColorBrightness types.Int64  `tfsdk:"led_override_color_brightness"`
	MgmtNetworkID              types.String `tfsdk:"mgmt_network_id"`
	PortOverrides              types.Map    `tfsdk:"port_overrides"`

	Model   types.String `tfsdk:"model"`
	Type    types.String `tfsdk:"type"`
	Version types.String `tfsdk:"version"`
	IP      types.String `tfsdk:"ip"`
	State   types.Int64  `tfsdk:"state"`
	Adopted types.Bool   `tfsdk:"adopted"`
}

func (d *deviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_device"
}

func (d *deviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Look up a UniFi network device by `mac` or `id` (one is required) and read its config and " +
			"telemetry. Read-only — use `unifi_network_device` (resource) to manage one.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Provide this or `mac`.",
			},
			"mac": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Device MAC address. Provide this or `id`.",
			},
			"name":                          dschema.StringAttribute{Computed: true, MarkdownDescription: "Device display name."},
			"led_override":                  dschema.StringAttribute{Computed: true, MarkdownDescription: "Status-LED behavior."},
			"led_override_color":            dschema.StringAttribute{Computed: true, MarkdownDescription: "LED color (hex)."},
			"led_override_color_brightness": dschema.Int64Attribute{Computed: true, MarkdownDescription: "LED brightness (0–100)."},
			"mgmt_network_id":               dschema.StringAttribute{Computed: true, MarkdownDescription: "Management network id."},
			"port_overrides": dschema.MapNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every per-port override the controller has, keyed by port number.",
				NestedObject:        dschema.NestedAttributeObject{Attributes: portOverrideDataSchemaAttrs()},
			},
			"model":   dschema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model code."},
			"type":    dschema.StringAttribute{Computed: true, MarkdownDescription: "Device type (`uap`, `usw`, `ugw`, `udm`, …)."},
			"version": dschema.StringAttribute{Computed: true, MarkdownDescription: "Running firmware version."},
			"ip":      dschema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP."},
			"state":   dschema.Int64Attribute{Computed: true, MarkdownDescription: "Controller state code (`1` = connected)."},
			"adopted": dschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the device is adopted."},
		},
	}
}

func (d *deviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.client = c
	}
}

func (d *deviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg deviceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	mac := strings.TrimSpace(cfg.MAC.ValueString())
	if id == "" && mac == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `mac` to look up a device.")
		return
	}

	var (
		dev *unifi.Device
		err error
	)
	if id != "" {
		dev, err = d.client.GetDevice(ctx, id)
	} else {
		dev, err = d.client.GetDeviceByMAC(ctx, mac)
	}
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Device not found", fmt.Sprintf("no device matched id=%q mac=%q", id, mac))
			return
		}
		resp.Diagnostics.AddError("Failed to read device", err.Error())
		return
	}

	cfg.ID = types.StringValue(dev.ID)
	cfg.MAC = types.StringValue(dev.MAC)
	cfg.Name = strToStateOptional(dev.Name)
	cfg.LEDOverride = strToStateOptional(dev.LEDOverride)
	cfg.LEDOverrideColor = strToStateOptional(dev.LEDOverrideColor)
	cfg.LEDOverrideColorBrightness = intToState(dev.LEDOverrideColorBrightness)
	cfg.MgmtNetworkID = strToStateOptional(dev.MgmtNetworkID)
	cfg.Model = types.StringValue(dev.Model)
	cfg.Type = types.StringValue(dev.Type)
	cfg.Version = types.StringValue(dev.Version)
	cfg.IP = types.StringValue(dev.IP)
	cfg.State = intToState(dev.State)
	cfg.Adopted = boolToState(dev.Adopted)

	// Expose every port the controller has an override for.
	keys := make([]int, 0, len(dev.PortOverrides))
	for _, po := range dev.PortOverrides {
		keys = append(keys, po.PortIdx)
	}
	sort.Ints(keys)
	cfg.PortOverrides = buildPortOverridesMap(ctx, dev.PortOverrides, keys, nil, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// portOverrideDataSchemaAttrs is the Computed-only variant of the port-override
// nested attributes for the data source, derived from the shared object type so
// the resource and data source can't drift.
func portOverrideDataSchemaAttrs() map[string]dschema.Attribute {
	out := map[string]dschema.Attribute{}
	for name, t := range portOverrideAttrTypes {
		switch {
		case t.Equal(types.BoolType):
			out[name] = dschema.BoolAttribute{Computed: true}
		case t.Equal(types.Int64Type):
			out[name] = dschema.Int64Attribute{Computed: true}
		case t.Equal(types.SetType{ElemType: types.StringType}):
			out[name] = dschema.SetAttribute{Computed: true, ElementType: types.StringType}
		default:
			out[name] = dschema.StringAttribute{Computed: true}
		}
	}
	return out
}
