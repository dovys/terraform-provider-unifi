package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ datasource.DataSource              = (*applicationInfoDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationInfoDataSource)(nil)
)

func NewApplicationInfoDataSource() datasource.DataSource { return &applicationInfoDataSource{} }

type applicationInfoDataSource struct {
	client unifi.Client
}

type applicationInfoDataSourceModel struct {
	Version types.String `tfsdk:"version"`
}

func (d *applicationInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_application_info"
}

func (d *applicationInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Detected UniFi Network application info, from the Configure-time probe.",
		Attributes: map[string]schema.Attribute{
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Detected Network application version (e.g. `10.4.57`).",
			},
		},
	}
}

func (d *applicationInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.client = c
	}
}

func (d *applicationInfoDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	caps := d.client.Capabilities()
	model := applicationInfoDataSourceModel{
		Version: types.StringValue(caps.VersionRaw),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
