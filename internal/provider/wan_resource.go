package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

var (
	_ resource.Resource                = (*wanResource)(nil)
	_ resource.ResourceWithConfigure   = (*wanResource)(nil)
	_ resource.ResourceWithImportState = (*wanResource)(nil)
)

func NewWANResource() resource.Resource { return &wanResource{} }

type wanResource struct {
	client unifi.Client
}

type wanResourceModel struct {
	ID                types.String `tfsdk:"id"`                 // controller _id; import key (computed)
	Name              types.String `tfsdk:"name"`               // display name (required)
	WANType           types.String `tfsdk:"wan_type"`           // dhcp | pppoe | static (required)
	NetworkGroup      types.String `tfsdk:"network_group"`      // WAN | WAN2 — which uplink interface
	Enabled           types.Bool   `tfsdk:"enabled"`            // uplink enabled on/off
	SettingPreference types.String `tfsdk:"setting_preference"` // controller auto/manual toggle (computed)
	RoutingTableID    types.Int64  `tfsdk:"routing_table_id"`   // policy-routing table ID (computed)
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

	IPv6Enabled        types.Bool   `tfsdk:"ipv6_enabled"`         // IPv6 on the uplink on/off
	IPv6Type           types.String `tfsdk:"ipv6_type"`            // wan_type_v6: disabled | dhcpv6 | static
	IPv6DelegationType types.String `tfsdk:"ipv6_delegation_type"` // IPv6 PD delegation type
	IPv6PDSize         types.Int64  `tfsdk:"ipv6_pd_size"`         // delegated IPv6 prefix size
}

func (r *wanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wan"
}

func (r *wanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A WAN uplink (`networkconf` with `purpose=wan`): DHCP/PPPoE/static, dual-WAN load balancing, " +
			"failover, smart-queue QoS, MAC clone, and IPv6. WAN interfaces are usually fixed in hardware — import the " +
			"existing WAN/WAN2 and manage its configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
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
				MarkdownDescription: "WAN interface group: `WAN` (primary) or `WAN2`.",
				Validators:          []validator.String{stringvalidator.OneOf("WAN", "WAN2")},
			},
			"enabled":            optBool("Whether the uplink is enabled."),
			"setting_preference": schema.StringAttribute{Computed: true, MarkdownDescription: "Controller auto/manual settings toggle (read-only)."},
			"routing_table_id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Routing table ID assigned by the controller (read-only)."},
			"single_network_lan": schema.StringAttribute{Computed: true, MarkdownDescription: "LAN this uplink is bound to (read-only)."},

			"dns_servers": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Up to two IPv4 DNS servers. Setting any switches DNS to manual; empty uses automatic.",
				Validators:          []validator.List{listvalidator.SizeAtMost(2)},
			},
			"ipv6_dns_servers": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
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
				MarkdownDescription: "Multi-WAN role: `weighted` (load balance) or `failover-only`.",
				Validators:          []validator.String{stringvalidator.OneOf("weighted", "failover-only")},
			},
			"load_balance_weight": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
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

			"ipv6_enabled":         optBool("Whether IPv6 is enabled on the uplink."),
			"ipv6_type":            optStr("IPv6 uplink type (`wan_type_v6`, e.g. `disabled`, `dhcpv6`, `static`)."),
			"ipv6_delegation_type": optStr("IPv6 prefix-delegation type (`none`, `dhcpv6`, …)."),
			"ipv6_pd_size":         optInt("IPv6 prefix-delegation size."),
		},
	}
}

func (r *wanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.client = c
	}
}

func (r *wanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateNetwork(ctx, plan.toNetworkConf(ctx, &resp.Diagnostics))
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
	conf, err := r.client.GetNetwork(ctx, state.ID.ValueString())
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
	updated, err := r.client.UpdateNetwork(ctx, plan.toNetworkConf(ctx, &resp.Diagnostics))
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
	if err := r.client.DeleteNetwork(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete WAN", err.Error())
	}
}

func (r *wanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *wanResourceModel) toNetworkConf(ctx context.Context, diags *diag.Diagnostics) *unifi.NetworkConf {
	conf := &unifi.NetworkConf{
		ID:                    m.ID.ValueString(),
		Name:                  m.Name.ValueString(),
		Purpose:               "wan",
		WANType:               strOrEmpty(m.WANType),
		WANNetworkGroup:       strOrEmpty(m.NetworkGroup),
		Enabled:               boolPtr(m.Enabled),
		WANIP:                 strOrEmpty(m.IP),
		WANNetmask:            strOrEmpty(m.Netmask),
		WANGateway:            strOrEmpty(m.Gateway),
		WANLoadBalanceType:    strOrEmpty(m.LoadBalanceType),
		WANLoadBalanceWeight:  intPtr(m.LoadBalanceWeight),
		WANFailoverPriority:   intPtr(m.FailoverPriority),
		WANSmartQEnabled:      boolPtr(m.SmartQEnabled),
		WANSmartQUpRate:       intPtr(m.SmartQUpRate),
		WANSmartQDownRate:     intPtr(m.SmartQDownRate),
		IGMPProxyUpstream:     boolPtr(m.IGMPProxyUpstream),
		IGMPProxyFor:          strOrEmpty(m.IGMPProxyFor),
		ReportWANEvent:        boolPtr(m.ReportWANEvent),
		IPv6Enabled:           boolPtr(m.IPv6Enabled),
		WANTypeV6:             strOrEmpty(m.IPv6Type),
		IPv6WANDelegationType: strOrEmpty(m.IPv6DelegationType),
		WANDHCPv6PDSize:       intPtr(m.IPv6PDSize),
	}

	// PPPoE credentials + their enable flags.
	conf.WANUsername = strOrEmpty(m.PPPoEUsername)
	conf.XWANPassword = strOrEmpty(m.PPPoEPassword)
	uEnabled := conf.WANUsername != ""
	pEnabled := conf.XWANPassword != ""
	conf.WANPPPoEUsernameEnabled = &uEnabled
	conf.WANPPPoEPasswordEnabled = &pEnabled

	// MAC clone: a value enables the override, empty disables it.
	mac := strOrEmpty(m.MACOverride)
	macEnabled := mac != ""
	conf.MACOverride = mac
	conf.MACOverrideEnabled = &macEnabled

	// WAN VLAN tag.
	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		v := int(m.VLANID.ValueInt64())
		conf.WANVLAN = &v
		on := true
		conf.WANVLANEnabled = &on
	} else {
		off := false
		conf.WANVLANEnabled = &off
	}

	// IPv4 DNS: any server → manual, else auto. Only touch the slots when the
	// list is configured; write both owned slots (freed ones as "") so dropping
	// the second server clears wan_dns2 on GET-merge-PUT.
	if !m.DNSServers.IsNull() && !m.DNSServers.IsUnknown() {
		dns := stringListToSlice(ctx, m.DNSServers, diags)
		if len(dns) > 0 {
			conf.WANDNSPreference = "manual"
		} else {
			conf.WANDNSPreference = "auto"
		}
		conf.WANDNS1 = dnsSlotPtr(dns, 0)
		conf.WANDNS2 = dnsSlotPtr(dns, 1)
	}

	// IPv6 DNS: same shape.
	if !m.IPv6DNSServers.IsNull() && !m.IPv6DNSServers.IsUnknown() {
		dns6 := stringListToSlice(ctx, m.IPv6DNSServers, diags)
		if len(dns6) > 0 {
			conf.WANIPv6DNSPreference = "manual"
		} else {
			conf.WANIPv6DNSPreference = "auto"
		}
		conf.WANIPv6DNS1 = dnsSlotPtr(dns6, 0)
		conf.WANIPv6DNS2 = dnsSlotPtr(dns6, 1)
	}

	up := intPtr(m.UploadKilobits)
	down := intPtr(m.DownloadKilobits)
	if up != nil || down != nil {
		conf.WANProviderCapabilities = &unifi.WANProviderCapabilities{
			UploadKilobitsPerSecond:   up,
			DownloadKilobitsPerSecond: down,
		}
	}
	return conf
}

func (m *wanResourceModel) fromNetworkConf(conf *unifi.NetworkConf) {
	m.ID = types.StringValue(conf.ID)
	m.Name = types.StringValue(conf.Name)
	m.WANType = types.StringValue(conf.WANType)
	m.NetworkGroup = types.StringValue(conf.WANNetworkGroup)
	m.Enabled = boolToState(conf.Enabled)
	m.SettingPreference = types.StringValue(conf.SettingPreference)
	m.RoutingTableID = intToState(conf.RoutingTableID)
	m.SingleNetworkLAN = types.StringValue(conf.SingleNetworkLAN)

	m.IP = types.StringValue(conf.WANIP)
	m.Netmask = types.StringValue(conf.WANNetmask)
	m.Gateway = types.StringValue(conf.WANGateway)

	m.LoadBalanceType = types.StringValue(conf.WANLoadBalanceType)
	m.LoadBalanceWeight = intToState(conf.WANLoadBalanceWeight)
	m.FailoverPriority = intToState(conf.WANFailoverPriority)
	m.SmartQEnabled = boolToState(conf.WANSmartQEnabled)
	m.SmartQUpRate = intToState(conf.WANSmartQUpRate)
	m.SmartQDownRate = intToState(conf.WANSmartQDownRate)
	m.IGMPProxyUpstream = boolToState(conf.IGMPProxyUpstream)
	m.IGMPProxyFor = types.StringValue(conf.IGMPProxyFor)
	m.MACOverride = types.StringValue(conf.MACOverride)
	m.ReportWANEvent = boolToState(conf.ReportWANEvent)
	m.IPv6Enabled = boolToState(conf.IPv6Enabled)
	m.IPv6Type = types.StringValue(conf.WANTypeV6)
	m.IPv6DelegationType = types.StringValue(conf.IPv6WANDelegationType)
	m.IPv6PDSize = intToState(conf.WANDHCPv6PDSize)

	m.PPPoEUsername = types.StringValue(conf.WANUsername)
	// Write-only secret: the controller doesn't echo it, so keep the existing
	// (plan/state) value rather than nulling it on read.
	if conf.XWANPassword != "" {
		m.PPPoEPassword = types.StringValue(conf.XWANPassword)
	}

	if conf.WANVLANEnabled != nil && *conf.WANVLANEnabled && conf.WANVLAN != nil {
		m.VLANID = types.Int64Value(int64(*conf.WANVLAN))
	} else {
		m.VLANID = types.Int64Null()
	}

	// Only surface manual DNS as managed values; in "auto" the controller may
	// hold DHCP-learned servers in these slots that we must not adopt.
	if conf.WANDNSPreference == "manual" {
		m.DNSServers = sliceToStringList(dnsSlots(conf.WANDNS1, conf.WANDNS2))
	} else {
		m.DNSServers = sliceToStringList(nil)
	}
	if conf.WANIPv6DNSPreference == "manual" {
		m.IPv6DNSServers = sliceToStringList(dnsSlots(conf.WANIPv6DNS1, conf.WANIPv6DNS2))
	} else {
		m.IPv6DNSServers = sliceToStringList(nil)
	}

	if conf.WANProviderCapabilities != nil {
		m.UploadKilobits = intToState(conf.WANProviderCapabilities.UploadKilobitsPerSecond)
		m.DownloadKilobits = intToState(conf.WANProviderCapabilities.DownloadKilobitsPerSecond)
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
