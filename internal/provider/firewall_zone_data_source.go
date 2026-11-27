package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
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
	zones, err := d.net.Firewall().Zones().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list firewall zones", err.Error()+
			"\n\nZones exist only on a site that runs the zone-based firewall.")
		return
	}
	found, err := findZone(zones, key, name)
	if err != nil {
		resp.Diagnostics.AddError("Firewall zone not found", err.Error())
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

// findZone returns the one zone matching every non-empty key.
func findZone(zones []wire.FirewallZone, key, name string) (*wire.FirewallZone, error) {
	var matches []*wire.FirewallZone
	for i := range zones {
		if (key == "" || deref(zones[i].ZoneKey) == key) && (name == "" || deref(zones[i].Name) == name) {
			matches = append(matches, &zones[i])
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("no zone matches zone_key=%q name=%q", key, name)
	default:
		return nil, fmt.Errorf("%d zones match zone_key=%q name=%q; narrow the lookup", len(matches), key, name)
	}
}

// uniqueMatch returns the only item satisfying match, or nil for none or
// several: a re-key fallback must never guess between candidates.
func uniqueMatch[T any](items []T, match func(*T) bool) *T {
	var found *T
	for i := range items {
		if !match(&items[i]) {
			continue
		}
		if found != nil {
			return nil
		}
		found = &items[i]
	}
	return found
}
