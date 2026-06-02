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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*networkResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkResource)(nil)
	_ resource.ResourceWithImportState = (*networkResource)(nil)
)

func NewNetworkResource() resource.Resource { return &networkResource{} }

type networkResource struct {
	net *network.Client
}

// networkResourceModel covers the full LAN/VLAN/guest networkconf surface.
// Most attributes are Optional+Computed: unset leaves the controller's value
// and reads back whatever it computes, so plans stay clean.
type networkResourceModel struct {
	ID                types.String `tfsdk:"id"`                 // controller _id; import key (computed)
	Name              types.String `tfsdk:"name"`               // display name (required)
	Purpose           types.String `tfsdk:"purpose"`            // corporate | guest | vlan-only (immutable)
	NetworkGroup      types.String `tfsdk:"network_group"`      // controller network group, e.g. LAN (computed)
	SettingPreference types.String `tfsdk:"setting_preference"` // controller auto/manual toggle (computed)
	GatewayType       types.String `tfsdk:"gateway_type"`       // gateway type, e.g. default (computed)
	Enabled           types.Bool   `tfsdk:"enabled"`            // network enabled on/off

	VLANID                types.Int64  `tfsdk:"vlan_id"`                 // 802.1Q VLAN tag (2–4094); unset = untagged
	Subnet                types.String `tfsdk:"subnet"`                  // gateway IP + prefix (CIDR)
	DomainName            types.String `tfsdk:"domain_name"`             // DHCP domain name
	NATEnabled            types.Bool   `tfsdk:"nat_enabled"`             // NAT the subnet (is_nat)
	InternetAccessEnabled types.Bool   `tfsdk:"internet_access_enabled"` // allow internet egress
	IGMPSnooping          types.Bool   `tfsdk:"igmp_snooping"`           // IGMP snooping (multicast pruning)
	UPNPEnabled           types.Bool   `tfsdk:"upnp_enabled"`            // UPnP IGD on this LAN
	DHCPGuardEnabled      types.Bool   `tfsdk:"dhcp_guard_enabled"`      // block rogue DHCP servers
	AutoScaleEnabled      types.Bool   `tfsdk:"auto_scale_enabled"`      // auto-grow the subnet
	LTEFailoverEnabled    types.Bool   `tfsdk:"lte_failover_enabled"`    // designate as the LTE-failover LAN

	DHCPEnabled           types.Bool   `tfsdk:"dhcp_enabled"`             // built-in DHCP server on/off
	DHCPStart             types.String `tfsdk:"dhcp_start"`               // DHCP pool first address
	DHCPStop              types.String `tfsdk:"dhcp_stop"`                // DHCP pool last address
	DHCPLeaseTime         types.Int64  `tfsdk:"dhcp_lease_time"`          // lease time (seconds)
	DHCPDNS               types.List   `tfsdk:"dhcp_dns"`                 // up to 4 DHCP DNS servers (sets dhcpd_dns_1..4)
	DHCPGatewayEnabled    types.Bool   `tfsdk:"dhcp_gateway_enabled"`     // hand out a custom gateway
	DHCPGateway           types.String `tfsdk:"dhcp_gateway"`             // custom DHCP gateway address
	DHCPNTPEnabled        types.Bool   `tfsdk:"dhcp_ntp_enabled"`         // hand out NTP servers via DHCP
	DHCPNTPServer1        types.String `tfsdk:"dhcp_ntp_server_1"`        // DHCP NTP server 1
	DHCPNTPServer2        types.String `tfsdk:"dhcp_ntp_server_2"`        // DHCP NTP server 2
	DHCPWINSEnabled       types.Bool   `tfsdk:"dhcp_wins_enabled"`        // hand out WINS servers via DHCP
	DHCPWINSServer1       types.String `tfsdk:"dhcp_wins_server_1"`       // DHCP WINS server 1
	DHCPWINSServer2       types.String `tfsdk:"dhcp_wins_server_2"`       // DHCP WINS server 2
	DHCPTFTPServer        types.String `tfsdk:"dhcp_tftp_server"`         // DHCP option 66 (TFTP server)
	DHCPBootEnabled       types.Bool   `tfsdk:"dhcp_boot_enabled"`        // hand out PXE network-boot options
	DHCPBootServer        types.String `tfsdk:"dhcp_boot_server"`         // PXE next-server address
	DHCPBootFilename      types.String `tfsdk:"dhcp_boot_filename"`       // DHCP option 67 (boot file)
	DHCPUnifiController   types.String `tfsdk:"dhcp_unifi_controller"`    // DHCP option 43 controller inform address
	DHCPConflictChecking  types.Bool   `tfsdk:"dhcp_conflict_checking"`   // probe for IP conflicts before leasing
	DHCPTimeOffsetEnabled types.Bool   `tfsdk:"dhcp_time_offset_enabled"` // hand out the DHCP time-offset option
	DHCPWPADURL           types.String `tfsdk:"dhcp_wpad_url"`            // WPAD proxy auto-config URL
	DHCPRelayEnabled      types.Bool   `tfsdk:"dhcp_relay_enabled"`       // relay DHCP instead of serving locally

	IPv6InterfaceType           types.String `tfsdk:"ipv6_interface_type"`            // none | pd | static
	IPv6RAEnabled               types.Bool   `tfsdk:"ipv6_ra_enabled"`                // send IPv6 router advertisements
	IPv6RAPriority              types.String `tfsdk:"ipv6_ra_priority"`               // RA priority: high | medium | low
	IPv6RAPreferredLifetime     types.Int64  `tfsdk:"ipv6_ra_preferred_lifetime"`     // RA preferred lifetime (seconds)
	IPv6PDStart                 types.String `tfsdk:"ipv6_pd_start"`                  // prefix-delegation pool start
	IPv6PDStop                  types.String `tfsdk:"ipv6_pd_stop"`                   // prefix-delegation pool end
	IPv6ClientAddressAssignment types.String `tfsdk:"ipv6_client_address_assignment"` // client IPv6 assignment, e.g. slaac
	DHCPv6DNSAuto               types.Bool   `tfsdk:"dhcpv6_dns_auto"`                // DHCPv6 DNS auto vs manual
	DHCPv6LeaseTime             types.Int64  `tfsdk:"dhcpv6_lease_time"`              // DHCPv6 lease time (seconds)
	DHCPv6Start                 types.String `tfsdk:"dhcpv6_start"`                   // DHCPv6 pool start
	DHCPv6Stop                  types.String `tfsdk:"dhcpv6_stop"`                    // DHCPv6 pool end
}

func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_network"
}

func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
		MarkdownDescription: "A UniFi Network LAN/VLAN or guest network (`networkconf`). Models the full configurable " +
			"surface (DHCP server + options, IPv6/DHCPv6, IGMP/mDNS, NAT/internet access). For WAN uplinks use `unifi_network_wan`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "Network name."},
			"purpose": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Network purpose: `corporate` (a routed LAN/VLAN), `guest`, or `vlan-only` (L2 VLAN, no gateway). Defaults to `corporate` on the controller when unset. Immutable — changing it forces replacement.",
				Validators:          []validator.String{stringvalidator.OneOf("corporate", "guest", "vlan-only")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_group":      schema.StringAttribute{Computed: true, MarkdownDescription: "Controller network group (e.g. `LAN`)."},
			"setting_preference": schema.StringAttribute{Computed: true, MarkdownDescription: "Controller auto/manual settings toggle (read-only)."},
			"gateway_type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway type (read-only)."},
			"enabled":            optBool("Whether the network is enabled."),

			"vlan_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "802.1Q VLAN ID (2–4094). Omit for an untagged network.",
				Validators:          []validator.Int64{int64validator.Between(2, 4094)},
			},
			"subnet":                  optStr("Gateway IP + prefix in CIDR, e.g. `192.0.2.1/24`. Not used by `vlan-only` networks."),
			"domain_name":             optStr("DHCP domain name."),
			"nat_enabled":             optBool("Whether the network is NATed (`is_nat`)."),
			"internet_access_enabled": optBool("Whether clients on this network may reach the internet."),
			"igmp_snooping":           optBool("Whether IGMP snooping is enabled."),
			"upnp_enabled":            optBool("Whether UPnP is enabled on this LAN."),
			"dhcp_guard_enabled":      optBool("Whether rogue-DHCP guarding is enabled."),
			"auto_scale_enabled":      optBool("Whether the subnet auto-scales."),
			"lte_failover_enabled":    optBool("Whether this is the LTE failover LAN."),

			"dhcp_enabled":    optBool("Whether the built-in DHCP server is enabled."),
			"dhcp_start":      optStr("First address of the DHCP pool. Computed from the controller when unset."),
			"dhcp_stop":       optStr("Last address of the DHCP pool. Computed from the controller when unset."),
			"dhcp_lease_time": optInt("DHCP lease time in seconds."),
			"dhcp_dns": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Up to four DNS servers handed out by DHCP. Setting any enables the DHCP DNS override.",
				Validators:          []validator.List{listvalidator.SizeAtMost(4)},
			},
			"dhcp_gateway_enabled":     optBool("Whether DHCP hands out a custom gateway (`dhcp_gateway`)."),
			"dhcp_gateway":             optStr("Custom DHCP gateway address (requires `dhcp_gateway_enabled`)."),
			"dhcp_ntp_enabled":         optBool("Whether DHCP hands out NTP servers."),
			"dhcp_ntp_server_1":        optStr("First DHCP NTP server."),
			"dhcp_ntp_server_2":        optStr("Second DHCP NTP server."),
			"dhcp_wins_enabled":        optBool("Whether DHCP hands out WINS servers."),
			"dhcp_wins_server_1":       optStr("First DHCP WINS server."),
			"dhcp_wins_server_2":       optStr("Second DHCP WINS server."),
			"dhcp_tftp_server":         optStr("DHCP TFTP server (option 66)."),
			"dhcp_boot_enabled":        optBool("Whether DHCP network-boot options are handed out."),
			"dhcp_boot_server":         optStr("DHCP boot server (next-server)."),
			"dhcp_boot_filename":       optStr("DHCP boot filename (option 67)."),
			"dhcp_unifi_controller":    optStr("DHCP option 43 UniFi controller address."),
			"dhcp_conflict_checking":   optBool("Whether the DHCP server probes for IP conflicts before leasing."),
			"dhcp_time_offset_enabled": optBool("Whether the DHCP time-offset option is handed out."),
			"dhcp_wpad_url":            optStr("WPAD proxy auto-config URL handed out by DHCP."),
			"dhcp_relay_enabled":       optBool("Whether DHCP relay is used instead of the built-in server."),

			"ipv6_interface_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "IPv6 mode: `none`, `pd` (prefix delegation), or `static`.",
				Validators:          []validator.String{stringvalidator.OneOf("none", "pd", "static")},
			},
			"ipv6_ra_enabled":                optBool("Whether IPv6 router advertisements are enabled."),
			"ipv6_ra_priority":               optStr("Router-advertisement priority (`high`/`medium`/`low`)."),
			"ipv6_ra_preferred_lifetime":     optInt("RA preferred lifetime in seconds."),
			"ipv6_pd_start":                  optStr("IPv6 prefix-delegation pool start."),
			"ipv6_pd_stop":                   optStr("IPv6 prefix-delegation pool end."),
			"ipv6_client_address_assignment": optStr("IPv6 client address assignment mode (e.g. `slaac`)."),
			"dhcpv6_dns_auto":                optBool("Whether DHCPv6 DNS is auto."),
			"dhcpv6_lease_time":              optInt("DHCPv6 lease time in seconds."),
			"dhcpv6_start":                   optStr("DHCPv6 pool start."),
			"dhcpv6_stop":                    optStr("DHCPv6 pool end."),
		},
	}
}

func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	conf := plan.toNetworkConf(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.Networks().Create(ctx, conf)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create network", err.Error())
		return
	}
	plan.fromNetworkConf(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkResourceModel
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
		resp.Diagnostics.AddError("Failed to read network", err.Error())
		return
	}
	state.fromNetworkConf(conf)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	conf := plan.toNetworkConf(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Networks().Update(ctx, conf)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update network", err.Error())
		return
	}
	plan.fromNetworkConf(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.Networks().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete network", err.Error())
	}
}

func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *networkResourceModel) toNetworkConf(ctx context.Context, diags *diag.Diagnostics) *wire.Networkconf {
	conf := &wire.Networkconf{
		UnderscoreId:          strPtrOrNil(m.ID),
		Name:                  strPtrOrNil(m.Name),
		Purpose:               (*wire.NetworkconfPurpose)(strPtrOrNil(m.Purpose)),
		Enabled:               boolPtr(m.Enabled),
		IpSubnet:              strPtrOrNil(m.Subnet),
		DomainName:            strPtrOrNil(m.DomainName),
		IsNat:                 boolPtr(m.NATEnabled),
		InternetAccessEnabled: boolPtr(m.InternetAccessEnabled),
		IgmpSnooping:          boolPtr(m.IGMPSnooping),
		UpnpLanEnabled:        boolPtr(m.UPNPEnabled),
		DhcpguardEnabled:      boolPtr(m.DHCPGuardEnabled),
		AutoScaleEnabled:      boolPtr(m.AutoScaleEnabled),
		LteLanEnabled:         boolPtr(m.LTEFailoverEnabled),

		DhcpdEnabled:           boolPtr(m.DHCPEnabled),
		DhcpdStart:             strPtrOrNil(m.DHCPStart),
		DhcpdStop:              strPtrOrNil(m.DHCPStop),
		DhcpdLeasetime:         intPtr(m.DHCPLeaseTime),
		DhcpdGatewayEnabled:    boolPtr(m.DHCPGatewayEnabled),
		DhcpdGateway:           strPtrOrNil(m.DHCPGateway),
		DhcpdNtpEnabled:        boolPtr(m.DHCPNTPEnabled),
		DhcpdNtp1:              strPtrOrNil(m.DHCPNTPServer1),
		DhcpdNtp2:              strPtrOrNil(m.DHCPNTPServer2),
		DhcpdWinsEnabled:       boolPtr(m.DHCPWINSEnabled),
		DhcpdWins1:             strPtrOrNil(m.DHCPWINSServer1),
		DhcpdWins2:             strPtrOrNil(m.DHCPWINSServer2),
		DhcpdTftpServer:        strPtrOrNil(m.DHCPTFTPServer),
		DhcpdBootEnabled:       boolPtr(m.DHCPBootEnabled),
		DhcpdBootServer:        strPtrOrNil(m.DHCPBootServer),
		DhcpdBootFilename:      strPtrOrNil(m.DHCPBootFilename),
		DhcpdUnifiController:   strPtrOrNil(m.DHCPUnifiController),
		DhcpdConflictChecking:  boolPtr(m.DHCPConflictChecking),
		DhcpdTimeOffsetEnabled: boolPtr(m.DHCPTimeOffsetEnabled),
		DhcpdWpadUrl:           strPtrOrNil(m.DHCPWPADURL),
		DhcpRelayEnabled:       boolPtr(m.DHCPRelayEnabled),

		Ipv6InterfaceType:           (*wire.NetworkconfIpv6InterfaceType)(strPtrOrNil(m.IPv6InterfaceType)),
		Ipv6RaEnabled:               boolPtr(m.IPv6RAEnabled),
		Ipv6RaPriority:              (*wire.NetworkconfIpv6RaPriority)(strPtrOrNil(m.IPv6RAPriority)),
		Ipv6RaPreferredLifetime:     intPtr(m.IPv6RAPreferredLifetime),
		Ipv6PdStart:                 strPtrOrNil(m.IPv6PDStart),
		Ipv6PdStop:                  strPtrOrNil(m.IPv6PDStop),
		Ipv6ClientAddressAssignment: (*wire.NetworkconfIpv6ClientAddressAssignment)(strPtrOrNil(m.IPv6ClientAddressAssignment)),
		Dhcpdv6DnsAuto:              boolPtr(m.DHCPv6DNSAuto),
		Dhcpdv6Leasetime:            intPtr(m.DHCPv6LeaseTime),
		Dhcpdv6Start:                strPtrOrNil(m.DHCPv6Start),
		Dhcpdv6Stop:                 strPtrOrNil(m.DHCPv6Stop),
	}

	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		v := int(m.VLANID.ValueInt64())
		conf.Vlan = &v
		enabled := true
		conf.VlanEnabled = &enabled
	} else {
		disabled := false
		conf.VlanEnabled = &disabled
	}

	// Only touch the DHCP DNS slots when the list is configured. When set, write
	// every owned slot (freed ones as "") so shrinking the list clears the
	// trailing slot on GET-merge-PUT instead of leaving it stale.
	if !m.DHCPDNS.IsNull() && !m.DHCPDNS.IsUnknown() {
		dns := stringListToSlice(ctx, m.DHCPDNS, diags)
		enabled := len(dns) > 0
		conf.DhcpdDnsEnabled = &enabled
		conf.DhcpdDns1 = dnsSlotPtr(dns, 0)
		conf.DhcpdDns2 = dnsSlotPtr(dns, 1)
		conf.DhcpdDns3 = dnsSlotPtr(dns, 2)
		conf.DhcpdDns4 = dnsSlotPtr(dns, 3)
	}
	return conf
}

func (m *networkResourceModel) fromNetworkConf(conf *wire.Networkconf) {
	m.ID = types.StringValue(deref(conf.UnderscoreId))
	m.Name = types.StringValue(deref(conf.Name))
	m.Purpose = types.StringValue(string(derefEnum(conf.Purpose)))
	m.NetworkGroup = types.StringValue(deref(conf.Networkgroup))
	m.SettingPreference = types.StringValue(string(derefEnum(conf.SettingPreference)))
	m.GatewayType = types.StringValue(string(derefEnum(conf.GatewayType)))
	m.Enabled = boolToState(conf.Enabled)

	m.Subnet = types.StringValue(deref(conf.IpSubnet))
	m.DomainName = types.StringValue(deref(conf.DomainName))
	m.NATEnabled = boolToState(conf.IsNat)
	m.InternetAccessEnabled = boolToState(conf.InternetAccessEnabled)
	m.IGMPSnooping = boolToState(conf.IgmpSnooping)
	m.UPNPEnabled = boolToState(conf.UpnpLanEnabled)
	m.DHCPGuardEnabled = boolToState(conf.DhcpguardEnabled)
	m.AutoScaleEnabled = boolToState(conf.AutoScaleEnabled)
	m.LTEFailoverEnabled = boolToState(conf.LteLanEnabled)

	m.DHCPEnabled = boolToState(conf.DhcpdEnabled)
	m.DHCPStart = types.StringValue(deref(conf.DhcpdStart))
	m.DHCPStop = types.StringValue(deref(conf.DhcpdStop))
	m.DHCPLeaseTime = intToState(conf.DhcpdLeasetime)
	m.DHCPGatewayEnabled = boolToState(conf.DhcpdGatewayEnabled)
	m.DHCPGateway = types.StringValue(deref(conf.DhcpdGateway))
	m.DHCPNTPEnabled = boolToState(conf.DhcpdNtpEnabled)
	m.DHCPNTPServer1 = types.StringValue(deref(conf.DhcpdNtp1))
	m.DHCPNTPServer2 = types.StringValue(deref(conf.DhcpdNtp2))
	m.DHCPWINSEnabled = boolToState(conf.DhcpdWinsEnabled)
	m.DHCPWINSServer1 = types.StringValue(deref(conf.DhcpdWins1))
	m.DHCPWINSServer2 = types.StringValue(deref(conf.DhcpdWins2))
	m.DHCPTFTPServer = types.StringValue(deref(conf.DhcpdTftpServer))
	m.DHCPBootEnabled = boolToState(conf.DhcpdBootEnabled)
	m.DHCPBootServer = types.StringValue(deref(conf.DhcpdBootServer))
	m.DHCPBootFilename = types.StringValue(deref(conf.DhcpdBootFilename))
	m.DHCPUnifiController = types.StringValue(deref(conf.DhcpdUnifiController))
	m.DHCPConflictChecking = boolToState(conf.DhcpdConflictChecking)
	m.DHCPTimeOffsetEnabled = boolToState(conf.DhcpdTimeOffsetEnabled)
	m.DHCPWPADURL = types.StringValue(deref(conf.DhcpdWpadUrl))
	m.DHCPRelayEnabled = boolToState(conf.DhcpRelayEnabled)

	m.IPv6InterfaceType = types.StringValue(string(derefEnum(conf.Ipv6InterfaceType)))
	m.IPv6RAEnabled = boolToState(conf.Ipv6RaEnabled)
	m.IPv6RAPriority = types.StringValue(string(derefEnum(conf.Ipv6RaPriority)))
	m.IPv6RAPreferredLifetime = intToState(conf.Ipv6RaPreferredLifetime)
	m.IPv6PDStart = types.StringValue(deref(conf.Ipv6PdStart))
	m.IPv6PDStop = types.StringValue(deref(conf.Ipv6PdStop))
	m.IPv6ClientAddressAssignment = types.StringValue(string(derefEnum(conf.Ipv6ClientAddressAssignment)))
	m.DHCPv6DNSAuto = boolToState(conf.Dhcpdv6DnsAuto)
	m.DHCPv6LeaseTime = intToState(conf.Dhcpdv6Leasetime)
	m.DHCPv6Start = types.StringValue(deref(conf.Dhcpdv6Start))
	m.DHCPv6Stop = types.StringValue(deref(conf.Dhcpdv6Stop))

	if conf.VlanEnabled != nil && *conf.VlanEnabled && conf.Vlan != nil {
		m.VLANID = types.Int64Value(int64(*conf.Vlan))
	} else {
		m.VLANID = types.Int64Null()
	}

	m.DHCPDNS = sliceToStringList(dnsSlots(conf.DhcpdDns1, conf.DhcpdDns2, conf.DhcpdDns3, conf.DhcpdDns4))
}
