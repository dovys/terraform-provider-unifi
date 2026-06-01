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
	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

var (
	_ resource.Resource                = (*networkResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkResource)(nil)
	_ resource.ResourceWithImportState = (*networkResource)(nil)
)

func NewNetworkResource() resource.Resource { return &networkResource{} }

type networkResource struct {
	client unifi.Client
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
	MDNSEnabled           types.Bool   `tfsdk:"mdns_enabled"`            // mDNS/Bonjour reflection
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
				Default:             stringdefault.StaticString("corporate"),
				MarkdownDescription: "Network purpose: `corporate` (a routed LAN/VLAN), `guest`, or `vlan-only` (L2 VLAN, no gateway). Immutable — changing it forces replacement.",
				Validators:          []validator.String{stringvalidator.OneOf("corporate", "guest", "vlan-only")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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
			"mdns_enabled":            optBool("Whether multicast DNS (mDNS) reflection is enabled."),
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
		r.client = c
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
	created, err := r.client.CreateNetwork(ctx, conf)
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
	conf, err := r.client.GetNetwork(ctx, state.ID.ValueString())
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
	updated, err := r.client.UpdateNetwork(ctx, conf)
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
	if err := r.client.DeleteNetwork(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete network", err.Error())
	}
}

func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *networkResourceModel) toNetworkConf(ctx context.Context, diags *diag.Diagnostics) *models.NetworkConf {
	conf := &models.NetworkConf{
		ID:                    m.ID.ValueString(),
		Name:                  m.Name.ValueString(),
		Purpose:               strOrEmpty(m.Purpose),
		Enabled:               boolPtr(m.Enabled),
		IPSubnet:              strOrEmpty(m.Subnet),
		DomainName:            strOrEmpty(m.DomainName),
		IsNAT:                 boolPtr(m.NATEnabled),
		InternetAccessEnabled: boolPtr(m.InternetAccessEnabled),
		IGMPSnooping:          boolPtr(m.IGMPSnooping),
		MDNSEnabled:           boolPtr(m.MDNSEnabled),
		UPNPLANEnabled:        boolPtr(m.UPNPEnabled),
		DHCPGuardEnabled:      boolPtr(m.DHCPGuardEnabled),
		AutoScaleEnabled:      boolPtr(m.AutoScaleEnabled),
		LTELANEnabled:         boolPtr(m.LTEFailoverEnabled),

		DHCPDEnabled:           boolPtr(m.DHCPEnabled),
		DHCPDStart:             strOrEmpty(m.DHCPStart),
		DHCPDStop:              strOrEmpty(m.DHCPStop),
		DHCPDLeaseTime:         intPtr(m.DHCPLeaseTime),
		DHCPDGatewayEnabled:    boolPtr(m.DHCPGatewayEnabled),
		DHCPDGateway:           strOrEmpty(m.DHCPGateway),
		DHCPDNTPEnabled:        boolPtr(m.DHCPNTPEnabled),
		DHCPDNTP1:              strOrEmpty(m.DHCPNTPServer1),
		DHCPDNTP2:              strOrEmpty(m.DHCPNTPServer2),
		DHCPDWINSEnabled:       boolPtr(m.DHCPWINSEnabled),
		DHCPDWINS1:             strOrEmpty(m.DHCPWINSServer1),
		DHCPDWINS2:             strOrEmpty(m.DHCPWINSServer2),
		DHCPDTFTPServer:        strOrEmpty(m.DHCPTFTPServer),
		DHCPDBootEnabled:       boolPtr(m.DHCPBootEnabled),
		DHCPDBootServer:        strOrEmpty(m.DHCPBootServer),
		DHCPDBootFilename:      strOrEmpty(m.DHCPBootFilename),
		DHCPDUnifiController:   strOrEmpty(m.DHCPUnifiController),
		DHCPDConflictChecking:  boolPtr(m.DHCPConflictChecking),
		DHCPDTimeOffsetEnabled: boolPtr(m.DHCPTimeOffsetEnabled),
		DHCPDWPADURL:           strOrEmpty(m.DHCPWPADURL),
		DHCPRelayEnabled:       boolPtr(m.DHCPRelayEnabled),

		IPv6InterfaceType:           strOrEmpty(m.IPv6InterfaceType),
		IPv6RAEnabled:               boolPtr(m.IPv6RAEnabled),
		IPv6RAPriority:              strOrEmpty(m.IPv6RAPriority),
		IPv6RAPreferredLifetime:     intPtr(m.IPv6RAPreferredLifetime),
		IPv6PDStart:                 strOrEmpty(m.IPv6PDStart),
		IPv6PDStop:                  strOrEmpty(m.IPv6PDStop),
		IPv6ClientAddressAssignment: strOrEmpty(m.IPv6ClientAddressAssignment),
		DHCPDv6DNSAuto:              boolPtr(m.DHCPv6DNSAuto),
		DHCPDv6LeaseTime:            intPtr(m.DHCPv6LeaseTime),
		DHCPDv6Start:                strOrEmpty(m.DHCPv6Start),
		DHCPDv6Stop:                 strOrEmpty(m.DHCPv6Stop),
	}

	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		v := int(m.VLANID.ValueInt64())
		conf.VLAN = &v
		enabled := true
		conf.VLANEnabled = &enabled
	} else {
		disabled := false
		conf.VLANEnabled = &disabled
	}

	dns := stringListToSlice(ctx, m.DHCPDNS, diags)
	enabled := len(dns) > 0
	conf.DHCPDDNSEnabled = &enabled
	for i, s := range dns {
		switch i {
		case 0:
			conf.DHCPDDNS1 = s
		case 1:
			conf.DHCPDDNS2 = s
		case 2:
			conf.DHCPDDNS3 = s
		case 3:
			conf.DHCPDDNS4 = s
		}
	}
	return conf
}

func (m *networkResourceModel) fromNetworkConf(conf *models.NetworkConf) {
	m.ID = types.StringValue(conf.ID)
	m.Name = types.StringValue(conf.Name)
	m.Purpose = types.StringValue(conf.Purpose)
	m.NetworkGroup = types.StringValue(conf.NetworkGroup)
	m.SettingPreference = types.StringValue(conf.SettingPreference)
	m.GatewayType = types.StringValue(conf.GatewayType)
	m.Enabled = boolToState(conf.Enabled)

	m.Subnet = types.StringValue(conf.IPSubnet)
	m.DomainName = types.StringValue(conf.DomainName)
	m.NATEnabled = boolToState(conf.IsNAT)
	m.InternetAccessEnabled = boolToState(conf.InternetAccessEnabled)
	m.IGMPSnooping = boolToState(conf.IGMPSnooping)
	m.MDNSEnabled = boolToState(conf.MDNSEnabled)
	m.UPNPEnabled = boolToState(conf.UPNPLANEnabled)
	m.DHCPGuardEnabled = boolToState(conf.DHCPGuardEnabled)
	m.AutoScaleEnabled = boolToState(conf.AutoScaleEnabled)
	m.LTEFailoverEnabled = boolToState(conf.LTELANEnabled)

	m.DHCPEnabled = boolToState(conf.DHCPDEnabled)
	m.DHCPStart = types.StringValue(conf.DHCPDStart)
	m.DHCPStop = types.StringValue(conf.DHCPDStop)
	m.DHCPLeaseTime = intToState(conf.DHCPDLeaseTime)
	m.DHCPGatewayEnabled = boolToState(conf.DHCPDGatewayEnabled)
	m.DHCPGateway = types.StringValue(conf.DHCPDGateway)
	m.DHCPNTPEnabled = boolToState(conf.DHCPDNTPEnabled)
	m.DHCPNTPServer1 = types.StringValue(conf.DHCPDNTP1)
	m.DHCPNTPServer2 = types.StringValue(conf.DHCPDNTP2)
	m.DHCPWINSEnabled = boolToState(conf.DHCPDWINSEnabled)
	m.DHCPWINSServer1 = types.StringValue(conf.DHCPDWINS1)
	m.DHCPWINSServer2 = types.StringValue(conf.DHCPDWINS2)
	m.DHCPTFTPServer = types.StringValue(conf.DHCPDTFTPServer)
	m.DHCPBootEnabled = boolToState(conf.DHCPDBootEnabled)
	m.DHCPBootServer = types.StringValue(conf.DHCPDBootServer)
	m.DHCPBootFilename = types.StringValue(conf.DHCPDBootFilename)
	m.DHCPUnifiController = types.StringValue(conf.DHCPDUnifiController)
	m.DHCPConflictChecking = boolToState(conf.DHCPDConflictChecking)
	m.DHCPTimeOffsetEnabled = boolToState(conf.DHCPDTimeOffsetEnabled)
	m.DHCPWPADURL = types.StringValue(conf.DHCPDWPADURL)
	m.DHCPRelayEnabled = boolToState(conf.DHCPRelayEnabled)

	m.IPv6InterfaceType = types.StringValue(conf.IPv6InterfaceType)
	m.IPv6RAEnabled = boolToState(conf.IPv6RAEnabled)
	m.IPv6RAPriority = types.StringValue(conf.IPv6RAPriority)
	m.IPv6RAPreferredLifetime = intToState(conf.IPv6RAPreferredLifetime)
	m.IPv6PDStart = types.StringValue(conf.IPv6PDStart)
	m.IPv6PDStop = types.StringValue(conf.IPv6PDStop)
	m.IPv6ClientAddressAssignment = types.StringValue(conf.IPv6ClientAddressAssignment)
	m.DHCPv6DNSAuto = boolToState(conf.DHCPDv6DNSAuto)
	m.DHCPv6LeaseTime = intToState(conf.DHCPDv6LeaseTime)
	m.DHCPv6Start = types.StringValue(conf.DHCPDv6Start)
	m.DHCPv6Stop = types.StringValue(conf.DHCPDv6Stop)

	if conf.VLANEnabled != nil && *conf.VLANEnabled && conf.VLAN != nil {
		m.VLANID = types.Int64Value(int64(*conf.VLAN))
	} else {
		m.VLANID = types.Int64Null()
	}

	var dns []string
	for _, s := range []string{conf.DHCPDDNS1, conf.DHCPDDNS2, conf.DHCPDDNS3, conf.DHCPDDNS4} {
		if s != "" {
			dns = append(dns, s)
		}
	}
	m.DHCPDNS = sliceToStringList(dns)
}
