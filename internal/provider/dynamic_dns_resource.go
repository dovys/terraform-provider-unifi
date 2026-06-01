package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*dynamicDNSResource)(nil)
	_ resource.ResourceWithConfigure   = (*dynamicDNSResource)(nil)
	_ resource.ResourceWithImportState = (*dynamicDNSResource)(nil)
)

func NewDynamicDNSResource() resource.Resource { return &dynamicDNSResource{} }

type dynamicDNSResource struct {
	client unifi.Client
}

type dynamicDNSResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Service   types.String `tfsdk:"service"`
	Hostname  types.String `tfsdk:"hostname"`
	Server    types.String `tfsdk:"server"`
	Login     types.String `tfsdk:"login"`
	Password  types.String `tfsdk:"password"` // x_password (write-only)
	Interface types.String `tfsdk:"interface"`
}

func (r *dynamicDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dynamic_dns"
}

func (r *dynamicDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi dynamic-DNS updater (`dynamicdns`, legacy `/rest`) — publishes the gateway's WAN " +
			"IP to a DDNS provider so a hostname tracks a changing IP.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "DDNS provider key (e.g. `dyndns`, `namecheap`, `noip`, `cloudflare`, `custom`). " +
					"Use `custom` together with `server`.",
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The fully-qualified hostname to keep updated.",
			},
			"server": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Update server hostname. Required when `service = custom`.",
			},
			"login": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Provider account username.",
			},
			"password": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Provider password/token (`x_password`). Write-only: the controller does not echo it " +
					"back, so the configured value is preserved in state.",
			},
			"interface": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "WAN interface whose IP is published (e.g. `wan`). Defaults to `wan`.",
			},
		},
	}
}

func (r *dynamicDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *dynamicDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dynamicDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateDynamicDNS(ctx, plan.toDynamicDNS())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create dynamic DNS", err.Error())
		return
	}
	plan.fromDynamicDNS(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dynamicDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dynamicDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDynamicDNS(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read dynamic DNS", err.Error())
		return
	}
	state.fromDynamicDNS(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dynamicDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dynamicDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateDynamicDNS(ctx, plan.toDynamicDNS())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update dynamic DNS", err.Error())
		return
	}
	plan.fromDynamicDNS(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dynamicDNSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dynamicDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDynamicDNS(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete dynamic DNS", err.Error())
	}
}

func (r *dynamicDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *dynamicDNSResourceModel) toDynamicDNS() *unifi.DynamicDNS {
	return &unifi.DynamicDNS{
		ID:        m.ID.ValueString(),
		Service:   strOrEmpty(m.Service),
		HostName:  strOrEmpty(m.Hostname),
		Server:    strOrEmpty(m.Server),
		Login:     strOrEmpty(m.Login),
		XPassword: strOrEmpty(m.Password),
		Interface: strOrEmpty(m.Interface),
	}
}

func (m *dynamicDNSResourceModel) fromDynamicDNS(d *unifi.DynamicDNS) {
	m.ID = types.StringValue(d.ID)
	m.Service = strToStateOptional(d.Service)
	m.Hostname = strToStateOptional(d.HostName)
	m.Server = strToStateOptional(d.Server)
	m.Login = strToStateOptional(d.Login)
	// password is write-only: keep the configured value if the controller echoes
	// it empty (the usual case), otherwise adopt what it returned.
	if d.XPassword != "" {
		m.Password = types.StringValue(d.XPassword)
	}
	m.Interface = strToStateOptional(d.Interface)
}
