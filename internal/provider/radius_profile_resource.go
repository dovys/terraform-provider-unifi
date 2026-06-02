package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*radiusProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*radiusProfileResource)(nil)
	_ resource.ResourceWithImportState = (*radiusProfileResource)(nil)
)

func NewRadiusProfileResource() resource.Resource { return &radiusProfileResource{} }

type radiusProfileResource struct {
	net *network.Client
}

type radiusProfileResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	AuthServers           types.List   `tfsdk:"auth_servers"` // ordered: primary then fallbacks
	AcctServers           types.List   `tfsdk:"acct_servers"`
	UseUSGAuthServer      types.Bool   `tfsdk:"use_usg_auth_server"`
	UseUSGAcctServer      types.Bool   `tfsdk:"use_usg_acct_server"`
	InterimUpdateEnabled  types.Bool   `tfsdk:"interim_update_enabled"`
	InterimUpdateInterval types.Int64  `tfsdk:"interim_update_interval"`
	VLANEnabled           types.Bool   `tfsdk:"vlan_enabled"`
	VLANWLANMode          types.String `tfsdk:"vlan_wlan_mode"`
	AttrNoDelete          types.Bool   `tfsdk:"attr_no_delete"`
}

// radiusServerObject mirrors one auth/acct server for List decoding.
type radiusServerObject struct {
	IP      types.String `tfsdk:"ip"`
	Port    types.Int64  `tfsdk:"port"`
	XSecret types.String `tfsdk:"x_secret"`
}

var radiusServerAttrTypes = map[string]attr.Type{
	"ip":       types.StringType,
	"port":     types.Int64Type,
	"x_secret": types.StringType,
}

func (r *radiusProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_radius_profile"
}

func radiusServerSchema(kind string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		Computed: true,
		MarkdownDescription: "Ordered list of " + kind + " RADIUS servers (primary first, then fallbacks). " +
			"Leave empty when using the gateway's built-in server.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"ip": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Server IP address.",
				},
				"port": schema.Int64Attribute{
					Optional:            true,
					Computed:            true,
					MarkdownDescription: "UDP port (default 1812 for auth, 1813 for accounting).",
					Validators:          []validator.Int64{int64validator.Between(1, 65535)},
				},
				"x_secret": schema.StringAttribute{
					Required:            true,
					Sensitive:           true,
					MarkdownDescription: "Shared secret for this server.",
				},
			},
		},
	}
}

func (r *radiusProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi RADIUS profile (`radiusprofile`, legacy `/rest`) — a named set of " +
			"authentication/accounting servers referenced by WLANs (`wpaeap`) and wired 802.1X. The " +
			"controller ships a protected default profile.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Profile name.",
			},
			"auth_servers": radiusServerSchema("authentication"),
			"acct_servers": radiusServerSchema("accounting"),
			"use_usg_auth_server": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Use the gateway's built-in RADIUS server for authentication instead of external `auth_servers`.",
			},
			"use_usg_acct_server": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Use the gateway's built-in RADIUS server for accounting.",
			},
			"interim_update_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Send RADIUS accounting interim updates.",
			},
			"interim_update_interval": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Interval (seconds) between accounting interim updates.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"vlan_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Honor RADIUS-assigned VLANs (tunnel attributes) returned by the server.",
			},
			"vlan_wlan_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "RADIUS-assigned VLAN behavior for WLANs: `disabled`, `optional`, or `required`.",
				Validators:          []validator.String{stringvalidator.OneOf("disabled", "optional", "required")},
			},
			"attr_no_delete": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the controller protects this profile from deletion (the built-in default " +
					"profile). When `true`, `terraform destroy` removes the resource from state without deleting it.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *radiusProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *radiusProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan radiusProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.RADIUS().Profiles().Create(ctx, plan.toRadiusProfile(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create RADIUS profile", err.Error())
		return
	}
	plan.fromRadiusProfile(ctx, created, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state radiusProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.net.RADIUS().Profiles().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read RADIUS profile", err.Error())
		return
	}
	state.fromRadiusProfile(ctx, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *radiusProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan radiusProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.RADIUS().Profiles().Update(ctx, plan.ID.ValueString(), plan.toRadiusProfile(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update RADIUS profile", err.Error())
		return
	}
	plan.fromRadiusProfile(ctx, updated, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state radiusProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The controller-protected default profile can't be deleted — drop it from
	// state rather than issuing a DELETE the controller would reject.
	if state.AttrNoDelete.ValueBool() {
		return
	}
	if err := r.net.RADIUS().Profiles().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete RADIUS profile", err.Error())
	}
}

func (r *radiusProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *radiusProfileResourceModel) toRadiusProfile(ctx context.Context, diags *diag.Diagnostics) *wire.Radiusprofile {
	p := &wire.Radiusprofile{
		UnderscoreId:          strPtrOrNil(m.ID),
		Name:                  strPtrOrNil(m.Name),
		UseUsgAuthServer:      boolPtr(m.UseUSGAuthServer),
		UseUsgAcctServer:      boolPtr(m.UseUSGAcctServer),
		InterimUpdateEnabled:  boolPtr(m.InterimUpdateEnabled),
		InterimUpdateInterval: intPtr(m.InterimUpdateInterval),
		VlanEnabled:           boolPtr(m.VLANEnabled),
		VlanWlanMode:          (*wire.RadiusprofileVlanWlanMode)(strPtrOrNil(m.VLANWLANMode)),
	}
	auth := listToRadiusServers(ctx, m.AuthServers, diags)
	acct := listToRadiusServers(ctx, m.AcctServers, diags)
	p.AuthServers = &auth
	p.AcctServers = &acct
	return p
}

func (m *radiusProfileResourceModel) fromRadiusProfile(ctx context.Context, p *wire.Radiusprofile, diags *diag.Diagnostics) {
	m.ID = types.StringValue(deref(p.UnderscoreId))
	m.Name = types.StringValue(deref(p.Name))
	m.AuthServers = radiusServersToList(ctx, p.AuthServers, diags)
	m.AcctServers = radiusServersToList(ctx, p.AcctServers, diags)
	m.UseUSGAuthServer = boolToState(p.UseUsgAuthServer)
	m.UseUSGAcctServer = boolToState(p.UseUsgAcctServer)
	m.InterimUpdateEnabled = boolToState(p.InterimUpdateEnabled)
	m.InterimUpdateInterval = intToState(p.InterimUpdateInterval)
	m.VLANEnabled = boolToState(p.VlanEnabled)
	m.VLANWLANMode = strToStateOptional(string(derefEnum(p.VlanWlanMode)))
	m.AttrNoDelete = types.BoolValue(p.AttrNoDelete != nil && *p.AttrNoDelete)
}

// radiusServer is the wire shape of one auth/acct server entry (the inline
// anonymous struct on wire.Radiusprofile, shared verbatim by both lists).
type radiusServer = struct {
	Ip      *string `json:"ip,omitempty"`
	Port    *int    `json:"port,omitempty"`
	XSecret *string `json:"x_secret,omitempty"`
}

// listToRadiusServers decodes the config list into wire servers. A null list
// becomes a non-nil empty slice so the full-replace PUT can clear the servers.
func listToRadiusServers(ctx context.Context, l types.List, diags *diag.Diagnostics) []radiusServer {
	out := []radiusServer{}
	if l.IsNull() || l.IsUnknown() {
		return out
	}
	var objs []radiusServerObject
	diags.Append(l.ElementsAs(ctx, &objs, false)...)
	for _, o := range objs {
		s := radiusServer{Ip: strPtrOrNil(o.IP), XSecret: strPtrOrNil(o.XSecret)}
		if !o.Port.IsNull() && !o.Port.IsUnknown() {
			s.Port = intPtr(o.Port)
		}
		out = append(out, s)
	}
	return out
}

// radiusServersToList builds the state list. A nil/empty server slice maps to a
// non-null empty list so an unconfigured (gateway-only) profile doesn't churn.
func radiusServersToList(ctx context.Context, servers *[]radiusServer, diags *diag.Diagnostics) types.List {
	objType := types.ObjectType{AttrTypes: radiusServerAttrTypes}
	objs := []radiusServerObject{}
	if servers != nil {
		for _, s := range *servers {
			objs = append(objs, radiusServerObject{
				IP:      types.StringValue(deref(s.Ip)),
				Port:    types.Int64Value(int64(intOrZero(s.Port))),
				XSecret: types.StringValue(deref(s.XSecret)),
			})
		}
	}
	l, d := types.ListValueFrom(ctx, objType, objs)
	diags.Append(d...)
	return l
}

// intOrZero dereferences a *int, treating nil as 0.
func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
