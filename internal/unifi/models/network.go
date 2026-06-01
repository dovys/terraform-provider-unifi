package models

// NetworkConf is a legacy `/rest/networkconf` document. One object type backs
// several Terraform resources, distinguished by Purpose: LAN/VLAN/guest
// (`unifi_network_network`) and WAN uplinks (`unifi_network_wan`,
// purpose=wan). Field names mirror the controller's wire shape exactly
// (confirmed against a live UCG Max). Pointers + omitempty give three-state
// semantics on GET-merge-PUT: nil = leave the controller's value, set = write
// it (including an explicit false/0). Fields not modeled here (e.g.
// ip_aliases, nat_outbound_ip_addresses, wan_dhcp_options) are carried through
// untouched by GET-merge-PUT.
type NetworkConf struct {
	ID                string `json:"_id,omitempty"`                // controller object ID (Mongo _id); the resource import key
	SiteID            string `json:"site_id,omitempty"`            // owning site UUID (controller-assigned)
	Name              string `json:"name"`                         // display name
	Purpose           string `json:"purpose,omitempty"`            // corporate | guest | vlan-only | wan — selects the LAN vs WAN field set
	NetworkGroup      string `json:"networkgroup,omitempty"`       // controller network group, e.g. "LAN" (computed)
	Enabled           *bool  `json:"enabled,omitempty"`            // whether the network/uplink is enabled
	SettingPreference string `json:"setting_preference,omitempty"` // auto | manual — controller UI toggle (read-only here)
	AttrNoDelete      *bool  `json:"attr_no_delete,omitempty"`     // controller-protected (can't be deleted); carried through
	AttrHiddenID      string `json:"attr_hidden_id,omitempty"`     // controller-internal stable key (e.g. "LAN"); carried through
	ExternalID        string `json:"external_id,omitempty"`        // external/system reference (computed)

	// LAN / VLAN (purpose corporate/guest/vlan-only).
	VLANEnabled           *bool  `json:"vlan_enabled,omitempty"`            // whether the network is 802.1Q tagged
	VLAN                  *int   `json:"vlan,omitempty"`                    // VLAN ID (2–4094) when vlan_enabled
	IPSubnet              string `json:"ip_subnet,omitempty"`               // gateway IP + prefix in CIDR, e.g. 192.0.2.1/24
	DomainName            string `json:"domain_name,omitempty"`             // DHCP domain name handed to clients
	IsNAT                 *bool  `json:"is_nat,omitempty"`                  // whether the subnet is NATed behind the gateway
	InternetAccessEnabled *bool  `json:"internet_access_enabled,omitempty"` // whether clients may reach the internet
	IGMPSnooping          *bool  `json:"igmp_snooping,omitempty"`           // IGMP snooping (multicast pruning)
	MDNSEnabled           *bool  `json:"mdns_enabled,omitempty"`            // mDNS/Bonjour reflection across VLANs
	UPNPLANEnabled        *bool  `json:"upnp_lan_enabled,omitempty"`        // UPnP IGD on this LAN
	DHCPGuardEnabled      *bool  `json:"dhcpguard_enabled,omitempty"`       // block rogue DHCP servers on this network
	AutoScaleEnabled      *bool  `json:"auto_scale_enabled,omitempty"`      // auto-grow the subnet as clients are added
	LTELANEnabled         *bool  `json:"lte_lan_enabled,omitempty"`         // designate this as the LTE-failover LAN
	GatewayType           string `json:"gateway_type,omitempty"`            // gateway type, e.g. "default" (read-only)

	// DHCP server.
	DHCPDEnabled           *bool  `json:"dhcpd_enabled,omitempty"`             // built-in DHCP server on/off
	DHCPDStart             string `json:"dhcpd_start,omitempty"`               // first address of the DHCP pool
	DHCPDStop              string `json:"dhcpd_stop,omitempty"`                // last address of the DHCP pool
	DHCPDLeaseTime         *int   `json:"dhcpd_leasetime,omitempty"`           // lease time in seconds
	DHCPDDNSEnabled        *bool  `json:"dhcpd_dns_enabled,omitempty"`         // override DNS handed out by DHCP (else gateway)
	DHCPDDNS1              string `json:"dhcpd_dns_1,omitempty"`               // DHCP DNS server 1 (maps to dhcp_dns[0])
	DHCPDDNS2              string `json:"dhcpd_dns_2,omitempty"`               // DHCP DNS server 2
	DHCPDDNS3              string `json:"dhcpd_dns_3,omitempty"`               // DHCP DNS server 3
	DHCPDDNS4              string `json:"dhcpd_dns_4,omitempty"`               // DHCP DNS server 4
	DHCPDGatewayEnabled    *bool  `json:"dhcpd_gateway_enabled,omitempty"`     // hand out a custom gateway (else this network's)
	DHCPDGateway           string `json:"dhcpd_gateway,omitempty"`             // custom DHCP gateway address
	DHCPDNTPEnabled        *bool  `json:"dhcpd_ntp_enabled,omitempty"`         // hand out NTP servers via DHCP
	DHCPDNTP1              string `json:"dhcpd_ntp_1,omitempty"`               // DHCP NTP server 1
	DHCPDNTP2              string `json:"dhcpd_ntp_2,omitempty"`               // DHCP NTP server 2
	DHCPDWINSEnabled       *bool  `json:"dhcpd_wins_enabled,omitempty"`        // hand out WINS servers via DHCP
	DHCPDWINS1             string `json:"dhcpd_wins_1,omitempty"`              // DHCP WINS server 1
	DHCPDWINS2             string `json:"dhcpd_wins_2,omitempty"`              // DHCP WINS server 2
	DHCPDTFTPServer        string `json:"dhcpd_tftp_server,omitempty"`         // DHCP option 66 (TFTP server name)
	DHCPDBootEnabled       *bool  `json:"dhcpd_boot_enabled,omitempty"`        // hand out network-boot options (PXE)
	DHCPDBootServer        string `json:"dhcpd_boot_server,omitempty"`         // PXE next-server address
	DHCPDBootFilename      string `json:"dhcpd_boot_filename,omitempty"`       // DHCP option 67 (boot file name)
	DHCPDUnifiController   string `json:"dhcpd_unifi_controller,omitempty"`    // DHCP option 43 UniFi controller inform address
	DHCPDConflictChecking  *bool  `json:"dhcpd_conflict_checking,omitempty"`   // ARP-probe for IP conflicts before leasing
	DHCPDTimeOffsetEnabled *bool  `json:"dhcpd_time_offset_enabled,omitempty"` // hand out the DHCP time-offset option
	DHCPDWPADURL           string `json:"dhcpd_wpad_url,omitempty"`            // WPAD proxy auto-config URL (option 252)
	DHCPRelayEnabled       *bool  `json:"dhcp_relay_enabled,omitempty"`        // relay DHCP to an external server instead of serving

	// IPv6 (LAN side).
	IPv6Enabled                 *bool  `json:"ipv6_enabled,omitempty"`                   // IPv6 on this network on/off
	IPv6InterfaceType           string `json:"ipv6_interface_type,omitempty"`            // none | pd (prefix delegation) | static
	IPv6ClientAddressAssignment string `json:"ipv6_client_address_assignment,omitempty"` // how clients get IPv6 (e.g. slaac)
	IPv6PDStart                 string `json:"ipv6_pd_start,omitempty"`                  // PD-derived pool start
	IPv6PDStop                  string `json:"ipv6_pd_stop,omitempty"`                   // PD-derived pool end
	IPv6RAEnabled               *bool  `json:"ipv6_ra_enabled,omitempty"`                // send IPv6 router advertisements
	IPv6RAPreferredLifetime     *int   `json:"ipv6_ra_preferred_lifetime,omitempty"`     // RA preferred lifetime (seconds)
	IPv6RAPriority              string `json:"ipv6_ra_priority,omitempty"`               // RA router priority: high | medium | low
	IPv6SettingPreference       string `json:"ipv6_setting_preference,omitempty"`        // auto | manual IPv6 toggle (read-only here)
	DHCPDv6DNSAuto              *bool  `json:"dhcpdv6_dns_auto,omitempty"`               // DHCPv6 DNS auto vs manual
	DHCPDv6LeaseTime            *int   `json:"dhcpdv6_leasetime,omitempty"`              // DHCPv6 lease time (seconds)
	DHCPDv6Start                string `json:"dhcpdv6_start,omitempty"`                  // DHCPv6 pool start
	DHCPDv6Stop                 string `json:"dhcpdv6_stop,omitempty"`                   // DHCPv6 pool end

	// WAN uplink (purpose=wan).
	WANType                 string                   `json:"wan_type,omitempty"`                   // dhcp | pppoe | static
	WANTypeV6               string                   `json:"wan_type_v6,omitempty"`                // IPv6 uplink type, e.g. disabled | dhcpv6 | static
	WANNetworkGroup         string                   `json:"wan_networkgroup,omitempty"`           // WAN | WAN2 — which uplink interface
	WANDNSPreference        string                   `json:"wan_dns_preference,omitempty"`         // auto | manual (manual reads wan_dns1/2)
	WANDNS1                 string                   `json:"wan_dns1,omitempty"`                   // manual IPv4 DNS 1
	WANDNS2                 string                   `json:"wan_dns2,omitempty"`                   // manual IPv4 DNS 2
	WANIPv6DNSPreference    string                   `json:"wan_ipv6_dns_preference,omitempty"`    // auto | manual IPv6 DNS
	WANIPv6DNS1             string                   `json:"wan_ipv6_dns1,omitempty"`              // manual IPv6 DNS 1
	WANIPv6DNS2             string                   `json:"wan_ipv6_dns2,omitempty"`              // manual IPv6 DNS 2
	WANUsername             string                   `json:"wan_username,omitempty"`               // PPPoE username
	XWANPassword            string                   `json:"x_wan_password,omitempty"`             // PPPoE password (x_ = controller-encrypted; not echoed on read)
	WANPPPoEUsernameEnabled *bool                    `json:"wan_pppoe_username_enabled,omitempty"` // controller flag: PPPoE username present
	WANPPPoEPasswordEnabled *bool                    `json:"wan_pppoe_password_enabled,omitempty"` // controller flag: PPPoE password present
	WANIP                   string                   `json:"wan_ip,omitempty"`                     // static WAN IP
	WANNetmask              string                   `json:"wan_netmask,omitempty"`                // static WAN netmask
	WANGateway              string                   `json:"wan_gateway,omitempty"`                // static WAN gateway
	WANVLANEnabled          *bool                    `json:"wan_vlan_enabled,omitempty"`           // whether the uplink is VLAN-tagged
	WANVLAN                 *int                     `json:"wan_vlan,omitempty"`                   // uplink VLAN tag
	WANLoadBalanceType      string                   `json:"wan_load_balance_type,omitempty"`      // weighted | failover-only (dual-WAN role)
	WANLoadBalanceWeight    *int                     `json:"wan_load_balance_weight,omitempty"`    // load-balance weight (0–100)
	WANFailoverPriority     *int                     `json:"wan_failover_priority,omitempty"`      // failover priority (lower wins)
	WANSmartQEnabled        *bool                    `json:"wan_smartq_enabled,omitempty"`         // smart-queue QoS on/off
	WANSmartQUpRate         *int                     `json:"wan_smartq_up_rate,omitempty"`         // smart-queue upload limit (kbit/s)
	WANSmartQDownRate       *int                     `json:"wan_smartq_down_rate,omitempty"`       // smart-queue download limit (kbit/s)
	WANProviderCapabilities *WANProviderCapabilities `json:"wan_provider_capabilities,omitempty"`  // advertised ISP up/down bandwidth
	IGMPProxyUpstream       *bool                    `json:"igmp_proxy_upstream,omitempty"`        // this uplink is the IGMP-proxy upstream
	IGMPProxyFor            string                   `json:"igmp_proxy_for,omitempty"`             // IGMP-proxy downstream network
	MACOverrideEnabled      *bool                    `json:"mac_override_enabled,omitempty"`       // whether the WAN MAC is cloned/overridden
	MACOverride             string                   `json:"mac_override,omitempty"`               // cloned MAC address
	ReportWANEvent          *bool                    `json:"report_wan_event,omitempty"`           // report WAN up/down events
	RoutingTableID          *int                     `json:"routing_table_id,omitempty"`           // policy-routing table ID (controller-assigned)
	SingleNetworkLAN        string                   `json:"single_network_lan,omitempty"`         // LAN this uplink is bound to (read-only)
	IPv6WANDelegationType   string                   `json:"ipv6_wan_delegation_type,omitempty"`   // IPv6 PD delegation type: none | dhcpv6 | …
	WANDHCPv6PDSizeAuto     *bool                    `json:"wan_dhcpv6_pd_size_auto,omitempty"`    // auto-size the delegated IPv6 prefix
	WANDHCPv6PDSize         *int                     `json:"wan_dhcpv6_pd_size,omitempty"`         // delegated IPv6 prefix size
}

// WANProviderCapabilities is the advertised ISP bandwidth on a WAN uplink.
// Pointers give three-state semantics so setting only one direction doesn't
// read the other back as an explicit 0 (which would churn the plan).
type WANProviderCapabilities struct {
	UploadKilobitsPerSecond   *int `json:"upload_kilobits_per_second,omitempty"`   // advertised upload (kbit/s)
	DownloadKilobitsPerSecond *int `json:"download_kilobits_per_second,omitempty"` // advertised download (kbit/s)
}
