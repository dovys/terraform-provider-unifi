package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

var (
	_ datasource.DataSource              = (*firewallZoneDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*firewallZoneDataSource)(nil)
)

func NewFirewallZoneDataSource() datasource.DataSource { return &firewallZoneDataSource{} }

type firewallZoneDataSource struct {
	net *network.Client
}

type firewallZoneDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ZoneKey     types.String `tfsdk:"zone_key"`
	DefaultZone types.Bool   `tfsdk:"default_zone"`
	NetworkIDs  types.Set    `tfsdk:"network_ids"`
}

func (d *firewallZoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_firewall_zone"
}

func (d *firewallZoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a zone-based firewall zone by `zone_key`, `name`, or both (both must match). " +
			"Built-in zones carry a `zone_key` (`internal`, `external`, `gateway`, `vpn`, `hotspot`, `dmz`); custom zones are found by `name`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Zone `_id`, the `zone_id` a firewall policy references."},
			"zone_key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Built-in zone key, e.g. `internal` or `external`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Zone name.",
			},
			"default_zone": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is a built-in zone."},
			"network_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "`_id`s of the networks in the zone.",
			},
		},
	}
}

func (d *firewallZoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.net = c.Network()
	}
}

func (d *firewallZoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg firewallZoneDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, name := strOrEmpty(cfg.ZoneKey), strOrEmpty(cfg.Name)
	if key == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set `zone_key`, `name`, or both.")
		return
	}
	found, err := d.net.Firewall().Zone(ctx, key, name)
	switch {
	case errors.Is(err, network.ErrNotFound):
		resp.Diagnostics.AddError("Firewall zone not found", fmt.Sprintf(
			"No zone matches zone_key=%q name=%q. Zones exist only on a site that runs the zone-based firewall.", key, name))
		return
	case errors.Is(err, network.ErrAmbiguous):
		resp.Diagnostics.AddError("Firewall zone lookup is ambiguous", fmt.Sprintf(
			"Several zones match zone_key=%q name=%q; set both to narrow it.", key, name))
		return
	case err != nil:
		resp.Diagnostics.AddError("Failed to list firewall zones", err.Error())
		return
	}
	cfg.ID = types.StringValue(deref(found.UnderscoreId))
	cfg.Name = types.StringValue(deref(found.Name))
	cfg.ZoneKey = strToStateOptional(deref(found.ZoneKey))
	cfg.DefaultZone = types.BoolValue(found.DefaultZone != nil && *found.DefaultZone)
	var ids []string
	if found.NetworkIds != nil {
		ids = *found.NetworkIds
	}
	cfg.NetworkIDs = sliceToStringSet(append([]string{}, ids...))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
