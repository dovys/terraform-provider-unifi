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
	_ datasource.DataSource              = (*accessHolidayGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*accessHolidayGroupDataSource)(nil)
)

func NewAccessHolidayGroupDataSource() datasource.DataSource { return &accessHolidayGroupDataSource{} }

type accessHolidayGroupDataSource struct {
	access *access.Client
}

type accessHolidayGroupDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	IsPrivate    types.Bool   `tfsdk:"is_private"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	TemplateName types.String `tfsdk:"template_name"`
	HolidayCount types.Int64  `tfsdk:"holiday_count"`
	CreateTime   types.String `tfsdk:"create_time"`
	HolidayIDs   types.Set    `tfsdk:"holiday_ids"`
}

func (d *accessHolidayGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_holiday_group"
}

func (d *accessHolidayGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a UniFi Access holiday group by `id` (`unique_id`) or `name` (read-only). **Modeled from the " +
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
				MarkdownDescription: "Holiday group display name. Provide `name` or `id`. If multiple groups share a name, the first match wins.",
			},
			"description":   schema.StringAttribute{Computed: true, MarkdownDescription: "Holiday group description."},
			"is_private":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the group is private to its creator."},
			"is_default":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the default holiday group."},
			"template_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Template the group was seeded from, if any."},
			"holiday_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of member holidays."},
			"create_time":   schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp, RFC3339."},
			"holiday_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Member holiday ids (`unique_id` of each holiday).",
			},
		},
	}
}

func (d *accessHolidayGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		d.access = c.Access()
	}
}

func (d *accessHolidayGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg accessHolidayGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := strings.TrimSpace(cfg.ID.ValueString())
	name := strings.TrimSpace(cfg.Name.ValueString())
	if id == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup key", "Set either `id` or `name` to look up a holiday group.")
		return
	}

	// Resolve the unique_id (a name lookup goes through the list), then read the
	// detail view for the full field set including member holidays.
	if id == "" {
		groups, err := d.access.HolidayGroups().List(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Failed to list holiday groups", err.Error())
			return
		}
		var match *wire.HolidayBoHolidayGroupSimple
		for i := range groups {
			if deref(groups[i].Name) == name {
				match = &groups[i]
				break
			}
		}
		if match == nil {
			resp.Diagnostics.AddError("Holiday group not found", "no holiday group matched the given name")
			return
		}
		id = deref(match.UniqueId)
	}

	g, err := d.access.HolidayGroups().Get(ctx, id)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Holiday group not found", "no holiday group matched the given id")
			return
		}
		resp.Diagnostics.AddError("Failed to read holiday group", err.Error())
		return
	}

	cfg.ID = types.StringValue(id)
	cfg.Name = strToStateOptional(deref(g.Name))
	cfg.Description = strToStateOptional(deref(g.Description))
	cfg.IsPrivate = boolToState(g.IsPrivate)
	cfg.IsDefault = boolToState(g.IsDefault)
	cfg.TemplateName = strToStateOptional(deref(g.TemplateName))
	cfg.CreateTime = accessTimeToState(g.CreateTime)

	ids := holidayMemberIDs(g.Holidays)
	cfg.HolidayIDs = sliceToStringSet(ids)
	cfg.HolidayCount = types.Int64Value(int64(len(ids)))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
