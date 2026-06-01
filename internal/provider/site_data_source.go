package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ datasource.DataSource              = (*siteDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*siteDataSource)(nil)
)

func NewSiteDataSource() datasource.DataSource { return &siteDataSource{} }

type siteDataSource struct {
	client unifi.Client
}

type siteDataSourceModel struct {
	Name              types.String `tfsdk:"name"`
	ID                types.String `tfsdk:"id"`
	DisplayName       types.String `tfsdk:"display_name"`
	InternalReference types.String `tfsdk:"internal_reference"`
}

func (d *siteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_site"
}

func (d *siteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi site by its slug (internal reference). Defaults to the provider's configured site.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Site slug (internal reference, e.g. `default`). Defaults to the provider's `site`.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Integration site UUID.",
			},
			"display_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable site name (e.g. `Default`).",
			},
			"internal_reference": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Site slug used by the legacy/v2 surfaces.",
			},
		},
	}
}

func (d *siteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.client = c
	}
}

func (d *siteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model siteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	slug := strOrEmpty(model.Name)
	if slug == "" {
		slug = d.client.Site()
	}

	sites, err := d.client.ListSites(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list sites", err.Error())
		return
	}
	for _, s := range sites {
		if s.InternalReference == slug {
			model.Name = types.StringValue(slug)
			model.ID = types.StringValue(s.ID)
			model.DisplayName = types.StringValue(s.Name)
			model.InternalReference = types.StringValue(s.InternalReference)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.Diagnostics.AddError("Site not found", fmt.Sprintf("No site with internal reference %q.", slug))
}
