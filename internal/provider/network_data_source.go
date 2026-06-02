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
	_ datasource.DataSource              = (*networkDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*networkDataSource)(nil)
)

func NewNetworkDataSource() datasource.DataSource { return &networkDataSource{} }

type networkDataSource struct {
	net *network.Client
}

type networkDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Purpose      types.String `tfsdk:"purpose"`
	VLANID       types.Int64  `tfsdk:"vlan_id"`
	Subnet       types.String `tfsdk:"subnet"`
	NetworkGroup types.String `tfsdk:"network_group"`
}

func (d *networkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_network"
}

func (d *networkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a single UniFi network by `id` or `name`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Controller object ID. Provide `id` or `name`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Network name. Provide `id` or `name`.",
			},
			"purpose":       schema.StringAttribute{Computed: true, MarkdownDescription: "Network purpose."},
			"vlan_id":       schema.Int64Attribute{Computed: true, MarkdownDescription: "VLAN ID (null if untagged)."},
			"subnet":        schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway IP + prefix (CIDR)."},
			"network_group": schema.StringAttribute{Computed: true, MarkdownDescription: "Controller network group."},
		},
	}
}

func (d *networkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.net = c.Network()
	}
}

func (d *networkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg networkDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := strOrEmpty(cfg.ID)
	name := strOrEmpty(cfg.Name)
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `name`.")
		return
	}

	var found *wire.Networkconf
	if id != "" {
		n, err := d.net.Networks().Get(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read network", err.Error())
			return
		}
		found = n
	} else {
		all, err := d.net.Networks().List(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Failed to list networks", err.Error())
			return
		}
		for i := range all {
			if deref(all[i].Name) == name {
				found = &all[i]
				break
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("Network not found", fmt.Sprintf("No network named %q.", name))
			return
		}
	}

	cfg.ID = types.StringValue(deref(found.UnderscoreId))
	cfg.Name = types.StringValue(deref(found.Name))
	cfg.Purpose = types.StringValue(string(derefEnum(found.Purpose)))
	cfg.Subnet = strToStateOptional(deref(found.IpSubnet))
	cfg.NetworkGroup = types.StringValue(deref(found.Networkgroup))
	if found.VlanEnabled != nil && *found.VlanEnabled && found.Vlan != nil {
		cfg.VLANID = types.Int64Value(int64(*found.Vlan))
	} else {
		cfg.VLANID = types.Int64Null()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
