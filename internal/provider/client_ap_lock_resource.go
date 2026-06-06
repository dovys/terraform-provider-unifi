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
	_ resource.Resource                = (*clientAPLockResource)(nil)
	_ resource.ResourceWithConfigure   = (*clientAPLockResource)(nil)
	_ resource.ResourceWithImportState = (*clientAPLockResource)(nil)
)

func NewClientAPLockResource() resource.Resource { return &clientAPLockResource{} }

type clientAPLockResource struct {
	net *network.Client
}

type clientAPLockResourceModel struct {
	ID    types.String `tfsdk:"id"`     // client record _id (computed)
	MAC   types.String `tfsdk:"mac"`    // client MAC (required, immutable)
	APMAC types.String `tfsdk:"ap_mac"` // AP MAC the client is pinned to (required)
}

func (r *clientAPLockResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_client_ap_lock"
}

func (r *clientAPLockResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pins a wireless client (by MAC) to a specific access point — the controller " +
			"won't steer or roam it to another AP. Like `unifi_network_dhcp_reservation`, this is a " +
			"client-record attribute, not a standalone object: it's *find-or-create by MAC* (the controller " +
			"may already track the client from a lease), and `terraform destroy` clears the AP lock " +
			"(`fixed_ap_enabled = false`) rather than deleting the client record. The lock and a DHCP " +
			"reservation can coexist on the same MAC — the legacy `/rest/user` PUT merges server-side, so " +
			"writing the AP-lock fields leaves any reservation fields untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller client-record ID (`_id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Client MAC address (case-insensitive — normalized to lower-case). Immutable — a different MAC is a different lock. Import key (you can `terraform import` by MAC).",
				PlanModifiers:       []planmodifier.String{lowercase(), stringplanmodifier.RequiresReplace()},
			},
			"ap_mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "MAC address of the access point to pin the client to (case-insensitive — normalized to lower-case). Mutable: change it to re-pin the client to a different AP in place.",
				PlanModifiers:       []planmodifier.String{lowercase()},
			},
		},
	}
}

func (r *clientAPLockResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *clientAPLockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clientAPLockResourceModel
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
			resp.Diagnostics.AddError("Failed to set client AP lock", uErr.Error())
			return
		}
		plan.fromUser(updated)
	case isNotFoundErr(err):
		created, cErr := r.net.Clients().Users().Create(ctx, desired)
		if cErr != nil {
			resp.Diagnostics.AddError("Failed to create client AP lock", cErr.Error())
			return
		}
		plan.fromUser(created)
	default:
		resp.Diagnostics.AddError("Failed to look up client by MAC", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientAPLockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clientAPLockResourceModel
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
		resp.Diagnostics.AddError("Failed to read client AP lock", err.Error())
		return
	}
	// The lock is an attribute on the shared client record: a record with the lock
	// disabled (or never set) means "no lock". Unlike the sibling dhcp_reservation
	// (whose Read keeps state regardless of use_fixedip), this resource has no
	// schema field tracking the enabled flag — fixed_ap_enabled is always asserted
	// true on write — so a disabled record whose stale fixed_ap_mac still matches
	// the config would read back clean and silently mask the drift. Removing it
	// from state instead forces a re-create that re-asserts the lock. (Intentional
	// divergence from dhcp_reservation; don't "align" it back.)
	if u.FixedApEnabled == nil || !*u.FixedApEnabled {
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromUser(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clientAPLockResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clientAPLockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Clients().Users().Update(ctx, plan.ID.ValueString(), plan.toUser())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update client AP lock", err.Error())
		return
	}
	plan.fromUser(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientAPLockResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clientAPLockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Safe by default: clear the lock, don't delete the client record. If the
	// record is already gone, the lock is gone too — treat that as success rather
	// than failing the destroy. The sparse PUT leaves any DHCP reservation on the
	// same record intact.
	off := false
	id := state.ID.ValueString()
	if _, err := r.net.Clients().Users().Update(ctx, id, &wire.User{UnderscoreId: strPtrOrNil(state.ID), FixedApEnabled: &off}); err != nil && !isNotFoundErr(err) {
		resp.Diagnostics.AddError("Failed to clear client AP lock", err.Error())
	}
}

// ImportState accepts either the client MAC (recommended — it's visible in the
// UI) or the raw controller `_id`, resolving both to the record `_id`.
func (r *clientAPLockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
		resp.Diagnostics.AddError("Failed to import client AP lock",
			"No client record found for "+req.ID+": "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), deref(u.UnderscoreId))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), strings.ToLower(deref(u.Mac)))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ap_mac"), strings.ToLower(deref(u.FixedApMac)))...)
}

func (m *clientAPLockResourceModel) toUser() *wire.User {
	on := true
	mac := strings.ToLower(m.MAC.ValueString())
	apMAC := strings.ToLower(m.APMAC.ValueString())
	u := &wire.User{
		UnderscoreId:   strPtrOrNil(m.ID),
		Mac:            &mac,
		FixedApEnabled: &on,
		FixedApMac:     &apMAC,
	}
	return u
}

func (m *clientAPLockResourceModel) fromUser(u *wire.User) {
	m.ID = types.StringValue(deref(u.UnderscoreId))
	// mac is the immutable key — keep the configured/imported value rather than
	// the controller's echo (it lowercases, which would churn an upper-case config).
	m.APMAC = types.StringValue(strings.ToLower(deref(u.FixedApMac)))
}
