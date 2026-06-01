package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

var (
	_ resource.Resource                = (*dnsRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsRecordResource)(nil)
	_ resource.ResourceWithImportState = (*dnsRecordResource)(nil)
)

func NewDNSRecordResource() resource.Resource { return &dnsRecordResource{} }

type dnsRecordResource struct {
	client unifi.Client
}

type dnsRecordResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	Value    types.String `tfsdk:"value"`
	Enabled  types.Bool   `tfsdk:"enabled"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Port     types.Int64  `tfsdk:"port"`
	Priority types.Int64  `tfsdk:"priority"`
	Weight   types.Int64  `tfsdk:"weight"`
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A local DNS record. Served from the v2 `/static-dns` surface today; routing moves to the " +
			"Integration DNS surface on newer firmware with no schema change.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS name (the record key), e.g. `host.example.com`.",
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("A"),
				MarkdownDescription: "Record type: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `SRV`, or `NS`.",
				Validators:          []validator.String{stringvalidator.OneOf("A", "AAAA", "CNAME", "MX", "TXT", "SRV", "NS")},
			},
			"value": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record target (IP address, hostname, or text depending on type).",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the record is active.",
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "TTL in seconds; `0` (or unset) uses the controller default.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Port (SRV records).",
			},
			"priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Priority (MX/SRV records).",
			},
			"weight": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Weight (SRV records).",
			},
		},
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateDNSRecord(ctx, plan.toRecord())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())
		return
	}
	plan.fromRecord(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rec, err := r.client.GetDNSRecord(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())
		return
	}
	state.fromRecord(rec)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateDNSRecord(ctx, plan.toRecord())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update DNS record", err.Error())
		return
	}
	plan.fromRecord(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDNSRecord(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *dnsRecordResourceModel) toRecord() *models.StaticDNSRecord {
	return &models.StaticDNSRecord{
		ID:         m.ID.ValueString(),
		Key:        m.Name.ValueString(),
		RecordType: strOrEmpty(m.Type),
		Value:      m.Value.ValueString(),
		Enabled:    boolPtr(m.Enabled),
		TTL:        intPtr(m.TTL),
		Port:       intPtr(m.Port),
		Priority:   intPtr(m.Priority),
		Weight:     intPtr(m.Weight),
	}
}

func (m *dnsRecordResourceModel) fromRecord(rec *models.StaticDNSRecord) {
	m.ID = types.StringValue(rec.ID)
	m.Name = types.StringValue(rec.Key)
	m.Type = types.StringValue(rec.RecordType)
	m.Value = types.StringValue(rec.Value)
	m.Enabled = boolToState(rec.Enabled)
	m.TTL = intToState(rec.TTL)
	m.Port = intToState(rec.Port)
	m.Priority = intToState(rec.Priority)
	m.Weight = intToState(rec.Weight)
}
