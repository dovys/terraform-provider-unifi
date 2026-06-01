package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*portForwardResource)(nil)
	_ resource.ResourceWithConfigure   = (*portForwardResource)(nil)
	_ resource.ResourceWithImportState = (*portForwardResource)(nil)
)

func NewPortForwardResource() resource.Resource { return &portForwardResource{} }

type portForwardResource struct {
	client unifi.Client
}

type portForwardResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Interface       types.String `tfsdk:"interface"`        // wan | wan2 | both
	Protocol        types.String `tfsdk:"protocol"`         // tcp_udp | tcp | udp
	Source          types.String `tfsdk:"source"`           // "any" or CIDR/IP
	DestinationPort types.String `tfsdk:"destination_port"` // external port(s)
	ForwardIP       types.String `tfsdk:"forward_ip"`       // internal host
	ForwardPort     types.String `tfsdk:"forward_port"`     // internal port(s)
	Log             types.Bool   `tfsdk:"log"`
}

func (r *portForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_port_forward"
}

func (r *portForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi port-forwarding rule (`portforward`, legacy `/rest`) — destination-NAT from a WAN " +
			"port to an internal host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Rule name.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the rule is active. Defaults to `true`.",
			},
			"interface": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "WAN the rule listens on: `wan`, `wan2`, or `both`. Defaults to `wan`.",
				Validators:          []validator.String{stringvalidator.OneOf("wan", "wan2", "both")},
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Protocol: `tcp_udp`, `tcp`, or `udp`. Defaults to `tcp_udp`.",
				Validators:          []validator.String{stringvalidator.OneOf("tcp_udp", "tcp", "udp")},
			},
			"source": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Source filter: `any` (default) or a restricting IP/CIDR.",
			},
			"destination_port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "External port or range to forward (e.g. `443` or `8000-8010`).",
			},
			"forward_ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Internal host IP to forward to.",
			},
			"forward_port": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Internal port or range. Defaults to `destination_port` when omitted.",
			},
			"log": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Log matches in the firewall log. Defaults to `false`.",
			},
		},
	}
}

func (r *portForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *portForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portForwardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreatePortForward(ctx, plan.toPortForward())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create port forward", err.Error())
		return
	}
	plan.fromPortForward(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portForwardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pf, err := r.client.GetPortForward(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read port forward", err.Error())
		return
	}
	state.fromPortForward(pf)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portForwardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portForwardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdatePortForward(ctx, plan.toPortForward())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update port forward", err.Error())
		return
	}
	plan.fromPortForward(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portForwardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePortForward(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete port forward", err.Error())
	}
}

func (r *portForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *portForwardResourceModel) toPortForward() *unifi.PortForward {
	return &unifi.PortForward{
		ID:            m.ID.ValueString(),
		Name:          m.Name.ValueString(),
		Enabled:       boolPtr(m.Enabled),
		PfwdInterface: strOrEmpty(m.Interface),
		Proto:         strOrEmpty(m.Protocol),
		Src:           strOrEmpty(m.Source),
		DstPort:       strOrEmpty(m.DestinationPort),
		Fwd:           strOrEmpty(m.ForwardIP),
		FwdPort:       strOrEmpty(m.ForwardPort),
		Log:           boolPtr(m.Log),
	}
}

func (m *portForwardResourceModel) fromPortForward(pf *unifi.PortForward) {
	m.ID = types.StringValue(pf.ID)
	m.Name = types.StringValue(pf.Name)
	m.Enabled = boolToState(pf.Enabled)
	m.Interface = strToStateOptional(pf.PfwdInterface)
	m.Protocol = strToStateOptional(pf.Proto)
	m.Source = strToStateOptional(pf.Src)
	m.DestinationPort = strToStateOptional(pf.DstPort)
	m.ForwardIP = strToStateOptional(pf.Fwd)
	m.ForwardPort = strToStateOptional(pf.FwdPort)
	m.Log = boolToState(pf.Log)
}
