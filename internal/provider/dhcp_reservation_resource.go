package provider

import (
	"context"
	"strings"

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
	_ resource.Resource                = (*dhcpReservationResource)(nil)
	_ resource.ResourceWithConfigure   = (*dhcpReservationResource)(nil)
	_ resource.ResourceWithImportState = (*dhcpReservationResource)(nil)
)

func NewDHCPReservationResource() resource.Resource { return &dhcpReservationResource{} }

type dhcpReservationResource struct {
	net *network.Client
}

type dhcpReservationResourceModel struct {
	ID                    types.String `tfsdk:"id"`                       // client record _id (computed)
	MAC                   types.String `tfsdk:"mac"`                      // client MAC (required, immutable)
	IPAddress             types.String `tfsdk:"ip_address"`               // the reserved fixed IP (required)
	NetworkID             types.String `tfsdk:"network_id"`               // network whose DHCP scope holds it
	Name                  types.String `tfsdk:"name"`                     // friendly alias
	Hostname              types.String `tfsdk:"hostname"`                 // DHCP-reported hostname (read-only)
	LocalDNSRecord        types.String `tfsdk:"local_dns_record"`         // optional name -> fixed-IP A record
	LocalDNSRecordEnabled types.Bool   `tfsdk:"local_dns_record_enabled"` // whether the local DNS record is active
}

func (r *dhcpReservationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dhcp_reservation"
}

func (r *dhcpReservationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A DHCP fixed-IP reservation — pins a client (by MAC) to an address in a network's DHCP " +
			"scope. This is the only client-level object the provider manages: it's *find-or-create by MAC* (the " +
			"controller may already track the client from a lease), and `terraform destroy` clears the reservation " +
			"rather than deleting the client record.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller client-record ID (`_id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Client MAC address. Immutable — a different MAC is a different reservation. Import key (you can `terraform import` by MAC).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The reserved IPv4 address. Must fall within the DHCP scope of `network_id`.",
			},
			"network_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "ID of the network (`unifi_network_network`) whose DHCP scope holds the reservation. The controller derives it from the address when unset.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Friendly alias for the client.",
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DHCP-reported hostname (read-only).",
			},
			"local_dns_record": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Optional local DNS name resolving to the reserved IP (e.g. `nas.lan`).",
			},
			"local_dns_record_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the `local_dns_record` is active.",
			},
		},
	}
}

func (r *dhcpReservationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *dhcpReservationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dhcpReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Find-or-create by MAC: the controller may already track the client from a
	// DHCP lease, in which case we update that record rather than POSTing a new
	// one (a duplicate MAC would be rejected).
	desired := plan.toUser()
	existing, err := r.net.Clients().GetByMAC(ctx, plan.MAC.ValueString())
	switch {
	case err == nil:
		desired.UnderscoreId = existing.UnderscoreId
		updated, uErr := r.net.Clients().Users().Update(ctx, deref(existing.UnderscoreId), desired)
		if uErr != nil {
			resp.Diagnostics.AddError("Failed to set DHCP reservation", uErr.Error())
			return
		}
		plan.fromUser(updated)
	case isNotFoundErr(err):
		created, cErr := r.net.Clients().Users().Create(ctx, desired)
		if cErr != nil {
			resp.Diagnostics.AddError("Failed to create DHCP reservation", cErr.Error())
			return
		}
		plan.fromUser(created)
	default:
		resp.Diagnostics.AddError("Failed to look up client by MAC", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dhcpReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	u, err := r.net.Clients().Users().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DHCP reservation", err.Error())
		return
	}
	state.fromUser(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dhcpReservationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dhcpReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Clients().Users().Update(ctx, plan.ID.ValueString(), plan.toUser())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update DHCP reservation", err.Error())
		return
	}
	plan.fromUser(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dhcpReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Safe by default: clear the reservation, don't delete the client record. If
	// the client record is already gone, the reservation is gone too — treat that
	// as success rather than failing the destroy.
	off := false
	id := state.ID.ValueString()
	if _, err := r.net.Clients().Users().Update(ctx, id, &wire.User{UnderscoreId: strPtrOrNil(state.ID), UseFixedip: &off}); err != nil && !isNotFoundErr(err) {
		resp.Diagnostics.AddError("Failed to clear DHCP reservation", err.Error())
	}
}

// ImportState accepts either the client MAC (recommended — it's visible in the
// UI) or the raw controller `_id`, resolving both to the record `_id`.
func (r *dhcpReservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var u *wire.User
	var err error
	// A controller _id is a 24-char hex Mongo ObjectID (no colons); a MAC always
	// has colons — so a ":" cleanly disambiguates the two import forms.
	if strings.Contains(req.ID, ":") {
		u, err = r.net.Clients().GetByMAC(ctx, req.ID)
	} else {
		u, err = r.net.Clients().Users().Get(ctx, req.ID)
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DHCP reservation",
			"No client record found for "+req.ID+": "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), deref(u.UnderscoreId))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), strings.ToLower(deref(u.Mac)))...)
}

func (m *dhcpReservationResourceModel) toUser() *wire.User {
	on := true
	mac := strings.ToLower(m.MAC.ValueString())
	u := &wire.User{
		UnderscoreId:          strPtrOrNil(m.ID),
		Mac:                   &mac,
		Name:                  strPtrOrNil(m.Name),
		UseFixedip:            &on,
		FixedIp:               strPtrOrNil(m.IPAddress),
		NetworkId:             strPtrOrNil(m.NetworkID),
		LocalDnsRecord:        strPtrOrNil(m.LocalDNSRecord),
		LocalDnsRecordEnabled: boolPtr(m.LocalDNSRecordEnabled),
	}
	return u
}

func (m *dhcpReservationResourceModel) fromUser(u *wire.User) {
	m.ID = types.StringValue(deref(u.UnderscoreId))
	// mac is the immutable key — keep the configured/imported value rather than
	// the controller's echo (it lowercases, which would churn an upper-case config).
	m.IPAddress = types.StringValue(deref(u.FixedIp)) // Required: always a concrete value, never null
	m.NetworkID = strToStateOptional(deref(u.NetworkId))
	m.Name = strToStateOptional(deref(u.Name))
	m.Hostname = strToStateOptional(deref(u.Hostname))
	m.LocalDNSRecord = strToStateOptional(deref(u.LocalDnsRecord))
	m.LocalDNSRecordEnabled = boolToState(u.LocalDnsRecordEnabled)
}
