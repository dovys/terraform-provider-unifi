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
	_ datasource.DataSource              = (*accessPolicyDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accessPolicyDataSource)(nil)
)

func NewAccessPolicyDataSource() datasource.DataSource { return &accessPolicyDataSource{} }

type accessPolicyDataSource struct {
	access *access.Client
}

type accessPolicyDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Description types.String `tfsdk:"description"`
	CreatorName types.String `tfsdk:"creator_name"`

	UserCount     types.Int64 `tfsdk:"user_count"`
	DoorCount     types.Int64 `tfsdk:"door_count"`
	ElevatorCount types.Int64 `tfsdk:"elevator_count"`
	GroupCount    types.Int64 `tfsdk:"group_count"`

	ValidStartTime types.String `tfsdk:"valid_start_time"`
	ValidEndTime   types.String `tfsdk:"valid_end_time"`
}

func (d *accessPolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_policy"
}

func (d *accessPolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi Access policy by `id` (`unique_id`) or `name` (read-only). **Modeled from the " +
			"undocumented Access API and not yet verified against a live console — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Access object id (`unique_id`). Provide `id` or `name`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Policy display name. Provide `name` or `id`. If multiple policies share a name, the first match wins.",
			},
			"enabled":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the policy is active."},
			"description":      schema.StringAttribute{Computed: true, MarkdownDescription: "Policy description."},
			"creator_name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the policy creator."},
			"user_count":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of users the policy applies to."},
			"door_count":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of doors granted."},
			"elevator_count":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of elevators granted."},
			"group_count":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of door groups granted."},
			"valid_start_time": schema.StringAttribute{Computed: true, MarkdownDescription: "Policy validity window start."},
			"valid_end_time":   schema.StringAttribute{Computed: true, MarkdownDescription: "Policy validity window end."},
		},
	}
}

func (d *accessPolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.access = c.Access()
	}
}

func (d *accessPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accessPolicyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	name := strings.TrimSpace(cfg.Name.ValueString())
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `name` to look up an access policy.")
		return
	}

	policies, err := d.access.AccessPolicies().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list access policies", err.Error())
		return
	}
	var match *wire.AccessPolicyBoAccessPolicy
	for i := range policies {
		p := &policies[i]
		if p.Policy == nil {
			continue
		}
		if (id != "" && deref(p.Policy.UniqueId) == id) || (id == "" && deref(p.Policy.Name) == name) {
			match = p
			break
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Access policy not found", "no access policy matched the given id/name")
		return
	}

	cfg.ID = types.StringValue(deref(match.Policy.UniqueId))
	cfg.Name = strToStateOptional(deref(match.Policy.Name))
	cfg.Description = strToStateOptional(deref(match.Policy.Description))
	cfg.Enabled = boolToState(match.Active)
	cfg.CreatorName = strToStateOptional(deref(match.CreatorName))
	cfg.UserCount = intToState(match.UserCount)

	var dc, ec, gc *int
	if match.ResourceCount != nil {
		dc, ec, gc = match.ResourceCount.DoorCount, match.ResourceCount.ElevatorCount, match.ResourceCount.GroupCount
	}
	cfg.DoorCount = intToState(dc)
	cfg.ElevatorCount = intToState(ec)
	cfg.GroupCount = intToState(gc)

	cfg.ValidStartTime = strToStateOptional(deref(match.ValidStartTime))
	cfg.ValidEndTime = strToStateOptional(deref(match.ValidEndTime))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
