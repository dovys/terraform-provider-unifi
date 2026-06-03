package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*wanResource)(nil)
	_ resource.ResourceWithConfigure   = (*wanResource)(nil)
	_ resource.ResourceWithImportState = (*wanResource)(nil)
)

func NewWANResource() resource.Resource { return &wanResource{} }

type wanResource struct {
	net *network.Client
}

type wanResourceModel struct {
	ID                types.String `tfsdk:"id"`                 // controller _id; import key (computed)
	Name              types.String `tfsdk:"name"`               // display name (required)
	WANType           types.String `tfsdk:"wan_type"`           // dhcp | pppoe | static (required)
	NetworkGroup      types.String `tfsdk:"network_group"`      // WAN | WAN2 — which uplink interface
	Enabled           types.Bool   `tfsdk:"enabled"`            // uplink enabled on/off
	SettingPreference types.String `tfsdk:"setting_preference"` // controller auto/manual toggle (computed)
	SingleNetworkLAN  types.String `tfsdk:"single_network_lan"` // LAN this uplink is bound to (computed)

	DNSServers     types.List `tfsdk:"dns_servers"`      // up to 2 IPv4 DNS (manual when set, else auto)
	IPv6DNSServers types.List `tfsdk:"ipv6_dns_servers"` // up to 2 IPv6 DNS (manual when set, else auto)

	PPPoEUsername types.String `tfsdk:"pppoe_username"` // PPPoE username (wan_username)
	PPPoEPassword types.String `tfsdk:"pppoe_password"` // PPPoE password (x_wan_password); write-only, sensitive

	IP      types.String `tfsdk:"ip"`      // static WAN IP
	Netmask types.String `tfsdk:"netmask"` // static WAN netmask
	Gateway types.String `tfsdk:"gateway"` // static WAN gateway

	VLANID types.Int64 `tfsdk:"vlan_id"` // 802.1Q uplink VLAN tag; unset = untagged

	LoadBalanceType   types.String `tfsdk:"load_balance_type"`   // weighted | failover-only (dual-WAN role)
	LoadBalanceWeight types.Int64  `tfsdk:"load_balance_weight"` // load-balance weight (0–100)
	FailoverPriority  types.Int64  `tfsdk:"failover_priority"`   // failover priority (lower wins)

	SmartQEnabled  types.Bool  `tfsdk:"smartq_enabled"`   // smart-queue QoS on/off
	SmartQUpRate   types.Int64 `tfsdk:"smartq_up_rate"`   // smart-queue upload limit (kbit/s)
	SmartQDownRate types.Int64 `tfsdk:"smartq_down_rate"` // smart-queue download limit (kbit/s)

	UploadKilobits   types.Int64 `tfsdk:"upload_kilobits"`   // advertised ISP upload (kbit/s)
	DownloadKilobits types.Int64 `tfsdk:"download_kilobits"` // advertised ISP download (kbit/s)

	IGMPProxyUpstream types.Bool   `tfsdk:"igmp_proxy_upstream"` // this uplink is the IGMP-proxy upstream
	IGMPProxyFor      types.String `tfsdk:"igmp_proxy_for"`      // IGMP-proxy downstream network
	MACOverride       types.String `tfsdk:"mac_override"`        // cloned WAN MAC (empty disables clone)
	ReportWANEvent    types.Bool   `tfsdk:"report_wan_event"`    // report WAN up/down events

	IPv6Type           types.String `tfsdk:"ipv6_type"`            // wan_type_v6: disabled | dhcpv6 | static
	IPv6DelegationType types.String `tfsdk:"ipv6_delegation_type"` // IPv6 PD delegation type
	IPv6PDSize         types.Int64  `tfsdk:"ipv6_pd_size"`         // delegated IPv6 prefix size
}

func (r *wanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wan"
}

func (r *wanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// These uplink knobs are Optional+Computed: unset in config, they take the
	// controller's value. UseStateForUnknown holds that value through an unrelated
	// update so an unconfigured field doesn't churn to "known after apply" on every
	// plan (the framework otherwise marks all un-pinned Computed fields unknown the
	// moment any sibling changes). The controller doesn't mutate these on its own,
	// so pinning prior state can't mask server-side drift — Read still reconciles.
	optBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc,
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	}
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	optInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: desc,
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A WAN uplink (`networkconf` with `purpose=wan`): DHCP/PPPoE/static, dual-WAN load balancing, " +
			"failover, smart-queue QoS, MAC clone, and IPv6. WAN interfaces are usually fixed in hardware — import the " +
			"existing WAN/WAN2 and manage its configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key — `terraform import` also accepts `name=<WAN name>` (e.g. `name=WAN1`), since the UniFi UI does not expose the `_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "WAN name."},
			"wan_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Uplink type: `dhcp`, `pppoe`, or `static`.",
				Validators:          []validator.String{stringvalidator.OneOf("dhcp", "pppoe", "static")},
			},
			"network_group": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("WAN"),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "WAN interface group: `WAN` (primary) or `WAN2`.",
				Validators:          []validator.String{stringvalidator.OneOf("WAN", "WAN2")},
			},
			"enabled":            optBool("Whether the uplink is enabled."),
			"setting_preference": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Controller auto/manual settings toggle (read-only)."},
			"single_network_lan": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "LAN this uplink is bound to (read-only)."},

			"dns_servers": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Up to two IPv4 DNS servers. Setting any switches DNS to manual; empty uses automatic.",
				Validators:          []validator.List{listvalidator.SizeAtMost(2)},
			},
			"ipv6_dns_servers": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Up to two IPv6 DNS servers. Setting any switches IPv6 DNS to manual.",
				Validators:          []validator.List{listvalidator.SizeAtMost(2)},
			},

			"pppoe_username": optStr("PPPoE username (`wan_type=pppoe`)."),
			"pppoe_password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "PPPoE password (`wan_type=pppoe`). Write-only — the controller does not return it, so it is not refreshed on read or recoverable on import.",
			},

			"ip":      optStr("Static WAN IP address (`wan_type=static`)."),
			"netmask": optStr("Static WAN netmask (`wan_type=static`)."),
			"gateway": optStr("Static WAN gateway (`wan_type=static`)."),

			"vlan_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "802.1Q VLAN tag for the uplink (2–4094). Omit for untagged.",
				Validators:          []validator.Int64{int64validator.Between(2, 4094)},
			},

			"load_balance_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Multi-WAN role: `weighted` (load balance) or `failover-only`.",
				Validators:          []validator.String{stringvalidator.OneOf("weighted", "failover-only")},
			},
			"load_balance_weight": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				MarkdownDescription: "Load-balance weight (0–100) when `load_balance_type=weighted`.",
				Validators:          []validator.Int64{int64validator.Between(0, 100)},
			},
			"failover_priority": optInt("Failover priority (lower wins) when `load_balance_type=failover-only`."),

			"smartq_enabled":   optBool("Whether smart-queue QoS is enabled on this uplink."),
			"smartq_up_rate":   optInt("Smart-queue upload limit (kbit/s)."),
			"smartq_down_rate": optInt("Smart-queue download limit (kbit/s)."),

			"upload_kilobits":   optInt("Advertised ISP upload bandwidth (kbit/s), used for smart-queue/load-balancing."),
			"download_kilobits": optInt("Advertised ISP download bandwidth (kbit/s)."),

			"igmp_proxy_upstream": optBool("Whether this uplink is the IGMP-proxy upstream."),
			"igmp_proxy_for":      optStr("IGMP-proxy downstream network."),
			"mac_override":        optStr("Clone this MAC address on the WAN interface (empty disables MAC override)."),
			"report_wan_event":    optBool("Whether WAN up/down events are reported."),

			"ipv6_type":            optStr("IPv6 uplink type (`wan_type_v6`, e.g. `disabled`, `dhcpv6`, `static`). Set to `disabled` to turn IPv6 off on the uplink."),
			"ipv6_delegation_type": optStr("IPv6 prefix-delegation type (`none`, `dhcpv6`, …)."),
			"ipv6_pd_size":         optInt("IPv6 prefix-delegation size."),
		},
	}
}

func (r *wanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *wanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.Networks().Create(ctx, plan.toNetworkConf(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create WAN", err.Error())
		return
	}
	plan.fromNetworkConf(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	conf, err := r.net.Networks().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read WAN", err.Error())
		return
	}
	state.fromNetworkConf(conf)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Networks().Update(ctx, plan.toNetworkConf(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update WAN", err.Error())
		return
	}
	plan.fromNetworkConf(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.Networks().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete WAN", err.Error())
	}
}

// ImportState accepts the controller `_id` or a friendly `name=<WAN name>` key
// (e.g. `name=WAN1`), resolved to the `_id` via the underlying networkconf.
func (r *wanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByNameOrID(ctx, req, resp, "WAN", func(ctx context.Context, name string) (string, error) {
		n, err := r.net.Networks().GetByName(ctx, name)
		if err != nil {
			return "", err
		}
		return deref(n.UnderscoreId), nil
	})
}

func (m *wanResourceModel) toNetworkConf(ctx context.Context, diags *diag.Diagnostics) *wire.Networkconf {
	wanPurpose := wire.NetworkconfPurpose("wan")
	conf := &wire.Networkconf{
		UnderscoreId:          strPtrOrNil(m.ID),
		Name:                  strPtrOrNil(m.Name),
		Purpose:               &wanPurpose,
		WanType:               (*wire.NetworkconfWanType)(strPtrOrNil(m.WANType)),
		WanNetworkgroup:       strPtrOrNil(m.NetworkGroup),
		Enabled:               boolPtr(m.Enabled),
		WanIp:                 strPtrOrNil(m.IP),
		WanNetmask:            strPtrOrNil(m.Netmask),
		WanGateway:            strPtrOrNil(m.Gateway),
		WanLoadBalanceType:    (*wire.NetworkconfWanLoadBalanceType)(strPtrOrNil(m.LoadBalanceType)),
		WanLoadBalanceWeight:  intPtr(m.LoadBalanceWeight),
		WanFailoverPriority:   intPtr(m.FailoverPriority),
		WanSmartqEnabled:      boolPtr(m.SmartQEnabled),
		WanSmartqUpRate:       intPtr(m.SmartQUpRate),
		WanSmartqDownRate:     intPtr(m.SmartQDownRate),
		IgmpProxyUpstream:     boolPtr(m.IGMPProxyUpstream),
		IgmpProxyFor:          (*wire.NetworkconfIgmpProxyFor)(strPtrOrNil(m.IGMPProxyFor)),
		ReportWanEvent:        boolPtr(m.ReportWANEvent),
		WanTypeV6:             (*wire.NetworkconfWanTypeV6)(strPtrOrNil(m.IPv6Type)),
		Ipv6WanDelegationType: (*wire.NetworkconfIpv6WanDelegationType)(strPtrOrNil(m.IPv6DelegationType)),
		WanDhcpv6PdSize:       intPtr(m.IPv6PDSize),
	}

	// PPPoE username + its enable flag (read back, so always safe to send).
	wanUsername := strOrEmpty(m.PPPoEUsername)
	conf.WanUsername = &wanUsername
	uEnabled := wanUsername != ""
	conf.WanPppoeUsernameEnabled = &uEnabled

	// pppoe_password is write-only (the controller never echoes it). Send it only
	// when explicitly set in config: an unset password must be left untouched so
	// GET-merge-PUT preserves the stored secret — sending "" would blank it and
	// drop a live PPPoE uplink on any update. Set it to "" explicitly to clear it.
	if !m.PPPoEPassword.IsNull() && !m.PPPoEPassword.IsUnknown() {
		xWanPassword := m.PPPoEPassword.ValueString()
		conf.XWanPassword = &xWanPassword
		pEnabled := xWanPassword != ""
		conf.WanPppoePasswordEnabled = &pEnabled
	}

	// MAC clone: a value enables the override, empty disables it.
	mac := strOrEmpty(m.MACOverride)
	macEnabled := mac != ""
	conf.MacOverride = &mac
	conf.MacOverrideEnabled = &macEnabled

	// WAN VLAN tag.
	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		v := int(m.VLANID.ValueInt64())
		conf.WanVlan = &v
		on := true
		conf.WanVlanEnabled = &on
	} else {
		off := false
		conf.WanVlanEnabled = &off
	}

	// IPv4 DNS: any server → manual, else auto. Only touch the slots when the
	// list is configured; write both owned slots (freed ones as "") so dropping
	// the second server clears wan_dns2 on GET-merge-PUT.
	if !m.DNSServers.IsNull() && !m.DNSServers.IsUnknown() {
		dns := stringListToSlice(ctx, m.DNSServers, diags)
		pref := wire.NetworkconfWanDnsPreference("auto")
		if len(dns) > 0 {
			pref = "manual"
		}
		conf.WanDnsPreference = &pref
		conf.WanDns1 = dnsSlotPtr(dns, 0)
		conf.WanDns2 = dnsSlotPtr(dns, 1)
	}

	// IPv6 DNS: same shape.
	if !m.IPv6DNSServers.IsNull() && !m.IPv6DNSServers.IsUnknown() {
		dns6 := stringListToSlice(ctx, m.IPv6DNSServers, diags)
		pref6 := wire.NetworkconfWanIpv6DnsPreference("auto")
		if len(dns6) > 0 {
			pref6 = "manual"
		}
		conf.WanIpv6DnsPreference = &pref6
		conf.WanIpv6Dns1 = dnsSlotPtr(dns6, 0)
		conf.WanIpv6Dns2 = dnsSlotPtr(dns6, 1)
	}

	up := intPtr(m.UploadKilobits)
	down := intPtr(m.DownloadKilobits)
	if up != nil || down != nil {
		conf.WanProviderCapabilities = &struct {
			DownloadKilobitsPerSecond *int `json:"download_kilobits_per_second,omitempty"`
			UploadKilobitsPerSecond   *int `json:"upload_kilobits_per_second,omitempty"`
		}{
			UploadKilobitsPerSecond:   up,
			DownloadKilobitsPerSecond: down,
		}
	}
	return conf
}

func (m *wanResourceModel) fromNetworkConf(conf *wire.Networkconf) {
	m.ID = types.StringValue(deref(conf.UnderscoreId))
	m.Name = types.StringValue(deref(conf.Name))
	m.WANType = types.StringValue(string(derefEnum(conf.WanType)))
	m.NetworkGroup = types.StringValue(deref(conf.WanNetworkgroup))
	m.Enabled = boolToState(conf.Enabled)
	m.SettingPreference = types.StringValue(string(derefEnum(conf.SettingPreference)))
	m.SingleNetworkLAN = types.StringValue(deref(conf.SingleNetworkLan))

	m.IP = types.StringValue(deref(conf.WanIp))
	m.Netmask = types.StringValue(deref(conf.WanNetmask))
	m.Gateway = types.StringValue(deref(conf.WanGateway))

	m.LoadBalanceType = types.StringValue(string(derefEnum(conf.WanLoadBalanceType)))
	m.LoadBalanceWeight = intToState(conf.WanLoadBalanceWeight)
	m.FailoverPriority = intToState(conf.WanFailoverPriority)
	m.SmartQEnabled = boolToState(conf.WanSmartqEnabled)
	m.SmartQUpRate = intToState(conf.WanSmartqUpRate)
	m.SmartQDownRate = intToState(conf.WanSmartqDownRate)
	m.IGMPProxyUpstream = boolToState(conf.IgmpProxyUpstream)
	m.IGMPProxyFor = types.StringValue(string(derefEnum(conf.IgmpProxyFor)))
	m.MACOverride = types.StringValue(deref(conf.MacOverride))
	m.ReportWANEvent = boolToState(conf.ReportWanEvent)
	m.IPv6Type = types.StringValue(string(derefEnum(conf.WanTypeV6)))
	m.IPv6DelegationType = types.StringValue(string(derefEnum(conf.Ipv6WanDelegationType)))
	m.IPv6PDSize = intToState(conf.WanDhcpv6PdSize)

	m.PPPoEUsername = types.StringValue(deref(conf.WanUsername))
	// Write-only secret: the controller doesn't echo it, so keep the existing
	// (plan/state) value rather than nulling it on read.
	if deref(conf.XWanPassword) != "" {
		m.PPPoEPassword = types.StringValue(deref(conf.XWanPassword))
	}

	if conf.WanVlanEnabled != nil && *conf.WanVlanEnabled && conf.WanVlan != nil {
		m.VLANID = types.Int64Value(int64(*conf.WanVlan))
	} else {
		m.VLANID = types.Int64Null()
	}

	// Only surface manual DNS as managed values; in "auto" the controller may
	// hold DHCP-learned servers in these slots that we must not adopt.
	if string(derefEnum(conf.WanDnsPreference)) == "manual" {
		m.DNSServers = sliceToStringList(dnsSlots(conf.WanDns1, conf.WanDns2))
	} else {
		m.DNSServers = sliceToStringList(nil)
	}
	if string(derefEnum(conf.WanIpv6DnsPreference)) == "manual" {
		m.IPv6DNSServers = sliceToStringList(dnsSlots(conf.WanIpv6Dns1, conf.WanIpv6Dns2))
	} else {
		m.IPv6DNSServers = sliceToStringList(nil)
	}

	if conf.WanProviderCapabilities != nil {
		m.UploadKilobits = intToState(conf.WanProviderCapabilities.UploadKilobitsPerSecond)
		m.DownloadKilobits = intToState(conf.WanProviderCapabilities.DownloadKilobitsPerSecond)
	} else {
		m.UploadKilobits = types.Int64Null()
		m.DownloadKilobits = types.Int64Null()
	}
}

// dnsSlots collects the non-empty DNS slot values, preserving order. nil and
// "" slots are skipped — the wire model uses *string so a cleared slot reads
// back as nil or "".
func dnsSlots(vals ...*string) []string {
	var out []string
	for _, v := range vals {
		if v != nil && *v != "" {
			out = append(out, *v)
		}
	}
	return out
}

// dnsSlotPtr returns the i-th DNS server as a pointer, or a pointer to "" when
// the list has no i-th element. A non-nil &"" overwrites (clears) a freed slot
// on GET-merge-PUT; a nil pointer would be dropped by omitempty and leave the
// controller's stale value in place.
func dnsSlotPtr(vals []string, i int) *string {
	if i < len(vals) {
		v := vals[i]
		return &v
	}
	empty := ""
	return &empty
}
