package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*radiusAccountResource)(nil)
	_ resource.ResourceWithConfigure   = (*radiusAccountResource)(nil)
	_ resource.ResourceWithImportState = (*radiusAccountResource)(nil)
)

func NewRadiusAccountResource() resource.Resource { return &radiusAccountResource{} }

type radiusAccountResource struct {
	net *network.Client
}

type radiusAccountResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`     // RADIUS username
	Password         types.String `tfsdk:"password"` // x_password (write-only; controller doesn't echo it)
	TunnelType       types.Int64  `tfsdk:"tunnel_type"`
	TunnelMediumType types.Int64  `tfsdk:"tunnel_medium_type"`
	VLAN             types.Int64  `tfsdk:"vlan"`
}

func (r *radiusAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_radius_account"
}

func (r *radiusAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi RADIUS user account (`account`, legacy `/rest`) — a username/password served by " +
			"the gateway's built-in RADIUS server, optionally carrying dynamic-VLAN tunnel attributes. Reference it " +
			"from a `unifi_network_radius_profile` that has `use_usg_auth_server = true`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "RADIUS username.",
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "RADIUS password (`x_password`). Write-only: the controller does not echo it back, " +
					"so the configured value is preserved in state.",
			},
			"tunnel_type": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "RADIUS Tunnel-Type attribute for dynamic VLAN assignment (e.g. `13` = VLAN). " +
					"Set together with `tunnel_medium_type` and `vlan`.",
			},
			"tunnel_medium_type": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "RADIUS Tunnel-Medium-Type attribute (e.g. `6` = IEEE-802).",
			},
			"vlan": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Tunnel-Private-Group-ID — the VLAN id assigned to clients authenticating as this user.",
			},
		},
	}
}

func (r *radiusAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *radiusAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan radiusAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.RADIUS().Accounts().Create(ctx, plan.toRadiusAccount())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create RADIUS account", err.Error())
		return
	}
	plan.fromRadiusAccount(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state radiusAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.net.RADIUS().Accounts().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read RADIUS account", err.Error())
		return
	}
	state.fromRadiusAccount(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *radiusAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan radiusAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.RADIUS().Accounts().Update(ctx, plan.ID.ValueString(), plan.toRadiusAccount())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update RADIUS account", err.Error())
		return
	}
	plan.fromRadiusAccount(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state radiusAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.RADIUS().Accounts().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete RADIUS account", err.Error())
	}
}

func (r *radiusAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *radiusAccountResourceModel) toRadiusAccount() *wire.Account {
	return &wire.Account{
		UnderscoreId:     strPtrOrNil(m.ID),
		Name:             strPtrOrNil(m.Name),
		XPassword:        strPtrOrNil(m.Password),
		TunnelType:       intPtr(m.TunnelType),
		TunnelMediumType: intPtr(m.TunnelMediumType),
		Vlan:             intPtr(m.VLAN),
	}
}

func (m *radiusAccountResourceModel) fromRadiusAccount(a *wire.Account) {
	m.ID = types.StringValue(deref(a.UnderscoreId))
	m.Name = types.StringValue(deref(a.Name))
	// password is write-only: keep the configured value if the controller echoes
	// it empty (the usual case), otherwise adopt what it returned.
	if deref(a.XPassword) != "" {
		m.Password = types.StringValue(deref(a.XPassword))
	}
	m.TunnelType = intToState(a.TunnelType)
	m.TunnelMediumType = intToState(a.TunnelMediumType)
	m.VLAN = intToState(a.Vlan)
}
