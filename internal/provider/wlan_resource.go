package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                = (*wlanResource)(nil)
	_ resource.ResourceWithConfigure   = (*wlanResource)(nil)
	_ resource.ResourceWithImportState = (*wlanResource)(nil)
)

func NewWLANResource() resource.Resource { return &wlanResource{} }

type wlanResource struct {
	net *network.Client
}

// wlanResourceModel covers the configurable wlanconf surface. Most attributes
// are Optional+Computed: unset leaves the controller's value and reads back what
// it computes, so plans stay clean. Per-client PSKs (private_preshared_keys),
// schedules, and Hotspot 2.0 detail are carried through by GET-merge-PUT but not
// yet modeled as attributes.
type wlanResourceModel struct {
	ID                types.String `tfsdk:"id"`                 // controller _id; import key (computed)
	Name              types.String `tfsdk:"name"`               // the SSID (required)
	Enabled           types.Bool   `tfsdk:"enabled"`            // whether the SSID is broadcast
	NetworkID         types.String `tfsdk:"network_id"`         // network/VLAN this SSID bridges onto (networkconf id)
	SettingPreference types.String `tfsdk:"setting_preference"` // controller auto/manual toggle (computed)

	// Security.
	Security        types.String `tfsdk:"security"`          // open | wpapsk | wpaeap
	WPAMode         types.String `tfsdk:"wpa_mode"`          // wpa2 | wpa3 | … cipher/mode
	Passphrase      types.String `tfsdk:"passphrase"`        // WPA pre-shared key (sensitive)
	WPA3Support     types.Bool   `tfsdk:"wpa3_support"`      // offer WPA3 alongside WPA2
	WPA3Transition  types.Bool   `tfsdk:"wpa3_transition"`   // WPA2/WPA3 transition mode
	WPA3Enhanced192 types.Bool   `tfsdk:"wpa3_enhanced_192"` // WPA3 Enterprise 192-bit
	PMFMode         types.String `tfsdk:"pmf_mode"`          // protected management frames: disabled|optional|required

	// AP coverage / user group.
	Bands       types.Set    `tfsdk:"bands"`         // 2g | 5g | 6g
	APGroupIDs  types.Set    `tfsdk:"ap_group_ids"`  // AP groups this SSID broadcasts on
	APGroupMode types.String `tfsdk:"ap_group_mode"` // all | exclude
	UserGroupID types.String `tfsdk:"user_group_id"` // bandwidth/user group applied to clients

	// Visibility / isolation / client behavior.
	HideSSID            types.Bool `tfsdk:"hide_ssid"`             // don't broadcast the SSID name
	IsGuest             types.Bool `tfsdk:"is_guest"`              // apply guest policies
	L2Isolation         types.Bool `tfsdk:"l2_isolation"`          // block client-to-client traffic
	ProxyARP            types.Bool `tfsdk:"proxy_arp"`             // answer ARP on behalf of clients
	BSSTransition       types.Bool `tfsdk:"bss_transition"`        // 802.11v BSS transition
	UAPSDEnabled        types.Bool `tfsdk:"uapsd_enabled"`         // WMM power save
	FastRoamingEnabled  types.Bool `tfsdk:"fast_roaming_enabled"`  // 802.11r fast roaming
	MCastEnhanceEnabled types.Bool `tfsdk:"mcast_enhance_enabled"` // multicast enhancement
	No2GHzOUI           types.Bool `tfsdk:"no_2ghz_oui"`           // steer known 5 GHz clients off 2.4 GHz
	MLOEnabled          types.Bool `tfsdk:"mlo_enabled"`           // Wi-Fi 7 multi-link operation

	// MAC filter.
	MACFilterEnabled types.Bool   `tfsdk:"mac_filter_enabled"` // enable the MAC allow/deny list
	MACFilterPolicy  types.String `tfsdk:"mac_filter_policy"`  // allow | deny
	MACFilterList    types.Set    `tfsdk:"mac_filter_list"`    // the MAC addresses

	// Minimum data rate (ng = 2.4 GHz, na = 5 GHz).
	MinrateNGEnabled      types.Bool  `tfsdk:"minrate_ng_enabled"`        // enforce a 2.4 GHz minimum rate
	MinrateNGDataRateKbps types.Int64 `tfsdk:"minrate_ng_data_rate_kbps"` // 2.4 GHz minimum rate (kbps)
	MinrateNAEnabled      types.Bool  `tfsdk:"minrate_na_enabled"`        // enforce a 5 GHz minimum rate
	MinrateNADataRateKbps types.Int64 `tfsdk:"minrate_na_data_rate_kbps"` // 5 GHz minimum rate (kbps)

	// Advanced.
	GroupRekey           types.Int64 `tfsdk:"group_rekey"`             // GTK rekey interval (seconds)
	RADIUSMACAuthEnabled types.Bool  `tfsdk:"radius_mac_auth_enabled"` // MAC-based RADIUS auth
	Hotspot2Enabled      types.Bool  `tfsdk:"hotspot2_enabled"`        // Hotspot 2.0 / Passpoint
}

func (r *wlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wlan"
}

func (r *wlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStrSet := func(desc string) schema.SetAttribute {
		return schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Wi-Fi network/SSID (`wlanconf`). Models the configurable surface: security " +
			"(WPA2/WPA3, PMF), the network/VLAN it bridges onto, AP groups, bands, MAC filtering, minimum data " +
			"rates, fast roaming, and guest policies.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":               schema.StringAttribute{Required: true, MarkdownDescription: "The SSID."},
			"enabled":            optBool("Whether the SSID is broadcast."),
			"network_id":         optStr("ID of the network/VLAN (`unifi_network_network`) this SSID bridges onto."),
			"setting_preference": schema.StringAttribute{Computed: true, MarkdownDescription: "Controller auto/manual toggle (read-only)."},

			"security": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Security suite: `open`, `wpapsk` (personal), or `wpaeap` (enterprise/RADIUS).",
				Validators:          []validator.String{stringvalidator.OneOf("open", "wpapsk", "wpaeap")},
			},
			"wpa_mode": optStr("WPA cipher/mode (e.g. `wpa2`, `wpa3`)."),
			"passphrase": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "WPA pre-shared key (for `security = wpapsk`). The controller echoes it on read, so it is also Computed: unset for `open`/`wpaeap` SSIDs it reads back as the controller's value (empty).",
			},
			"wpa3_support":      optBool("Offer WPA3 alongside WPA2."),
			"wpa3_transition":   optBool("WPA2/WPA3 transition mode."),
			"wpa3_enhanced_192": optBool("WPA3 Enterprise 192-bit mode."),
			"pmf_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Protected Management Frames: `disabled`, `optional`, or `required`.",
				Validators:          []validator.String{stringvalidator.OneOf("disabled", "optional", "required")},
			},

			"bands":        optStrSet("Radio bands to broadcast on: `2g`, `5g`, `6g`."),
			"ap_group_ids": optStrSet("IDs of the AP groups (`unifi_network_ap_group`) this SSID broadcasts on."),
			"ap_group_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How `ap_group_ids` is applied: `all` or `exclude`.",
				Validators:          []validator.String{stringvalidator.OneOf("all", "exclude")},
			},
			"user_group_id": optStr("ID of the user group (bandwidth profile) applied to clients."),

			"hide_ssid":             optBool("Don't broadcast the SSID name."),
			"is_guest":              optBool("Apply guest policies (isolation, captive portal)."),
			"l2_isolation":          optBool("Block client-to-client (layer 2) traffic."),
			"proxy_arp":             optBool("Answer ARP on behalf of clients."),
			"bss_transition":        optBool("802.11v BSS transition management."),
			"uapsd_enabled":         optBool("WMM power save (U-APSD)."),
			"fast_roaming_enabled":  optBool("802.11r fast roaming."),
			"mcast_enhance_enabled": optBool("Multicast enhancement (convert multicast to unicast)."),
			"no_2ghz_oui":           optBool("Steer known 5 GHz-capable clients off 2.4 GHz."),
			"mlo_enabled":           optBool("Wi-Fi 7 multi-link operation."),

			"mac_filter_enabled": optBool("Enable the MAC address allow/deny list."),
			"mac_filter_policy": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "MAC filter policy: `allow` (whitelist) or `deny` (blacklist).",
				Validators:          []validator.String{stringvalidator.OneOf("allow", "deny")},
			},
			"mac_filter_list": optStrSet("MAC addresses for the filter list."),

			"minrate_ng_enabled":        optBool("Enforce a minimum data rate on 2.4 GHz."),
			"minrate_ng_data_rate_kbps": optInt("2.4 GHz minimum data rate (kbps)."),
			"minrate_na_enabled":        optBool("Enforce a minimum data rate on 5 GHz."),
			"minrate_na_data_rate_kbps": optInt("5 GHz minimum data rate (kbps)."),

			"group_rekey": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "GTK group-rekey interval in seconds.",
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"radius_mac_auth_enabled": optBool("MAC-based RADIUS authentication."),
			"hotspot2_enabled":        optBool("Hotspot 2.0 / Passpoint."),
		},
	}
}

func (r *wlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *wlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.WiFi().WLANs().Create(ctx, plan.toWLANConf(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create WLAN", err.Error())
		return
	}
	plan.fromWLANConf(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.net.WiFi().WLANs().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read WLAN", err.Error())
		return
	}
	state.fromWLANConf(w)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.WiFi().WLANs().Update(ctx, plan.ID.ValueString(), plan.toWLANConf(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update WLAN", err.Error())
		return
	}
	plan.fromWLANConf(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.WiFi().WLANs().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete WLAN", err.Error())
	}
}

func (r *wlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *wlanResourceModel) toWLANConf(ctx context.Context, diags *diag.Diagnostics) *wire.Wlanconf {
	w := &wire.Wlanconf{
		UnderscoreId:          strPtrOrNil(m.ID),
		Name:                  strPtrOrNil(m.Name),
		Enabled:               boolPtr(m.Enabled),
		NetworkconfId:         strPtrOrNil(m.NetworkID),
		Security:              (*wire.WlanconfSecurity)(strPtrOrNil(m.Security)),
		WpaMode:               (*wire.WlanconfWpaMode)(strPtrOrNil(m.WPAMode)),
		XPassphrase:           strPtrOrNil(m.Passphrase),
		Wpa3Support:           boolPtr(m.WPA3Support),
		Wpa3Transition:        boolPtr(m.WPA3Transition),
		Wpa3Enhanced192:       boolPtr(m.WPA3Enhanced192),
		PmfMode:               (*wire.WlanconfPmfMode)(strPtrOrNil(m.PMFMode)),
		ApGroupMode:           (*wire.WlanconfApGroupMode)(strPtrOrNil(m.APGroupMode)),
		UsergroupId:           strPtrOrNil(m.UserGroupID),
		HideSsid:              boolPtr(m.HideSSID),
		IsGuest:               boolPtr(m.IsGuest),
		L2Isolation:           boolPtr(m.L2Isolation),
		ProxyArp:              boolPtr(m.ProxyARP),
		BssTransition:         boolPtr(m.BSSTransition),
		UapsdEnabled:          boolPtr(m.UAPSDEnabled),
		FastRoamingEnabled:    boolPtr(m.FastRoamingEnabled),
		McastenhanceEnabled:   boolPtr(m.MCastEnhanceEnabled),
		No2ghzOui:             boolPtr(m.No2GHzOUI),
		MloEnabled:            boolPtr(m.MLOEnabled),
		MacFilterEnabled:      boolPtr(m.MACFilterEnabled),
		MacFilterPolicy:       (*wire.WlanconfMacFilterPolicy)(strPtrOrNil(m.MACFilterPolicy)),
		MinrateNgEnabled:      boolPtr(m.MinrateNGEnabled),
		MinrateNgDataRateKbps: intPtr(m.MinrateNGDataRateKbps),
		MinrateNaEnabled:      boolPtr(m.MinrateNAEnabled),
		MinrateNaDataRateKbps: intPtr(m.MinrateNADataRateKbps),
		GroupRekey:            intPtr(m.GroupRekey),
		RadiusMacAuthEnabled:  boolPtr(m.RADIUSMACAuthEnabled),
		Hotspot2confEnabled:   boolPtr(m.Hotspot2Enabled),
	}
	// Leave these slices nil when the config is null so GET-merge-PUT preserves
	// the controller's bands/groups/filter rather than clearing them (the fields
	// are omitempty — a nil pointer is dropped from the overlay).
	if b := stringSetToSlice(ctx, m.Bands, diags); b != nil {
		bands := make([]wire.WlanconfWlanBands, len(b))
		for i, v := range b {
			bands[i] = wire.WlanconfWlanBands(v)
		}
		w.WlanBands = &bands
	}
	if g := stringSetToSlice(ctx, m.APGroupIDs, diags); g != nil {
		w.ApGroupIds = &g
	}
	if l := stringSetToSlice(ctx, m.MACFilterList, diags); l != nil {
		w.MacFilterList = &l
	}
	return w
}

func (m *wlanResourceModel) fromWLANConf(w *wire.Wlanconf) {
	m.ID = types.StringValue(deref(w.UnderscoreId))
	m.Name = types.StringValue(deref(w.Name))
	m.Enabled = boolToState(w.Enabled)
	m.NetworkID = strToStateOptional(deref(w.NetworkconfId))
	m.SettingPreference = types.StringValue(string(derefEnum(w.SettingPreference)))
	m.Security = types.StringValue(string(derefEnum(w.Security)))
	m.WPAMode = types.StringValue(string(derefEnum(w.WpaMode)))
	// passphrase is Optional+Computed and write-preserving. Adopt the controller's
	// value when it echoes one; if it echoes empty, keep the configured/prior key
	// rather than wiping it, and fall back to "" only when nothing was set (so a
	// Computed value is always known after apply — never unknown/null).
	switch {
	case deref(w.XPassphrase) != "":
		m.Passphrase = types.StringValue(deref(w.XPassphrase))
	case m.Passphrase.IsNull() || m.Passphrase.IsUnknown():
		m.Passphrase = types.StringValue("")
	}
	m.WPA3Support = boolToState(w.Wpa3Support)
	m.WPA3Transition = boolToState(w.Wpa3Transition)
	m.WPA3Enhanced192 = boolToState(w.Wpa3Enhanced192)
	m.PMFMode = types.StringValue(string(derefEnum(w.PmfMode)))
	m.Bands = wlanBandsToStringSet(w.WlanBands)
	m.APGroupIDs = sliceToStringSet(derefSlice(w.ApGroupIds))
	m.APGroupMode = types.StringValue(string(derefEnum(w.ApGroupMode)))
	m.UserGroupID = strToStateOptional(deref(w.UsergroupId))
	m.HideSSID = boolToState(w.HideSsid)
	m.IsGuest = boolToState(w.IsGuest)
	m.L2Isolation = boolToState(w.L2Isolation)
	m.ProxyARP = boolToState(w.ProxyArp)
	m.BSSTransition = boolToState(w.BssTransition)
	m.UAPSDEnabled = boolToState(w.UapsdEnabled)
	m.FastRoamingEnabled = boolToState(w.FastRoamingEnabled)
	m.MCastEnhanceEnabled = boolToState(w.McastenhanceEnabled)
	m.No2GHzOUI = boolToState(w.No2ghzOui)
	m.MLOEnabled = boolToState(w.MloEnabled)
	m.MACFilterEnabled = boolToState(w.MacFilterEnabled)
	m.MACFilterPolicy = types.StringValue(string(derefEnum(w.MacFilterPolicy)))
	m.MACFilterList = sliceToStringSet(derefSlice(w.MacFilterList))
	m.MinrateNGEnabled = boolToState(w.MinrateNgEnabled)
	m.MinrateNGDataRateKbps = intToState(w.MinrateNgDataRateKbps)
	m.MinrateNAEnabled = boolToState(w.MinrateNaEnabled)
	m.MinrateNADataRateKbps = intToState(w.MinrateNaDataRateKbps)
	m.GroupRekey = intToState(w.GroupRekey)
	m.RADIUSMACAuthEnabled = boolToState(w.RadiusMacAuthEnabled)
	m.Hotspot2Enabled = boolToState(w.Hotspot2confEnabled)
}

// derefSlice returns the pointed-to slice, or nil for a nil pointer — so the
// nil/null and explicit-empty distinction sliceToStringSet relies on survives
// the wire type's *[]string fields.
func derefSlice(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

// wlanBandsToStringSet maps the wire enum-band slice (*[]WlanconfWlanBands) back
// to a framework string set, preserving the nil→null distinction.
func wlanBandsToStringSet(p *[]wire.WlanconfWlanBands) types.Set {
	if p == nil {
		return sliceToStringSet(nil)
	}
	out := make([]string, len(*p))
	for i, b := range *p {
		out[i] = string(b)
	}
	return sliceToStringSet(out)
}
