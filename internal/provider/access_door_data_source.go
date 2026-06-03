package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

var (
	_ datasource.DataSource              = (*accessDoorDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accessDoorDataSource)(nil)
)

func NewAccessDoorDataSource() datasource.DataSource { return &accessDoorDataSource{} }

type accessDoorDataSource struct {
	access *access.Client
}

type accessDoorDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	FullName     types.String `tfsdk:"full_name"`
	FloorID      types.String `tfsdk:"floor_id"`
	LocationType types.String `tfsdk:"location_type"`
	ExtraType    types.String `tfsdk:"extra_type"`
	Timezone     types.String `tfsdk:"timezone"`
	IsOpen       types.Bool   `tfsdk:"is_open"`
	DpsConnected types.Bool   `tfsdk:"dps_connected"`
}

func (d *accessDoorDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door"
}

func (d *accessDoorDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi Access door by `id` (`unique_id`) or `name` (read-only). Use it to obtain a door's id " +
			"for `unifi_access_door_group` / `unifi_access_door` / `unifi_access_door_guard`. **Modeled from the " +
			"reverse-engineered Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Provide `id` or `name`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Door display name. Provide `name` or `id`. If multiple doors share a name, the first match wins.",
			},
			"full_name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Full location path of the door."},
			"floor_id":      schema.StringAttribute{Computed: true, MarkdownDescription: "`unique_id` of the door's parent floor."},
			"location_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Location type of the door."},
			"extra_type":    schema.StringAttribute{Computed: true, MarkdownDescription: "Door type tag."},
			"timezone":      schema.StringAttribute{Computed: true, MarkdownDescription: "Door timezone."},
			"is_open":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the door is currently open (live state)."},
			"dps_connected": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a door-position sensor is connected (live state)."},
		},
	}
}

func (d *accessDoorDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.access = c.Access()
	}
}

func (d *accessDoorDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accessDoorDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	name := strings.TrimSpace(cfg.Name.ValueString())
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `name` to look up a door.")
		return
	}

	doors, err := d.access.Doors().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list doors", err.Error())
		return
	}
	var match *wire.LocationBoLocation
	var door *wire.LocationBoDoor
	for i := range doors {
		loc := doors[i].Location
		if loc == nil {
			continue
		}
		if (id != "" && deref(loc.UniqueId) == id) || (id == "" && deref(loc.Name) == name) {
			match = loc
			door = &doors[i]
			break
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Door not found", "no door matched the given id/name")
		return
	}

	cfg.ID = types.StringValue(deref(match.UniqueId))
	cfg.Name = strToStateOptional(deref(match.Name))
	cfg.FullName = strToStateOptional(deref(match.FullName))
	cfg.FloorID = strToStateOptional(deref(match.UpId))
	cfg.LocationType = strToStateOptional(deref(match.LocationType))
	cfg.ExtraType = strToStateOptional(deref(match.ExtraType))
	cfg.Timezone = strToStateOptional(deref(match.Timezone))
	cfg.IsOpen = boolToState(door.IsOpen)
	cfg.DpsConnected = boolToState(door.DpsConnected)

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
