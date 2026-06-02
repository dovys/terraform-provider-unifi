package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*firewallGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*firewallGroupResource)(nil)
	_ resource.ResourceWithImportState = (*firewallGroupResource)(nil)
)

func NewFirewallGroupResource() resource.Resource { return &firewallGroupResource{} }

type firewallGroupResource struct {
	net *network.Client
}

type firewallGroupResourceModel struct {
	ID      types.String `tfsdk:"id"`      // controller _id; import key (computed)
	Name    types.String `tfsdk:"name"`    // display name (required)
	Type    types.String `tfsdk:"type"`    // group_type (required, immutable)
	Members types.Set    `tfsdk:"members"` // addresses or ports
}

func (r *firewallGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_firewall_group"
}

func (r *firewallGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi firewall group (`firewallgroup`, legacy `/rest`) — a reusable set of addresses or " +
			"ports referenced by `unifi_network_firewall_rule` via its `src_firewall_group_ids`/`dst_firewall_group_ids`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group name.",
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Group type: `address-group` (IPv4 addresses/CIDRs), `port-group` (ports), or " +
					"`ipv6-address-group` (IPv6 addresses/CIDRs). Immutable — changing it replaces the group.",
				Validators:    []validator.String{stringvalidator.OneOf("address-group", "port-group", "ipv6-address-group")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"members": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Members: IPs/CIDRs for an address group, port numbers/ranges for a port group. Empty groups are allowed.",
			},
		},
	}
}

func (r *firewallGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *firewallGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.Firewall().Groups().Create(ctx, plan.toFirewallGroup(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall group", err.Error())
		return
	}
	plan.fromFirewallGroup(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.net.Firewall().Groups().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall group", err.Error())
		return
	}
	state.fromFirewallGroup(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *firewallGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Firewall().Groups().Update(ctx, plan.ID.ValueString(), plan.toFirewallGroup(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall group", err.Error())
		return
	}
	plan.fromFirewallGroup(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.Firewall().Groups().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall group", err.Error())
	}
}

func (r *firewallGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *firewallGroupResourceModel) toFirewallGroup(ctx context.Context, diags *diag.Diagnostics) *wire.Firewallgroup {
	members := stringSetToSlice(ctx, m.Members, diags)
	if members == nil {
		members = []string{} // send an explicit empty array so emptying the set clears the group
	}
	return &wire.Firewallgroup{
		UnderscoreId: strPtrOrNil(m.ID),
		Name:         strPtrOrNil(m.Name),
		GroupType:    (*wire.FirewallgroupGroupType)(strPtrOrNil(m.Type)),
		GroupMembers: &members,
	}
}

func (m *firewallGroupResourceModel) fromFirewallGroup(g *wire.Firewallgroup) {
	m.ID = types.StringValue(deref(g.UnderscoreId))
	m.Name = types.StringValue(deref(g.Name))
	m.Type = types.StringValue(string(derefEnum(g.GroupType)))
	m.Members = sliceToStringSet(derefSlice(g.GroupMembers))
}
