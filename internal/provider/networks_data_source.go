package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

var (
	_ datasource.DataSource              = (*networksDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*networksDataSource)(nil)
)

func NewNetworksDataSource() datasource.DataSource { return &networksDataSource{} }

type networksDataSource struct {
	net *network.Client
}

type networkSummaryModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Purpose types.String `tfsdk:"purpose"`
	VLANID  types.Int64  `tfsdk:"vlan_id"`
	Subnet  types.String `tfsdk:"subnet"`
}

type networksDataSourceModel struct {
	Networks []networkSummaryModel `tfsdk:"networks"`
}

func (d *networksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_networks"
}

func (d *networksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All UniFi networks on the configured site (LAN/VLAN/guest and WAN).",
		Attributes: map[string]schema.Attribute{
			"networks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The networks.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true},
						"name":    schema.StringAttribute{Computed: true},
						"purpose": schema.StringAttribute{Computed: true},
						"vlan_id": schema.Int64Attribute{Computed: true},
						"subnet":  schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *networksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.net = c.Network()
	}
}

func (d *networksDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	all, err := d.net.Networks().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list networks", err.Error())
		return
	}
	model := networksDataSourceModel{Networks: make([]networkSummaryModel, 0, len(all))}
	for i := range all {
		n := all[i]
		sum := networkSummaryModel{
			ID:      types.StringValue(deref(n.UnderscoreId)),
			Name:    types.StringValue(deref(n.Name)),
			Purpose: types.StringValue(string(derefEnum(n.Purpose))),
			Subnet:  strToStateOptional(deref(n.IpSubnet)),
			VLANID:  types.Int64Null(),
		}
		if n.VlanEnabled != nil && *n.VlanEnabled && n.Vlan != nil {
			sum.VLANID = types.Int64Value(int64(*n.Vlan))
		}
		model.Networks = append(model.Networks, sum)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
