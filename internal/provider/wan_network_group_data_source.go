package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

// wanNetworkGroupDataSource is the user's literal naming example,
// unifi_network_wan_network_group (app=network, group=wan, resource=
// network_group), on the rebuilt client (c.Network().WAN().ListNetworkGroups()).
// WAN network groups (WAN, WAN2, …) are read-only on the v2 surface — the
// gateway derives them from the WAN networkconf — so this is a data source, not
// a managed resource; the WAN interface itself is configured via
// unifi_network_wan (networkconf).
type wanNetworkGroupDataSource struct {
	net *network.Client
}

var (
	_ datasource.DataSource              = (*wanNetworkGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*wanNetworkGroupDataSource)(nil)
)

func NewWANNetworkGroupDataSource() datasource.DataSource { return &wanNetworkGroupDataSource{} }

type wanNetworkGroupDataSourceModel struct {
	Name              types.String `tfsdk:"name"`
	ID                types.String `tfsdk:"id"`
	NetworkID         types.String `tfsdk:"network_id"`
	LoadBalancingMode types.String `tfsdk:"load_balancing_mode"`
	Priority          types.Int64  `tfsdk:"priority"`
	WANMagicEnabled   types.Bool   `tfsdk:"wan_magic_enabled"`
	IsMobileBroadband types.Bool   `tfsdk:"is_mobile_broadband"`
}

func (d *wanNetworkGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wan_network_group"
}

func (d *wanNetworkGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi WAN network group (e.g. `WAN`, `WAN2`) by name (`wan/networkgroups`, v2). " +
			"WAN groups are derived from the WAN networks the gateway has; this reads their load-balancing role and binding.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "WAN network group name to look up (e.g. `WAN`, `WAN2`).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "WAN network group id (e.g. `WAN`, `WAN2`).",
			},
			"network_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`_id` of the bound WAN network (networkconf).",
			},
			"load_balancing_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Load-balancing role of this WAN (e.g. failover/weighted).",
			},
			"priority": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Failover priority.",
			},
			"wan_magic_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether WAN Magic is enabled on this WAN.",
			},
			"is_mobile_broadband": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this WAN is a mobile-broadband (cellular) link.",
			},
		},
	}
}

func (d *wanNetworkGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.net = c.Network()
	}
}

func (d *wanNetworkGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model wanNetworkGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	want := model.Name.ValueString()

	groups, err := d.net.WAN().ListNetworkGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list WAN network groups", err.Error())
		return
	}
	for i := range groups {
		g := groups[i]
		if g.Name == nil || *g.Name != want {
			continue
		}
		model.ID = types.StringValue(string(derefEnum(g.Id)))
		model.NetworkID = strToStateOptional(deref(g.NetworkId))
		model.LoadBalancingMode = strToStateOptional(string(derefEnum(g.LoadBalancingMode)))
		model.Priority = intToState(g.Priority)
		model.WANMagicEnabled = boolToState(g.WanMagicEnabled)
		model.IsMobileBroadband = boolToState(g.IsMobileBroadband)
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		return
	}
	resp.Diagnostics.AddError("WAN network group not found",
		fmt.Sprintf("No WAN network group named %q on this site.", want))
}
