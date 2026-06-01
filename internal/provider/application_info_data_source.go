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
	Version           types.String `tfsdk:"version"`
	IntegrationWrites types.Bool   `tfsdk:"integration_writes"`
	SiteID            types.String `tfsdk:"site_id"`
}

func (d *applicationInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_application_info"
}

func (d *applicationInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Detected UniFi Network application info and capabilities, from the Configure-time probe.",
		Attributes: map[string]schema.Attribute{
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Detected Network application version (e.g. `10.4.57`).",
			},
			"integration_writes": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the firmware supports Integration API writes (version floor met).",
			},
			"site_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Integration UUID of the configured site (empty if the Integration API was unavailable).",
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
		Version:           types.StringValue(caps.VersionRaw),
		IntegrationWrites: types.BoolValue(caps.Flags.IntegrationWrites),
		SiteID:            types.StringValue(d.client.SiteID()),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
