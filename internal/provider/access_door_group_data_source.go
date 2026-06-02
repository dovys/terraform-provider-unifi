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
	_ datasource.DataSource              = (*accessDoorGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accessDoorGroupDataSource)(nil)
)

func NewAccessDoorGroupDataSource() datasource.DataSource { return &accessDoorGroupDataSource{} }

type accessDoorGroupDataSource struct {
	access *access.Client
}

type accessDoorGroupDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	GroupType types.String `tfsdk:"group_type"`
	Type      types.String `tfsdk:"type"`

	DoorCount   types.Int64 `tfsdk:"door_count"`
	FloorCount  types.Int64 `tfsdk:"floor_count"`
	PolicyCount types.Int64 `tfsdk:"policy_count"`

	DoorIDs types.Set `tfsdk:"door_ids"`
}

func (d *accessDoorGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_door_group"
}

func (d *accessDoorGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi Access door group by `id` (`unique_id`) or `name` (read-only). Use it to reference a " +
			"door group that Terraform does not manage. **Modeled from the reverse-engineered Access API and not yet verified " +
			"against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Provide `id` or `name`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Door group display name. Provide `name` or `id`. If multiple groups share a name, the first match wins — look up by `id` to disambiguate.",
			},
			"group_type":   schema.StringAttribute{Computed: true, MarkdownDescription: "Grouping kind (e.g. `door`)."},
			"type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Group type identifier."},
			"door_count":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of member doors."},
			"floor_count":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of member floors."},
			"policy_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of access policies referencing the group."},
			"door_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Member door ids (`unique_id` of each door).",
			},
		},
	}
}

func (d *accessDoorGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.access = c.Access()
	}
}

func (d *accessDoorGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accessDoorGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	name := strings.TrimSpace(cfg.Name.ValueString())
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `name` to look up a door group.")
		return
	}

	groups, err := d.access.DoorGroups().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list door groups", err.Error())
		return
	}
	var match *wire.DoorGroupControllerDoorGroupInfo
	for i := range groups {
		g := &groups[i]
		if (id != "" && deref(g.UniqueId) == id) || (id == "" && deref(g.Name) == name) {
			match = g
			break
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Door group not found", "no door group matched the given id/name")
		return
	}

	uid := deref(match.UniqueId)
	cfg.ID = types.StringValue(uid)
	cfg.Name = strToStateOptional(deref(match.Name))
	cfg.GroupType = strToStateOptional(deref(match.GroupType))
	cfg.Type = strToStateOptional(deref(match.Type))
	cfg.DoorCount = int64PtrToState(match.DoorCount)
	cfg.FloorCount = int64PtrToState(match.FloorCount)
	cfg.PolicyCount = int64PtrToState(match.PolicyCount)

	ids, err := d.access.DoorGroups().DoorIDs(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read door group members", err.Error())
		return
	}
	cfg.DoorIDs = sliceToStringSet(ids)

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
