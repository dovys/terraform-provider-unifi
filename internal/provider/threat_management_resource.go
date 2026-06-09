package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

var (
	_ resource.Resource                = (*threatManagementResource)(nil)
	_ resource.ResourceWithConfigure   = (*threatManagementResource)(nil)
	_ resource.ResourceWithImportState = (*threatManagementResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*threatManagementResource)(nil)
)

// threatManagementID is the fixed state id of the IPS/IDS settings singleton. The
// `ips` section is one document per site, so the id is constant rather than a
// controller-assigned _id.
const threatManagementID = "ips"

// ipsFeature is the v2 /features flag for IDS/IPS firmware support (gateways
// only). Presence ≠ active subscription: a console can list IPS yet still refuse
// to enable without CyberSecure (confirmed live), so it gates only the wholly-
// unsupported case; the subscription state comes from the write response.
const ipsFeature = "IPS"

// ipsModes mirrors the controller's `ips_mode` enum: `disabled` (off), `ids`
// (detection — log only), `ips`/`ipsInline` (prevention — block). Both prevention
// modes are accepted; `ipsInline` runs the engine inline on the data path.
var ipsModes = []string{"disabled", "ids", "ips", "ipsInline"}

// advancedFilteringPreferences mirrors `advanced_filtering_preference`.
var advancedFilteringPreferences = []string{"manual", "disabled"}

// ipsCategories is the controller's validated `enabled_categories` member set
// (../unifi-specs objects/setting.json, ips section). It gates the Set so an
// unknown category fails at plan time rather than as a raw controller 4xx.
var ipsCategories = []string{
	"emerging-activex", "emerging-attackresponse", "botcc", "emerging-chat",
	"ciarmy", "compromised", "emerging-dns", "emerging-dos",
	"dshield", "emerging-exploit", "emerging-ftp", "emerging-games",
	"emerging-icmp", "emerging-icmpinfo", "emerging-imap", "emerging-inappropriate",
	"emerging-info", "emerging-malware", "emerging-misc", "emerging-mobile",
	"emerging-netbios", "emerging-p2p", "emerging-policy", "emerging-pop3",
	"emerging-rpc", "emerging-scada", "emerging-scan", "emerging-shellcode",
	"emerging-smtp", "emerging-snmp", "emerging-sql", "emerging-telnet",
	"emerging-tftp", "tor", "emerging-useragent", "emerging-voip",
	"emerging-webapps", "emerging-webclient", "emerging-webserver", "emerging-worm",
	"exploit-kit", "adware-pup", "botcc-portgrouped", "phishing",
	"threatview-cs-c2", "3coresec", "chat", "coinminer",
	"current-events", "drop", "hunting", "icmp-info",
	"inappropriate", "info", "ja3", "policy",
	"scada", "dark-web-blocker-list", "malicious-hosts",
}

func NewThreatManagementResource() resource.Resource { return &threatManagementResource{} }

type threatManagementResource struct {
	net *network.Client
}

// threatManagementModel is the `ips` settings section as flat attributes. Only
// the controller's validated writable fields are modeled; server-managed
// (_id/site_id/key) and the per-network `honeypot` array are left to the
// controller (honeypot writes need live verification — see TODO).
type threatManagementModel struct {
	ID                       types.String `tfsdk:"id"`
	Mode                     types.String `tfsdk:"mode"`
	EnabledCategories        types.Set    `tfsdk:"enabled_categories"`
	EnabledNetworks          types.Set    `tfsdk:"enabled_networks"`
	AdvancedFiltering        types.String `tfsdk:"advanced_filtering_preference"`
	RestrictTorrents         types.Bool   `tfsdk:"restrict_torrents"`
	HoneypotEnabled          types.Bool   `tfsdk:"honeypot_enabled"`
	MemoryOptimized          types.Bool   `tfsdk:"memory_optimized"`
	ContentFilteringBlocking types.Bool   `tfsdk:"content_filtering_blocking_page_enabled"`
}

// ipsPayload is the omitempty owned-field overlay written to set/setting/ips. A
// nil field is omitted so the controller shallow-merge keeps the stored value
// (the sparse-PUT contract). enabled_categories/enabled_networks are composite
// arrays the resource owns wholesale: a *[]string so omitempty distinguishes an
// absent (null/unmanaged) set, which is omitted, from a configured empty set,
// which is sent as `[]` to clear the controller's list — matching the read side
// (sliceToStringSet maps `[]` back to an empty set, not null).
type ipsPayload struct {
	IpsMode                  *string   `json:"ips_mode,omitempty"`
	EnabledCategories        *[]string `json:"enabled_categories,omitempty"`
	EnabledNetworks          *[]string `json:"enabled_networks,omitempty"`
	AdvancedFiltering        *string   `json:"advanced_filtering_preference,omitempty"`
	RestrictTorrents         *bool     `json:"restrict_torrents,omitempty"`
	HoneypotEnabled          *bool     `json:"honeypot_enabled,omitempty"`
	MemoryOptimized          *bool     `json:"memory_optimized,omitempty"`
	ContentFilteringBlocking *bool     `json:"content_filtering_blocking_page_enabled,omitempty"`
}

// ipsDoc decodes the stored `ips` section. Pointers/slices distinguish an absent
// field (nil → null attribute) from a present zero value.
type ipsDoc struct {
	IpsMode                  *string  `json:"ips_mode"`
	EnabledCategories        []string `json:"enabled_categories"`
	EnabledNetworks          []string `json:"enabled_networks"`
	AdvancedFiltering        *string  `json:"advanced_filtering_preference"`
	RestrictTorrents         *bool    `json:"restrict_torrents"`
	HoneypotEnabled          *bool    `json:"honeypot_enabled"`
	MemoryOptimized          *bool    `json:"memory_optimized"`
	ContentFilteringBlocking *bool    `json:"content_filtering_blocking_page_enabled"`
}

func (r *threatManagementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_threat_management"
}

func (r *threatManagementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "UniFi **Threat Management** (IDS/IPS) — the site `ips` settings singleton. " +
			"`mode` toggles the engine off (`disabled`), detection-only (`ids`), or prevention " +
			"(`ips`/`ipsInline`); `enabled_categories` selects the threat signature categories. " +
			"Writes shallow-merge the configured fields over the controller's stored section, so " +
			"unmodeled fields (per-network honeypot entries) are preserved. There is one document per " +
			"site; destroy removes the resource from state without disabling Threat Management.\n\n" +
			"**Enabling IDS/IPS requires a UniFi gateway (UDM/UCG/USG) with an active CyberSecure " +
			"subscription.** Without it the controller accepts the write but silently keeps the engine " +
			"`disabled`; the provider detects that and fails with a clear error rather than looping on " +
			"drift. A console with no gateway is rejected at plan time. `mode = \"disabled\"` (and the " +
			"non-engine flags) work everywhere.\n\n" +
			"**The enabled (non-`disabled`) write path is modeled from the reverse-engineered controller " +
			"spec and has NOT been verified against live IDS/IPS hardware — no CyberSecure-licensed " +
			"console was available. See the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed identifier of the Threat Management singleton (`ips`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Engine mode: `disabled` (off), `ids` (detection — log only), `ips` " +
					"(prevention — block), or `ipsInline` (prevention, inline on the data path).",
				Validators:    []validator.String{stringvalidator.OneOf(ipsModes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled_categories": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Threat signature categories to enable (e.g. `emerging-malware`, `botcc`, `tor`). " +
					"Set to `[]` to clear every category; omit the attribute to leave the controller's selection unmanaged.",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(ipsCategories...)),
				},
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"enabled_networks": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Network (`networkconf`) ids the engine inspects. Omit to leave the " +
					"controller's selection (typically all networks) unmanaged.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"advanced_filtering_preference": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Advanced (per-category) filtering: `manual` to manage categories explicitly, or `disabled`.",
				Validators:          []validator.String{stringvalidator.OneOf(advancedFilteringPreferences...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"restrict_torrents": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Block BitTorrent / peer-to-peer traffic.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"honeypot_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Enable the honeypot feature. Per-network honeypot IP entries are managed " +
					"by the controller and are not modeled here.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"memory_optimized": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Trade detection coverage for lower memory use on constrained gateways.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"content_filtering_blocking_page_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Show a block page when content filtering denies a request.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *threatManagementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *threatManagementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan threatManagementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threatManagementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state threatManagementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := r.net.Settings().GetKey(ctx, threatManagementID)
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read Threat Management settings", err.Error())
		return
	}
	r.refresh(&state, raw, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *threatManagementResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan threatManagementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond state removal: the `ips` document always exists;
// removing the resource stops managing Threat Management without disabling it.
func (r *threatManagementResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState seeds the singleton id; the next Read folds in the controller's
// stored `ips` document.
func (r *threatManagementResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), threatManagementID)...)
}

// ModifyPlan gates the wholly-unsupported case: a plan that enables Threat
// Management on a console with no IPS capability fails at plan time, not with an
// opaque apply error (invariant 3). A subscription-less console that DOES list IPS
// is indistinguishable here — that's caught at apply (verifyThreatManagementEnabled).
func (r *threatManagementResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.net == nil {
		return // destroy, or not yet configured
	}
	var plan threatManagementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !threatManagementWantsEnable(&plan) {
		return
	}
	feats, err := r.net.Features(ctx)
	if err != nil {
		return // can't probe (old firmware / offline) — don't block; apply surfaces real errors
	}
	if feats.Has(ipsFeature) {
		return
	}
	resp.Diagnostics.AddError(
		"Threat Management (IDS/IPS) is not available on this console",
		"The console does not report the IPS capability, which UniFi exposes on gateways (UDM/UCG/USG). "+
			"Threat Management cannot be enabled here. Remove unifi_network_threat_management, or set mode = \"disabled\".",
	)
}

// threatManagementWantsEnable reports whether the plan turns Threat Management on
// in any way — a non-"disabled" mode, or a non-empty category/network selection.
// Only then does the IPS capability matter, so the disabled/plumbing path skips
// the plan-time probe entirely.
func threatManagementWantsEnable(m *threatManagementModel) bool {
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() && m.Mode.ValueString() != "disabled" {
		return true
	}
	for _, s := range []types.Set{m.EnabledCategories, m.EnabledNetworks} {
		if !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 {
			return true
		}
	}
	return false
}

// apply writes the owned `ips` fields via set/setting/ips and folds the
// controller's stored copy back into the plan so Computed fields settle. It then
// verifies the controller actually honored a requested enable — see
// verifyThreatManagementEnabled.
func (r *threatManagementResource) apply(ctx context.Context, plan *threatManagementModel, diags *diag.Diagnostics) {
	payload := r.toPayload(ctx, plan, diags)
	if diags.HasError() {
		return
	}
	want := *plan // configured intent, captured before refresh overwrites the fields
	raw, err := r.net.Settings().SetKey(ctx, threatManagementID, payload)
	if err != nil {
		diags.AddError("Failed to apply Threat Management settings", err.Error())
		return
	}
	plan.ID = types.StringValue(threatManagementID)
	r.refresh(plan, raw, diags)
	if diags.HasError() {
		return
	}
	verifyThreatManagementEnabled(&want, plan, diags)
}

// verifyThreatManagementEnabled is the subscription check. Without active
// CyberSecure the controller accepts the write (HTTP 200) but silently refuses to
// enable: a non-"disabled" mode reads back "disabled", a non-empty category/network
// selection reads back empty. Undetected, that surfaces as the framework's opaque
// "inconsistent result after apply" and churns the field every plan; caught here it
// becomes a clear diagnostic. The write response is the only authoritative signal —
// the IPS feature flag can't gate it at plan time (a console can list IPS yet still
// refuse, confirmed live).
func verifyThreatManagementEnabled(want, stored *threatManagementModel, diags *diag.Diagnostics) {
	modeRefused := !want.Mode.IsNull() && !want.Mode.IsUnknown() &&
		want.Mode.ValueString() != "disabled" && stored.Mode.ValueString() == "disabled"
	if modeRefused || setRefused(want.EnabledCategories, stored.EnabledCategories) ||
		setRefused(want.EnabledNetworks, stored.EnabledNetworks) {
		diags.AddError(
			"Threat Management could not be enabled",
			fmt.Sprintf("The controller accepted the write but did not enable Threat Management — the "+
				"requested mode/categories were dropped (stored mode %q). Enabling IDS/IPS requires a UniFi "+
				"gateway with an active CyberSecure subscription; without it the controller keeps the engine "+
				"disabled. Activate CyberSecure on the gateway, or set mode = \"disabled\" with no categories.",
				stored.Mode.ValueString()),
		)
	}
}

// setRefused reports whether a configured, non-empty set came back empty — the
// signature of the controller silently dropping it (the subscription refusal).
func setRefused(want, stored types.Set) bool {
	if want.IsNull() || want.IsUnknown() || len(want.Elements()) == 0 {
		return false
	}
	return stored.IsNull() || len(stored.Elements()) == 0
}

// toPayload builds the omitempty overlay from the configured (non-null) plan
// fields, so apply sends only the fields the resource owns.
func (r *threatManagementResource) toPayload(ctx context.Context, m *threatManagementModel, diags *diag.Diagnostics) *ipsPayload {
	return &ipsPayload{
		IpsMode:                  strPtrOrNil(m.Mode),
		EnabledCategories:        ownedStringSet(ctx, m.EnabledCategories, diags),
		EnabledNetworks:          ownedStringSet(ctx, m.EnabledNetworks, diags),
		AdvancedFiltering:        strPtrOrNil(m.AdvancedFiltering),
		RestrictTorrents:         boolPtrOrNil(m.RestrictTorrents),
		HoneypotEnabled:          boolPtrOrNil(m.HoneypotEnabled),
		MemoryOptimized:          boolPtrOrNil(m.MemoryOptimized),
		ContentFilteringBlocking: boolPtrOrNil(m.ContentFilteringBlocking),
	}
}

// ownedStringSet renders a Set the resource owns wholesale into the write
// overlay: nil (omitted — leave the controller's list unmanaged) when the
// attribute is null/unknown, or a pointer to the slice — including an explicit
// empty `[]` — when it is configured. Behind omitempty on a *[]string only the
// nil case is dropped, so a configured `[]` is sent to clear the list instead of
// being silently swallowed (which would read back non-empty and churn the plan).
func ownedStringSet(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	out := stringSetToSlice(ctx, s, diags)
	if out == nil {
		out = []string{} // a configured empty set marshals as `[]`, not omitted
	}
	return &out
}

// refresh decodes the stored `ips` document into the model so Computed fields
// reflect the controller's stored value after a read or write.
func (r *threatManagementResource) refresh(m *threatManagementModel, raw json.RawMessage, diags *diag.Diagnostics) {
	var doc ipsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		diags.AddError("Failed to decode Threat Management settings", err.Error())
		return
	}
	m.ID = types.StringValue(threatManagementID)
	m.Mode = stringOrNull(deref(doc.IpsMode))
	m.EnabledCategories = sliceToStringSet(doc.EnabledCategories)
	m.EnabledNetworks = sliceToStringSet(doc.EnabledNetworks)
	m.AdvancedFiltering = stringOrNull(deref(doc.AdvancedFiltering))
	m.RestrictTorrents = boolToState(doc.RestrictTorrents)
	m.HoneypotEnabled = boolToState(doc.HoneypotEnabled)
	m.MemoryOptimized = boolToState(doc.MemoryOptimized)
	m.ContentFilteringBlocking = boolToState(doc.ContentFilteringBlocking)
}
