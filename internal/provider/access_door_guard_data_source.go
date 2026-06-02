package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
)

var (
	_ datasource.DataSource              = (*accessDoorGuardDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accessDoorGuardDataSource)(nil)
)

func NewAccessDoorGuardDataSource() datasource.DataSource { return &accessDoorGuardDataSource{} }

type accessDoorGuardDataSource struct {
	access *access.Client
}

type accessDoorGuardDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	DoorID  types.String `tfsdk:"door_id"`
	UserIDs types.Set    `tfsdk:"user_ids"`
}

func (d *accessDoorGuardDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door_guard"
}

func (d *accessDoorGuardDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up the guard user set of a UniFi Access door by `door_id` (read-only). **Modeled from the " +
			"reverse-engineered Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Access object id; equals `door_id`.",
			},
			"door_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The door's `unique_id`.",
			},
			"user_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Guard user ids (the `unique_id` of each Access user).",
			},
		},
	}
}

func (d *accessDoorGuardDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.access = c.Access()
	}
}

func (d *accessDoorGuardDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accessDoorGuardDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.DoorID.ValueString())
	if id == "" {
		resp.Diagnostics.AddError("Missing door_id", "Set `door_id` to look up a door's guard set.")
		return
	}

	g, err := d.access.DoorGuards().Get(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read door guard", err.Error())
		return
	}

	cfg.ID = types.StringValue(id)
	cfg.DoorID = types.StringValue(id)
	cfg.UserIDs = sliceToStringSet(derefSlice(g.UserIds))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
