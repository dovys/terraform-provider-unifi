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

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*dhcpReservationResource)(nil)
	_ resource.ResourceWithConfigure   = (*dhcpReservationResource)(nil)
	_ resource.ResourceWithImportState = (*dhcpReservationResource)(nil)
)

func NewDHCPReservationResource() resource.Resource { return &dhcpReservationResource{} }

type dhcpReservationResource struct {
	client unifi.Client
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
		r.client = c
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
	existing, err := r.client.GetUserByMAC(ctx, plan.MAC.ValueString())
	switch {
	case err == nil:
		desired.ID = existing.ID
		updated, uErr := r.client.UpdateUser(ctx, desired)
		if uErr != nil {
			resp.Diagnostics.AddError("Failed to set DHCP reservation", uErr.Error())
			return
		}
		plan.fromUser(updated)
	case isNotFoundErr(err):
		created, cErr := r.client.CreateUser(ctx, desired)
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
	u, err := r.client.GetUser(ctx, state.ID.ValueString())
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
	updated, err := r.client.UpdateUser(ctx, plan.toUser())
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
	// Safe by default: clear the reservation, don't delete the client record.
	off := false
	if _, err := r.client.UpdateUser(ctx, &unifi.User{ID: state.ID.ValueString(), UseFixedIP: &off}); err != nil {
		resp.Diagnostics.AddError("Failed to clear DHCP reservation", err.Error())
	}
}

// ImportState accepts either the client MAC (recommended — it's visible in the
// UI) or the raw controller `_id`, resolving both to the record `_id`.
func (r *dhcpReservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var u *unifi.User
	var err error
	if strings.Contains(req.ID, ":") {
		u, err = r.client.GetUserByMAC(ctx, req.ID)
	} else {
		u, err = r.client.GetUser(ctx, req.ID)
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DHCP reservation",
			"No client record found for "+req.ID+": "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), u.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), strings.ToLower(u.MAC))...)
}

func (m *dhcpReservationResourceModel) toUser() *unifi.User {
	on := true
	u := &unifi.User{
		ID:                    m.ID.ValueString(),
		MAC:                   strings.ToLower(m.MAC.ValueString()),
		Name:                  strOrEmpty(m.Name),
		UseFixedIP:            &on,
		FixedIP:               strOrEmpty(m.IPAddress),
		NetworkID:             strOrEmpty(m.NetworkID),
		LocalDNSRecord:        strOrEmpty(m.LocalDNSRecord),
		LocalDNSRecordEnabled: boolPtr(m.LocalDNSRecordEnabled),
	}
	return u
}

func (m *dhcpReservationResourceModel) fromUser(u *unifi.User) {
	m.ID = types.StringValue(u.ID)
	// mac is the immutable key — keep the configured/imported value rather than
	// the controller's echo (it lowercases, which would churn an upper-case config).
	m.IPAddress = strToStateOptional(u.FixedIP)
	m.NetworkID = strToStateOptional(u.NetworkID)
	m.Name = strToStateOptional(u.Name)
	m.Hostname = strToStateOptional(u.Hostname)
	m.LocalDNSRecord = strToStateOptional(u.LocalDNSRecord)
	m.LocalDNSRecordEnabled = boolToState(u.LocalDNSRecordEnabled)
}
