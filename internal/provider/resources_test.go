package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// sp returns a pointer to s — for building wire fixtures with *string slots.
func sp(s string) *string { return &s }

// derefStr returns the pointed-to string, or "" for a nil pointer.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestProviderSchemasValidate drives the full GetProviderSchema RPC, which runs
// the framework's schema validation across the provider, every resource, and
// every data source — catching invalid combinations (Required+Computed, a
// default on a required attribute, bad nested types) that the per-Schema unit
// tests don't. It also asserts the per-phase registration set.
func TestProviderSchemasValidate(t *testing.T) {
	ctx := context.Background()
	srv := providerserver.NewProtocol6(New("test")())()
	resp, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("schema error diagnostic: %s — %s", d.Summary, d.Detail)
		}
	}

	for _, name := range []string{"unifi_network_network", "unifi_network_wan", "unifi_network_dns_record", "unifi_network_wlan", "unifi_network_ap_group", "unifi_network_firewall_group", "unifi_network_firewall_rule", "unifi_network_dhcp_reservation", "unifi_network_port_profile", "unifi_network_device", "unifi_network_user_group", "unifi_network_radius_profile", "unifi_network_radius_account", "unifi_network_static_route", "unifi_network_port_forward", "unifi_network_dynamic_dns", "unifi_network_wlan_group", "unifi_network_traffic_rule", "unifi_network_traffic_route", "unifi_network_settings"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %q not registered", name)
		}
	}
	for _, name := range []string{"unifi_network_network", "unifi_network_networks", "unifi_network_site", "unifi_network_application_info", "unifi_network_device"} {
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %q not registered", name)
		}
	}
}

func TestNetworkConf_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Config → wire: a VLAN with DHCP-DNS overrides.
	m := &networkResourceModel{
		ID:                types.StringNull(),
		Name:              types.StringValue("iot"),
		Purpose:           types.StringValue("corporate"),
		VLANID:            types.Int64Value(30),
		Subnet:            types.StringValue("198.51.100.1/24"),
		NetworkGroup:      types.StringNull(),
		DHCPEnabled:       types.BoolValue(true),
		DHCPStart:         types.StringNull(),
		DHCPStop:          types.StringNull(),
		DHCPDNS:           sliceToStringList([]string{"1.1.1.1", "8.8.8.8"}),
		DomainName:        types.StringNull(),
		IPv6InterfaceType: types.StringNull(),
		IPv6RAEnabled:     types.BoolNull(),
	}
	var diags diag.Diagnostics
	conf := m.toNetworkConf(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toNetworkConf diags: %v", diags)
	}
	if conf.Vlan == nil || *conf.Vlan != 30 || conf.VlanEnabled == nil || !*conf.VlanEnabled {
		t.Errorf("VLAN mapping wrong: VLAN=%v Enabled=%v", conf.Vlan, conf.VlanEnabled)
	}
	if derefStr(conf.DhcpdDns1) != "1.1.1.1" || derefStr(conf.DhcpdDns2) != "8.8.8.8" {
		t.Errorf("dhcp dns mapping wrong: %v %v", conf.DhcpdDns1, conf.DhcpdDns2)
	}
	if conf.DhcpdDnsEnabled == nil || !*conf.DhcpdDnsEnabled {
		t.Errorf("DhcpdDnsEnabled should be true when servers are set")
	}

	// No VLAN ID → vlan disabled explicitly (not left nil).
	m.VLANID = types.Int64Null()
	conf = m.toNetworkConf(ctx, &diags)
	if conf.Vlan != nil || conf.VlanEnabled == nil || *conf.VlanEnabled {
		t.Errorf("untagged network: VLAN=%v Enabled=%v, want nil/false", conf.Vlan, conf.VlanEnabled)
	}

	// Wire → state.
	on := true
	vid := 40
	guestPurpose := wire.NetworkconfPurpose("guest")
	var out networkResourceModel
	out.fromNetworkConf(&wire.Networkconf{
		UnderscoreId: sp("net9"), Name: sp("guests"), Purpose: &guestPurpose, Networkgroup: sp("LAN"),
		VlanEnabled: &on, Vlan: &vid, DhcpdDns1: sp("9.9.9.9"),
	})
	if out.ID.ValueString() != "net9" || out.VLANID.ValueInt64() != 40 || out.NetworkGroup.ValueString() != "LAN" {
		t.Errorf("fromNetworkConf wrong: %+v", out)
	}
	var dns []string
	out.DHCPDNS.ElementsAs(ctx, &dns, false)
	if len(dns) != 1 || dns[0] != "9.9.9.9" {
		t.Errorf("dhcp_dns round-trip wrong: %v", dns)
	}
}

func TestWANConf_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &wanResourceModel{
		ID:               types.StringNull(),
		Name:             types.StringValue("uplink"),
		WANType:          types.StringValue("dhcp"),
		NetworkGroup:     types.StringValue("WAN"),
		Enabled:          types.BoolValue(true),
		DNSServers:       sliceToStringList([]string{"1.1.1.1", "8.8.8.8"}),
		LoadBalanceType:  types.StringNull(),
		UploadKilobits:   types.Int64Value(1000),
		DownloadKilobits: types.Int64Value(50000),
	}
	conf := m.toNetworkConf(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toNetworkConf diags: %v", diags)
	}
	if string(derefEnum(conf.Purpose)) != "wan" {
		t.Errorf("purpose = %q, want wan", string(derefEnum(conf.Purpose)))
	}
	if string(derefEnum(conf.WanDnsPreference)) != "manual" || derefStr(conf.WanDns1) != "1.1.1.1" || derefStr(conf.WanDns2) != "8.8.8.8" {
		t.Errorf("manual DNS mapping wrong: pref=%q dns1=%v dns2=%v", string(derefEnum(conf.WanDnsPreference)), conf.WanDns1, conf.WanDns2)
	}
	if conf.WanProviderCapabilities == nil || conf.WanProviderCapabilities.UploadKilobitsPerSecond == nil ||
		*conf.WanProviderCapabilities.UploadKilobitsPerSecond != 1000 {
		t.Errorf("provider caps wrong: %+v", conf.WanProviderCapabilities)
	}

	// Narrowing the list to one server must explicitly clear the freed slot
	// (non-nil &"") so the sparse PUT overwrites the stale wan_dns2 — omitting it
	// would leave the controller's server-side merge holding the extra server.
	m.DNSServers = sliceToStringList([]string{"1.1.1.1"})
	conf = m.toNetworkConf(ctx, &diags)
	if derefStr(conf.WanDns1) != "1.1.1.1" || conf.WanDns2 == nil || *conf.WanDns2 != "" {
		t.Errorf("narrowed DNS must clear slot 2: dns1=%v dns2=%v", conf.WanDns1, conf.WanDns2)
	}

	// A null list (unconfigured) is left untouched — the controller keeps its
	// own preference rather than being forced to auto.
	m.DNSServers = sliceToStringList(nil)
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WanDnsPreference != nil || conf.WanDns1 != nil {
		t.Errorf("null DNS must not touch the slots: pref=%v dns1=%v", conf.WanDnsPreference, conf.WanDns1)
	}

	// Only upload set: download must round-trip back to null (not 0), or the
	// plan churns forever.
	m.UploadKilobits = types.Int64Value(1000)
	m.DownloadKilobits = types.Int64Null()
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WanProviderCapabilities == nil || conf.WanProviderCapabilities.DownloadKilobitsPerSecond != nil {
		t.Errorf("unset download must stay nil on the wire: %+v", conf.WanProviderCapabilities)
	}
	var out wanResourceModel
	out.fromNetworkConf(conf)
	if !out.DownloadKilobits.IsNull() {
		t.Errorf("download_kilobits should read back null when unset, got %v", out.DownloadKilobits)
	}
	if out.UploadKilobits.ValueInt64() != 1000 {
		t.Errorf("upload_kilobits should round-trip to 1000, got %v", out.UploadKilobits)
	}
}

// TestWANConf_OmitsEmptyStaticFields proves the overlay omits wan_ip / netmask /
// gateway / igmp_proxy_for when unset rather than sending "". The controller
// validates these and rejects an empty string (api.err.InvalidValue) on a
// non-static WAN, so a sparse PUT must leave them out entirely.
func TestWANConf_OmitsEmptyStaticFields(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// DHCP WAN, static-IP fields unset: must be omitted from the wire.
	dhcp := (&wanResourceModel{
		Name:         types.StringValue("uplink"),
		WANType:      types.StringValue("dhcp"),
		NetworkGroup: types.StringValue("WAN"),
		Enabled:      types.BoolValue(true),
		IP:           types.StringNull(),
		Netmask:      types.StringNull(),
		Gateway:      types.StringNull(),
		IGMPProxyFor: types.StringNull(),
	}).toNetworkConf(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toNetworkConf diags: %v", diags)
	}
	if dhcp.WanIp != nil || dhcp.WanNetmask != nil || dhcp.WanGateway != nil || dhcp.IgmpProxyFor != nil {
		t.Errorf("static-IP / igmp fields must be omitted on a DHCP WAN, got ip=%v mask=%v gw=%v igmp=%v",
			dhcp.WanIp, dhcp.WanNetmask, dhcp.WanGateway, dhcp.IgmpProxyFor)
	}

	// Static WAN with real values: the fields must be sent.
	static := (&wanResourceModel{
		Name:         types.StringValue("uplink"),
		WANType:      types.StringValue("static"),
		NetworkGroup: types.StringValue("WAN"),
		Enabled:      types.BoolValue(true),
		IP:           types.StringValue("203.0.113.2"),
		Netmask:      types.StringValue("255.255.255.0"),
		Gateway:      types.StringValue("203.0.113.1"),
	}).toNetworkConf(ctx, &diags)
	if derefStr(static.WanIp) != "203.0.113.2" || derefStr(static.WanNetmask) != "255.255.255.0" || derefStr(static.WanGateway) != "203.0.113.1" {
		t.Errorf("static WAN must send ip/netmask/gateway, got ip=%v mask=%v gw=%v", static.WanIp, static.WanNetmask, static.WanGateway)
	}
}

func TestDNSRecord_RoundTrip(t *testing.T) {
	m := &dnsRecordResourceModel{
		ID:       types.StringNull(),
		Name:     types.StringValue("host.example.com"),
		Type:     types.StringValue("A"),
		Value:    types.StringValue("192.0.2.5"),
		Enabled:  types.BoolValue(true),
		TTL:      types.Int64Null(),
		Port:     types.Int64Null(),
		Priority: types.Int64Null(),
		Weight:   types.Int64Null(),
	}
	rec := m.toRecord()
	if rec.Key != "host.example.com" || derefEnum(rec.RecordType) != "A" || rec.Value != "192.0.2.5" {
		t.Errorf("toRecord wrong: %+v", rec)
	}
	if rec.Enabled == nil || !*rec.Enabled {
		t.Errorf("enabled should be &true")
	}
	if rec.Ttl != nil || rec.Port != nil {
		t.Errorf("unset int optionals must be nil: ttl=%v port=%v", rec.Ttl, rec.Port)
	}

	var out dnsRecordResourceModel
	ttl := 300
	cname := wire.StaticDnsRecordType("CNAME")
	out.fromRecord(&wire.StaticDns{UnderscoreId: sp("d1"), Key: "k", RecordType: &cname, Value: "t", Enabled: boolPtrLit(false), Ttl: &ttl})
	if out.ID.ValueString() != "d1" || out.Type.ValueString() != "CNAME" || out.TTL.ValueInt64() != 300 {
		t.Errorf("fromRecord wrong: %+v", out)
	}
	if out.Enabled.ValueBool() {
		t.Errorf("enabled should round-trip as false")
	}
}

func TestConversionHelpers(t *testing.T) {
	if boolPtr(types.BoolNull()) != nil || boolPtr(types.BoolUnknown()) != nil {
		t.Error("boolPtr(null/unknown) should be nil")
	}
	if p := boolPtr(types.BoolValue(false)); p == nil || *p {
		t.Error("boolPtr(false) should be &false")
	}
	if intPtr(types.Int64Null()) != nil {
		t.Error("intPtr(null) should be nil")
	}
	if p := intPtr(types.Int64Value(0)); p == nil || *p != 0 {
		t.Error("intPtr(0) should be &0 (explicit zero, not omitted)")
	}
	if strOrEmpty(types.StringNull()) != "" {
		t.Error("strOrEmpty(null) should be empty")
	}
	if !strToStateOptional("").IsNull() {
		t.Error("strToStateOptional(\"\") should be null")
	}
	if !sliceToStringList(nil).IsNull() {
		t.Error("sliceToStringList(nil) should be null")
	}
	if sliceToStringList([]string{"a"}).IsNull() {
		t.Error("sliceToStringList non-empty should not be null")
	}

	// sliceToStringSet distinguishes nil (field absent → null) from a non-nil
	// empty slice (controller returned `[]` → empty set), so an attribute
	// configured as `[]` reads back as `[]` and survives the consistency check.
	if !sliceToStringSet(nil).IsNull() {
		t.Error("sliceToStringSet(nil) should be null")
	}
	emptySet := sliceToStringSet([]string{})
	if emptySet.IsNull() || len(emptySet.Elements()) != 0 {
		t.Errorf("sliceToStringSet([]) should be a non-null empty set, got %v", emptySet)
	}
	if sliceToStringSet([]string{"a"}).IsNull() {
		t.Error("sliceToStringSet non-empty should not be null")
	}
}

func boolPtrLit(b bool) *bool { return &b }

func TestWAN_PPPoEStaticAndSecrets(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// PPPoE: creds map to wan_username/x_wan_password and set the enable flags.
	m := &wanResourceModel{
		Name: types.StringValue("wan"), WANType: types.StringValue("pppoe"),
		PPPoEUsername: types.StringValue("user@isp"), PPPoEPassword: types.StringValue("s3cret"),
		MACOverride: types.StringValue("02:00:00:00:00:01"),
	}
	conf := m.toNetworkConf(ctx, &diags)
	if derefStr(conf.WanUsername) != "user@isp" || derefStr(conf.XWanPassword) != "s3cret" {
		t.Errorf("pppoe creds wrong: %q / %q", derefStr(conf.WanUsername), derefStr(conf.XWanPassword))
	}
	if conf.WanPppoeUsernameEnabled == nil || !*conf.WanPppoeUsernameEnabled ||
		conf.WanPppoePasswordEnabled == nil || !*conf.WanPppoePasswordEnabled {
		t.Errorf("pppoe enable flags should be true when creds set")
	}
	if derefStr(conf.MacOverride) != "02:00:00:00:00:01" || conf.MacOverrideEnabled == nil || !*conf.MacOverrideEnabled {
		t.Errorf("mac override should be enabled with value: %+v", conf.MacOverrideEnabled)
	}

	// Static uplink fields.
	m = &wanResourceModel{
		Name: types.StringValue("wan"), WANType: types.StringValue("static"),
		IP: types.StringValue("203.0.113.2"), Netmask: types.StringValue("255.255.255.0"),
		Gateway: types.StringValue("203.0.113.1"),
	}
	conf = m.toNetworkConf(ctx, &diags)
	if derefStr(conf.WanIp) != "203.0.113.2" || derefStr(conf.WanNetmask) != "255.255.255.0" || derefStr(conf.WanGateway) != "203.0.113.1" {
		t.Errorf("static WAN mapping wrong: %+v", conf)
	}

	// Write-only password: a read that doesn't echo it must keep the prior value.
	pppoeType := wire.NetworkconfWanType("pppoe")
	out := wanResourceModel{PPPoEPassword: types.StringValue("keep-me")}
	out.fromNetworkConf(&wire.Networkconf{Name: sp("wan"), WanType: &pppoeType, WanUsername: sp("u")})
	if out.PPPoEPassword.ValueString() != "keep-me" {
		t.Errorf("write-only password should be preserved on read, got %q", out.PPPoEPassword.ValueString())
	}
}

func TestWAN_PPPoEPasswordOmittedWhenUnset(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// Unset password (null): must be omitted from the wire conf so the sparse PUT
	// leaves the controller's stored secret rather than blanking it. The
	// username (read back) is still sent.
	m := &wanResourceModel{
		Name: types.StringValue("wan"), WANType: types.StringValue("pppoe"),
		PPPoEUsername: types.StringValue("user@isp"), // PPPoEPassword left null
	}
	conf := m.toNetworkConf(ctx, &diags)
	if conf.XWanPassword != nil {
		t.Errorf("x_wan_password should be omitted when unset, got %q", derefStr(conf.XWanPassword))
	}
	if conf.WanPppoePasswordEnabled != nil {
		t.Errorf("wan_pppoe_password_enabled should be omitted when password unset, got %v", *conf.WanPppoePasswordEnabled)
	}
	if derefStr(conf.WanUsername) != "user@isp" {
		t.Errorf("username should still be sent: %q", derefStr(conf.WanUsername))
	}

	// Explicit empty string is a deliberate clear: sent as "" with enabled=false.
	m.PPPoEPassword = types.StringValue("")
	conf = m.toNetworkConf(ctx, &diags)
	if conf.XWanPassword == nil || *conf.XWanPassword != "" {
		t.Errorf(`explicit empty password should be sent as "", got %v`, conf.XWanPassword)
	}
	if conf.WanPppoePasswordEnabled == nil || *conf.WanPppoePasswordEnabled {
		t.Errorf("explicit empty password should disable the flag")
	}

	// Set password: sent with the enable flag true.
	m.PPPoEPassword = types.StringValue("s3cret")
	conf = m.toNetworkConf(ctx, &diags)
	if derefStr(conf.XWanPassword) != "s3cret" || conf.WanPppoePasswordEnabled == nil || !*conf.WanPppoePasswordEnabled {
		t.Errorf("set password should be sent with the enable flag true")
	}
}

func TestNetwork_DHCPOptionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &networkResourceModel{
		Name: types.StringValue("lan"), Purpose: types.StringValue("corporate"),
		DHCPLeaseTime:  types.Int64Value(86400),
		DHCPNTPEnabled: types.BoolValue(true), DHCPNTPServer1: types.StringValue("192.0.2.1"),
		DHCPGuardEnabled: types.BoolValue(true),
	}
	conf := m.toNetworkConf(ctx, &diags)
	if conf.DhcpdLeasetime == nil || *conf.DhcpdLeasetime != 86400 {
		t.Errorf("lease time wrong: %v", conf.DhcpdLeasetime)
	}
	if conf.DhcpdNtpEnabled == nil || !*conf.DhcpdNtpEnabled || derefStr(conf.DhcpdNtp1) != "192.0.2.1" {
		t.Errorf("dhcp ntp wrong: %v / %q", conf.DhcpdNtpEnabled, derefStr(conf.DhcpdNtp1))
	}
	if conf.DhcpguardEnabled == nil || !*conf.DhcpguardEnabled {
		t.Errorf("guard flag wrong")
	}

	var out networkResourceModel
	out.fromNetworkConf(conf)
	if out.DHCPLeaseTime.ValueInt64() != 86400 || out.DHCPNTPServer1.ValueString() != "192.0.2.1" {
		t.Errorf("round-trip wrong: %+v", out)
	}
}

func TestWLANConf_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &wlanResourceModel{
		ID:               types.StringNull(),
		Name:             types.StringValue("Home"),
		Enabled:          types.BoolValue(true),
		NetworkID:        types.StringValue("net1"),
		Security:         types.StringValue("wpapsk"),
		WPAMode:          types.StringValue("wpa2"),
		Passphrase:       types.StringValue("hunter2hunter2"),
		PMFMode:          types.StringValue("optional"),
		Bands:            sliceToStringSet([]string{"2g", "5g"}),
		APGroupIDs:       sliceToStringSet([]string{"ag1"}),
		APGroupMode:      types.StringValue("all"),
		MACFilterEnabled: types.BoolValue(true),
		MACFilterPolicy:  types.StringValue("allow"),
		MACFilterList:    sliceToStringSet([]string{"aa:bb:cc:00:11:22"}),
		GroupRekey:       types.Int64Value(3600),
		RADIUSProfileID:  types.StringValue("rprof1"),
	}
	conf := m.toWLANConf(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toWLANConf diags: %v", diags)
	}
	if derefStr(conf.Name) != "Home" || string(derefEnum(conf.Security)) != "wpapsk" || derefStr(conf.NetworkconfId) != "net1" {
		t.Errorf("scalar mapping wrong: %+v", conf)
	}
	if derefStr(conf.XPassphrase) != "hunter2hunter2" {
		t.Errorf("passphrase mapping wrong: %q", derefStr(conf.XPassphrase))
	}
	if conf.Enabled == nil || !*conf.Enabled {
		t.Errorf("enabled should be &true")
	}
	if conf.WlanBands == nil || len(*conf.WlanBands) != 2 ||
		conf.ApGroupIds == nil || len(*conf.ApGroupIds) != 1 ||
		conf.MacFilterList == nil || len(*conf.MacFilterList) != 1 {
		t.Errorf("set mapping wrong: bands=%v aps=%v macs=%v", conf.WlanBands, conf.ApGroupIds, conf.MacFilterList)
	}
	if conf.GroupRekey == nil || *conf.GroupRekey != 3600 {
		t.Errorf("group_rekey mapping wrong: %v", conf.GroupRekey)
	}
	if derefStr(conf.RadiusprofileId) != "rprof1" {
		t.Errorf("radius_profile_id mapping wrong: %q", derefStr(conf.RadiusprofileId))
	}

	// Wire → state. The controller commonly echoes an empty x_passphrase on
	// read; the passphrase must stay whatever the config set rather than wiping.
	out := *m // carry the configured passphrase
	wpapsk := wire.WlanconfSecurity("wpapsk")
	wpa2 := wire.WlanconfWpaMode("wpa2")
	bands := []wire.WlanconfWlanBands{"2g", "5g"}
	out.fromWLANConf(&wire.Wlanconf{
		UnderscoreId: sp("wlan9"), Name: sp("Home"), Security: &wpapsk, WpaMode: &wpa2,
		XPassphrase: sp(""), NetworkconfId: sp("net1"), WlanBands: &bands, RadiusprofileId: sp("rprof1"),
	})
	if out.ID.ValueString() != "wlan9" {
		t.Errorf("id round-trip wrong: %q", out.ID.ValueString())
	}
	if out.RADIUSProfileID.ValueString() != "rprof1" {
		t.Errorf("radius_profile_id round-trip wrong: %q", out.RADIUSProfileID.ValueString())
	}
	if out.Passphrase.ValueString() != "hunter2hunter2" {
		t.Errorf("passphrase must be preserved when the controller echoes empty, got %q", out.Passphrase.ValueString())
	}

	// When the controller does return a passphrase, adopt it.
	out.fromWLANConf(&wire.Wlanconf{UnderscoreId: sp("wlan9"), Name: sp("Home"), XPassphrase: sp("fromserver1234")})
	if out.Passphrase.ValueString() != "fromserver1234" {
		t.Errorf("passphrase should adopt the controller value, got %q", out.Passphrase.ValueString())
	}

	// mac_filter_policy is empty when filtering is off — Read must null it (not
	// "") so an imported WLAN doesn't trip the allow/deny enum validator (HIL).
	if !out.MACFilterPolicy.IsNull() {
		t.Errorf("mac_filter_policy must be null when the controller returns none, got %q", out.MACFilterPolicy.ValueString())
	}
	allow := wire.WlanconfMacFilterPolicy("allow")
	out.fromWLANConf(&wire.Wlanconf{UnderscoreId: sp("wlan9"), Name: sp("Home"), MacFilterPolicy: &allow})
	if out.MACFilterPolicy.ValueString() != "allow" {
		t.Errorf("mac_filter_policy should adopt the controller value, got %q", out.MACFilterPolicy.ValueString())
	}
}

func TestAPGroup_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// A null device_macs must serialize as an explicit empty array (the field is
	// non-omitempty), so clearing a group's members actually empties it.
	m := &apGroupResourceModel{
		ID:         types.StringNull(),
		Name:       types.StringValue("Upstairs"),
		DeviceMACs: types.SetNull(types.StringType),
	}
	g := m.toAPGroup(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toAPGroup diags: %v", diags)
	}
	if g.Name != "Upstairs" || g.DeviceMacs == nil || len(g.DeviceMacs) != 0 {
		t.Errorf("null device_macs must become an empty (non-nil) slice: %+v", g.DeviceMacs)
	}

	m.DeviceMACs = sliceToStringSet([]string{"aa:bb:cc:00:11:22", "de:ad:be:ef:00:01"})
	g = m.toAPGroup(ctx, &diags)
	if len(g.DeviceMacs) != 2 {
		t.Errorf("device_macs mapping wrong: %v", g.DeviceMacs)
	}

	var out apGroupResourceModel
	out.fromAPGroup(&wire.Apgroups{UnderscoreId: sp("ag5"), Name: "All APs", DeviceMacs: []string{"aa:bb:cc:00:11:22"}})
	if out.ID.ValueString() != "ag5" || out.Name.ValueString() != "All APs" {
		t.Errorf("fromAPGroup wrong: %+v", out)
	}
	if out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should read back false when the controller omits it, got true")
	}
	var macs []string
	out.DeviceMACs.ElementsAs(ctx, &macs, false)
	if len(macs) != 1 || macs[0] != "aa:bb:cc:00:11:22" {
		t.Errorf("device_macs round-trip wrong: %v", macs)
	}

	// The protected default group reads attr_no_delete = true (destroy → no-op).
	protected := true
	out.fromAPGroup(&wire.Apgroups{UnderscoreId: sp("default"), Name: "All APs", AttrNoDelete: &protected})
	if !out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should round-trip true for the protected default group")
	}
}

func TestFirewallGroup_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// A null members set serializes as an explicit empty array (non-omitempty).
	m := &firewallGroupResourceModel{
		ID:      types.StringNull(),
		Name:    types.StringValue("Servers"),
		Type:    types.StringValue("address-group"),
		Members: types.SetNull(types.StringType),
	}
	g := m.toFirewallGroup(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toFirewallGroup diags: %v", diags)
	}
	if derefStr(g.Name) != "Servers" || string(derefEnum(g.GroupType)) != "address-group" || g.GroupMembers == nil || len(*g.GroupMembers) != 0 {
		t.Errorf("null members must become an empty (non-nil) slice: %+v", g)
	}

	m.Members = sliceToStringSet([]string{"192.0.2.10", "192.0.2.0/24"})
	g = m.toFirewallGroup(ctx, &diags)
	if g.GroupMembers == nil || len(*g.GroupMembers) != 2 {
		t.Errorf("members mapping wrong: %v", g.GroupMembers)
	}

	var out firewallGroupResourceModel
	portGroup := wire.FirewallgroupGroupType("port-group")
	out.fromFirewallGroup(&wire.Firewallgroup{UnderscoreId: sp("fg5"), Name: sp("Ports"), GroupType: &portGroup, GroupMembers: &[]string{"443"}})
	if out.ID.ValueString() != "fg5" || out.Type.ValueString() != "port-group" {
		t.Errorf("fromFirewallGroup wrong: %+v", out)
	}

	// An empty group (controller echoes `[]`) must read back as an empty set, not
	// null — otherwise a `members = []` config trips the post-apply consistency check.
	var empty firewallGroupResourceModel
	addrGroup := wire.FirewallgroupGroupType("address-group")
	empty.fromFirewallGroup(&wire.Firewallgroup{UnderscoreId: sp("fg6"), Name: sp("Empty"), GroupType: &addrGroup, GroupMembers: &[]string{}})
	if empty.Members.IsNull() || len(empty.Members.Elements()) != 0 {
		t.Errorf("empty group members should be a non-null empty set, got %v", empty.Members)
	}
	var members []string
	out.Members.ElementsAs(ctx, &members, false)
	if len(members) != 1 || members[0] != "443" {
		t.Errorf("members round-trip wrong: %v", members)
	}
}

func TestFirewallRule_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &firewallRuleResourceModel{
		ID:                  types.StringNull(),
		Name:                types.StringValue("block-iot-to-lan"),
		Enabled:             types.BoolValue(true),
		Ruleset:             types.StringValue("LAN_IN"),
		RuleIndex:           types.Int64Value(20010),
		Action:              types.StringValue("drop"),
		Protocol:            types.StringValue("all"),
		Logging:             types.BoolValue(true),
		StateNew:            types.BoolValue(true),
		SrcFirewallGroupIDs: sliceToStringSet([]string{"fg-iot"}),
		DstNetworkID:        types.StringValue("net-lan"),
		// leave the rest null
		ProtocolV6:          types.StringNull(),
		ProtocolExcepted:    types.BoolNull(),
		ICMPTypename:        types.StringNull(),
		IPSec:               types.StringNull(),
		StateEstablished:    types.BoolNull(),
		StateRelated:        types.BoolNull(),
		StateInvalid:        types.BoolNull(),
		SrcAddress:          types.StringNull(),
		SrcPort:             types.StringNull(),
		SrcNetworkID:        types.StringNull(),
		SrcNetworkType:      types.StringNull(),
		DstAddress:          types.StringNull(),
		DstPort:             types.StringNull(),
		DstNetworkType:      types.StringNull(),
		DstFirewallGroupIDs: types.SetNull(types.StringType),
		SettingPreference:   types.StringNull(),
	}
	r := m.toFirewallRule(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toFirewallRule diags: %v", diags)
	}
	if derefStr(r.Name) != "block-iot-to-lan" || string(derefEnum(r.Ruleset)) != "LAN_IN" || string(derefEnum(r.Action)) != "drop" {
		t.Errorf("scalar mapping wrong: %+v", r)
	}
	if r.RuleIndex == nil || *r.RuleIndex != 20010 {
		t.Errorf("rule_index mapping wrong: %v", r.RuleIndex)
	}
	if r.Enabled == nil || !*r.Enabled || r.StateNew == nil || !*r.StateNew {
		t.Errorf("bool mapping wrong: enabled=%v state_new=%v", r.Enabled, r.StateNew)
	}
	// Unset optionals stay nil/empty so the sparse PUT leaves the controller's value.
	if r.SrcAddress != nil || r.ProtocolMatchExcepted != nil || r.DstFirewallgroupIds != nil {
		t.Errorf("unset optionals must be nil: src_address=%v excepted=%v dst_groups=%v", r.SrcAddress, r.ProtocolMatchExcepted, r.DstFirewallgroupIds)
	}
	if r.SrcFirewallgroupIds == nil || len(*r.SrcFirewallgroupIds) != 1 || (*r.SrcFirewallgroupIds)[0] != "fg-iot" {
		t.Errorf("src_firewall_group_ids mapping wrong: %v", r.SrcFirewallgroupIds)
	}
	if derefStr(r.DstNetworkconfId) != "net-lan" {
		t.Errorf("dst_network_id mapping wrong: %q", derefStr(r.DstNetworkconfId))
	}

	var out firewallRuleResourceModel
	idx := 20010
	lanIn := wire.FirewallruleRuleset("LAN_IN")
	drop := wire.FirewallruleAction("drop")
	manual := wire.FirewallruleSettingPreference("manual")
	out.fromFirewallRule(&wire.Firewallrule{
		UnderscoreId: sp("fr9"), Name: sp("block-iot-to-lan"), Ruleset: &lanIn, RuleIndex: &idx, Action: &drop,
		Enabled: boolPtrLit(true), SettingPreference: &manual, DstNetworkconfId: sp("net-lan"),
	})
	if out.ID.ValueString() != "fr9" || out.RuleIndex.ValueInt64() != 20010 || out.SettingPreference.ValueString() != "manual" {
		t.Errorf("fromFirewallRule wrong: %+v", out)
	}
	// An unset optional reads back null, not "".
	if !out.SrcAddress.IsNull() {
		t.Errorf("unset src_address should read back null, got %v", out.SrcAddress)
	}
}

func TestRuleIndexConflict(t *testing.T) {
	i1, i2 := 20000, 20001
	lanIn := wire.FirewallruleRuleset("LAN_IN")
	wanIn := wire.FirewallruleRuleset("WAN_IN")
	rules := []wire.Firewallrule{
		{UnderscoreId: sp("a"), Name: sp("first"), Ruleset: &lanIn, RuleIndex: &i1},
		{UnderscoreId: sp("b"), Name: sp("second"), Ruleset: &lanIn, RuleIndex: &i2},
		{UnderscoreId: sp("c"), Name: sp("wan"), Ruleset: &wanIn, RuleIndex: &i1},
	}
	// Same ruleset + index as an existing different rule → conflict.
	if name, conflict := ruleIndexConflict(rules, "LAN_IN", 20000, ""); !conflict || name != "first" {
		t.Errorf("expected conflict with %q, got name=%q conflict=%v", "first", name, conflict)
	}
	// Same index but different ruleset → no conflict.
	if _, conflict := ruleIndexConflict(rules, "WAN_OUT", 20000, ""); conflict {
		t.Errorf("different ruleset must not conflict")
	}
	// Updating the same rule (selfID excluded) → no conflict.
	if _, conflict := ruleIndexConflict(rules, "LAN_IN", 20000, "a"); conflict {
		t.Errorf("self must be excluded from the uniqueness check")
	}
	// A free index → no conflict.
	if _, conflict := ruleIndexConflict(rules, "LAN_IN", 20099, ""); conflict {
		t.Errorf("free index must not conflict")
	}
}

func TestDHCPReservation_RoundTrip(t *testing.T) {
	m := &dhcpReservationResourceModel{
		ID:                    types.StringNull(),
		MAC:                   types.StringValue("AA:BB:CC:DD:EE:01"), // mixed case
		IPAddress:             types.StringValue("192.0.2.10"),
		NetworkID:             types.StringValue("net1"),
		Name:                  types.StringValue("nas"),
		LocalDNSRecord:        types.StringNull(),
		LocalDNSRecordEnabled: types.BoolNull(),
	}
	u := m.toUser()
	if deref(u.Mac) != "aa:bb:cc:dd:ee:01" {
		t.Errorf("mac should be lower-cased on the wire, got %q", deref(u.Mac))
	}
	if u.UseFixedip == nil || !*u.UseFixedip {
		t.Errorf("toUser must always assert use_fixedip=true for a reservation")
	}
	if deref(u.FixedIp) != "192.0.2.10" || deref(u.NetworkId) != "net1" || deref(u.Name) != "nas" {
		t.Errorf("scalar mapping wrong: %+v", u)
	}

	// fromUser must NOT overwrite mac (immutable key, controller lowercases it),
	// so a mixed-case config doesn't churn.
	m.fromUser(&wire.User{UnderscoreId: sp("u9"), Mac: sp("aa:bb:cc:dd:ee:01"), FixedIp: sp("192.0.2.10"), Hostname: sp("nas"), NetworkId: sp("net1")})
	if m.ID.ValueString() != "u9" {
		t.Errorf("id round-trip wrong: %q", m.ID.ValueString())
	}
	if m.MAC.ValueString() != "AA:BB:CC:DD:EE:01" {
		t.Errorf("fromUser must preserve the configured mac, got %q", m.MAC.ValueString())
	}
	if m.Hostname.ValueString() != "nas" {
		t.Errorf("hostname should round-trip, got %q", m.Hostname.ValueString())
	}
}

func TestPortProfile_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &portProfileResourceModel{
		ID:                 types.StringNull(),
		Name:               types.StringValue("Trunk"),
		Forward:            types.StringValue("customize"),
		NativeNetworkID:    types.StringValue("net-mgmt"),
		TaggedVLANMgmt:     types.StringValue("custom"),
		ExcludedNetworkIDs: sliceToStringSet([]string{"net-iot"}),
		POEMode:            types.StringValue("auto"),
		Autoneg:            types.BoolValue(false),
		Speed:              types.Int64Value(1000),
		STPPortMode:        types.BoolValue(false),
		// leave the rest null so the sparse PUT leaves the controller's value
		VoiceNetworkID:           types.StringNull(),
		FullDuplex:               types.BoolNull(),
		OpMode:                   types.StringNull(),
		PortSecurityEnabled:      types.BoolNull(),
		PortSecurityMACAddresses: types.SetNull(types.StringType),
		Dot1XCtrl:                types.StringNull(),
		Dot1XIdleTimeout:         types.Int64Null(),
		Isolation:                types.BoolNull(),
		EgressRateLimitEnabled:   types.BoolNull(),
		EgressRateLimitKbps:      types.Int64Null(),
		STPBPDUGuard:             types.BoolNull(),
		STPEdgeState:             types.StringNull(),
		LLDPMEDEnabled:           types.BoolNull(),
		PortKeepaliveEnabled:     types.BoolNull(),
		MulticastRouterMode:      types.StringNull(),
	}
	p := m.toPortProfile(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toPortProfile diags: %v", diags)
	}
	if deref(p.Name) != "Trunk" || string(derefEnum(p.Forward)) != "customize" || deref(p.NativeNetworkconfId) != "net-mgmt" {
		t.Errorf("scalar mapping wrong: %+v", p)
	}
	// Explicit false on autoneg must be a non-nil *bool so the per-port merge writes it.
	if p.Autoneg == nil || *p.Autoneg != false || p.StpPortMode == nil || *p.StpPortMode != false {
		t.Errorf("explicit false bool must be non-nil: autoneg=%v stp_port_mode=%v", p.Autoneg, p.StpPortMode)
	}
	if p.Speed == nil || *p.Speed != 1000 {
		t.Errorf("speed mapping wrong: %v", p.Speed)
	}
	if p.ExcludedNetworkconfIds == nil || len(*p.ExcludedNetworkconfIds) != 1 || (*p.ExcludedNetworkconfIds)[0] != "net-iot" {
		t.Errorf("excluded_network_ids mapping wrong: %v", p.ExcludedNetworkconfIds)
	}
	// Unset scalar optionals stay nil/empty so the per-port merge leaves the controller's value.
	if p.FullDuplex != nil || p.Dot1xCtrl != nil {
		t.Errorf("unset scalar optionals must be nil: full_duplex=%v dot1x=%v", p.FullDuplex, p.Dot1xCtrl)
	}
	// The owned MAC list, unset (null config), must send a concrete empty slice
	// (not nil) so an explicit `[]` clears it on the wire instead of reading null.
	if p.PortSecurityMacAddress == nil || len(*p.PortSecurityMacAddress) != 0 {
		t.Errorf("owned port_security_mac_address must be a non-nil empty slice when unset, got %v", p.PortSecurityMacAddress)
	}

	// An explicit empty exclusion set must reach the wire as a concrete empty slice.
	m.ExcludedNetworkIDs = sliceToStringSet([]string{})
	if cleared := m.toPortProfile(ctx, &diags); cleared.ExcludedNetworkconfIds == nil || len(*cleared.ExcludedNetworkconfIds) != 0 {
		t.Errorf("explicit empty excluded_network_ids must marshal as []; got %v", cleared.ExcludedNetworkconfIds)
	}

	var out portProfileResourceModel
	allEmpty := []string{}
	allPower := wire.PortconfPoeMode("passthrough")
	allFwd := wire.PortconfForward("all")
	out.fromPortProfile(&wire.Portconf{
		UnderscoreId: sp("pc7"), Name: sp("Uplink"), Forward: &allFwd, PoeMode: &allPower,
		Autoneg: boolPtrLit(true), ExcludedNetworkconfIds: &allEmpty,
	})
	if out.ID.ValueString() != "pc7" || out.Forward.ValueString() != "all" || out.POEMode.ValueString() != "passthrough" {
		t.Errorf("fromPortProfile wrong: %+v", out)
	}
	if out.Autoneg.IsNull() || !out.Autoneg.ValueBool() {
		t.Errorf("autoneg should round-trip true, got %v", out.Autoneg)
	}
	// An explicit empty array reads back as a non-null empty set.
	if out.ExcludedNetworkIDs.IsNull() || len(out.ExcludedNetworkIDs.Elements()) != 0 {
		t.Errorf("empty excluded_network_ids should be a non-null empty set, got %v", out.ExcludedNetworkIDs)
	}
	// An unset optional string reads back null.
	if !out.VoiceNetworkID.IsNull() {
		t.Errorf("unset voice_network_id should read back null, got %v", out.VoiceNetworkID)
	}
}

func TestUserGroup_RoundTrip(t *testing.T) {
	// Config → wire: a capped group. Both rate fields ride along (including an
	// explicit 0, the controller's unlimited sentinel).
	m := &userGroupResourceModel{
		ID:             types.StringNull(),
		Name:           types.StringValue("Capped"),
		QOSRateMaxUp:   types.Int64Value(5000),
		QOSRateMaxDown: types.Int64Value(0),
	}
	g := m.toUserGroup()
	if deref(g.Name) != "Capped" {
		t.Errorf("name mapping wrong: %+v", g)
	}
	if g.QosRateMaxUp == nil || *g.QosRateMaxUp != 5000 {
		t.Errorf("qos_rate_max_up should map to &5000, got %v", g.QosRateMaxUp)
	}
	if g.QosRateMaxDown == nil || *g.QosRateMaxDown != 0 {
		t.Errorf("explicit 0 must map to &0 (not nil), got %v", g.QosRateMaxDown)
	}

	// An unconfigured cap stays nil so the sparse PUT leaves the controller's value.
	m.QOSRateMaxUp = types.Int64Null()
	if g := m.toUserGroup(); g.QosRateMaxUp != nil {
		t.Errorf("null qos_rate_max_up must map to nil, got %v", g.QosRateMaxUp)
	}

	// Wire → state.
	var out userGroupResourceModel
	up, down := 10000, 20000
	out.fromUserGroup(&wire.Usergroup{UnderscoreId: sp("ug9"), Name: sp("Renamed"), QosRateMaxUp: &up, QosRateMaxDown: &down})
	if out.ID.ValueString() != "ug9" || out.Name.ValueString() != "Renamed" {
		t.Errorf("fromUserGroup wrong: %+v", out)
	}
	if out.QOSRateMaxUp.ValueInt64() != 10000 || out.QOSRateMaxDown.ValueInt64() != 20000 {
		t.Errorf("rate round-trip wrong: up=%v down=%v", out.QOSRateMaxUp, out.QOSRateMaxDown)
	}
	if out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should read back false when the controller omits it, got true")
	}

	// The protected default group reads attr_no_delete = true (destroy → no-op).
	protected := true
	out.fromUserGroup(&wire.Usergroup{UnderscoreId: sp("default"), Name: sp("Default"), AttrNoDelete: &protected})
	if !out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should round-trip true for the protected default group")
	}
}

func TestRadiusProfile_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	authServers, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: radiusServerAttrTypes}, []radiusServerObject{
		{IP: types.StringValue("192.0.2.9"), Port: types.Int64Value(1812), XSecret: types.StringValue("s3cret")},
	})
	diags.Append(d...)
	if diags.HasError() {
		t.Fatalf("building auth_servers: %v", diags)
	}

	m := &radiusProfileResourceModel{
		ID:               types.StringNull(),
		Name:             types.StringValue("Corp"),
		AuthServers:      authServers,
		AcctServers:      types.ListNull(types.ObjectType{AttrTypes: radiusServerAttrTypes}),
		UseUSGAuthServer: types.BoolValue(false),
		VLANWLANMode:     types.StringValue("optional"),
	}
	p := m.toRadiusProfile(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toRadiusProfile diags: %v", diags)
	}
	if deref(p.Name) != "Corp" || string(derefEnum(p.VlanWlanMode)) != "optional" {
		t.Errorf("scalar mapping wrong: %+v", p)
	}
	if p.AuthServers == nil || len(*p.AuthServers) != 1 ||
		deref((*p.AuthServers)[0].Ip) != "192.0.2.9" || intOrZero((*p.AuthServers)[0].Port) != 1812 || deref((*p.AuthServers)[0].XSecret) != "s3cret" {
		t.Errorf("auth_servers mapping wrong: %+v", p.AuthServers)
	}
	// A null server list maps to a non-nil empty slice (full-replace clears it).
	if p.AcctServers == nil || len(*p.AcctServers) != 0 {
		t.Errorf("null acct_servers must map to a non-nil empty slice, got %v", p.AcctServers)
	}
	if p.UseUsgAuthServer == nil || *p.UseUsgAuthServer {
		t.Errorf("use_usg_auth_server should map to &false, got %v", p.UseUsgAuthServer)
	}

	// Wire → state.
	var out radiusProfileResourceModel
	port := 1812
	out.fromRadiusProfile(ctx, &wire.Radiusprofile{
		UnderscoreId: sp("rp9"), Name: sp("Corp"),
		AuthServers: &[]radiusServer{{Ip: sp("192.0.2.9"), Port: &port, XSecret: sp("s3cret")}},
		AcctServers: &[]radiusServer{},
	}, &diags)
	if out.ID.ValueString() != "rp9" || out.Name.ValueString() != "Corp" {
		t.Errorf("fromRadiusProfile wrong: %+v", out)
	}
	var servers []radiusServerObject
	out.AuthServers.ElementsAs(ctx, &servers, false)
	if len(servers) != 1 || servers[0].IP.ValueString() != "192.0.2.9" || servers[0].XSecret.ValueString() != "s3cret" {
		t.Errorf("auth_servers round-trip wrong: %+v", servers)
	}
	// An empty controller list reads back as a non-null empty list (Computed).
	if out.AcctServers.IsNull() || len(out.AcctServers.Elements()) != 0 {
		t.Errorf("empty acct_servers should be a non-null empty list, got %v", out.AcctServers)
	}
	if out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should read back false when omitted, got true")
	}

	// The protected default profile reads attr_no_delete = true.
	protected := true
	out.fromRadiusProfile(ctx, &wire.Radiusprofile{UnderscoreId: sp("default"), Name: sp("Default"), AttrNoDelete: &protected}, &diags)
	if !out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should round-trip true for the protected default profile")
	}
}

func TestRadiusAccount_RoundTrip(t *testing.T) {
	m := &radiusAccountResourceModel{
		ID:               types.StringNull(),
		Name:             types.StringValue("alice"),
		Password:         types.StringValue("pw"),
		TunnelType:       types.Int64Value(13),
		TunnelMediumType: types.Int64Value(6),
		VLAN:             types.Int64Value(30),
	}
	a := m.toRadiusAccount()
	if deref(a.Name) != "alice" || deref(a.XPassword) != "pw" || a.Vlan == nil || *a.Vlan != 30 {
		t.Errorf("scalar mapping wrong: %+v", a)
	}
	if a.TunnelType == nil || *a.TunnelType != 13 || a.TunnelMediumType == nil || *a.TunnelMediumType != 6 {
		t.Errorf("tunnel attrs mapping wrong: %+v", a)
	}

	// Write-only password: a read that doesn't echo it keeps the configured value.
	out := radiusAccountResourceModel{Password: types.StringValue("keep-me")}
	vlan := 30
	out.fromRadiusAccount(&wire.Account{UnderscoreId: sp("ac9"), Name: sp("alice"), Vlan: &vlan})
	if out.ID.ValueString() != "ac9" {
		t.Errorf("id round-trip wrong: %q", out.ID.ValueString())
	}
	if out.Password.ValueString() != "keep-me" {
		t.Errorf("write-only password should be preserved on read, got %q", out.Password.ValueString())
	}
	// An unset tunnel attr reads back null, not 0.
	if !out.TunnelType.IsNull() {
		t.Errorf("unset tunnel_type should read back null, got %v", out.TunnelType)
	}
}

func TestStaticRoute_RoundTrip(t *testing.T) {
	m := &staticRouteResourceModel{
		ID:        types.StringNull(),
		Name:      types.StringValue("to-lab"),
		Enabled:   types.BoolValue(true),
		Network:   types.StringValue("203.0.113.0/24"),
		RouteType: types.StringValue("nexthop-route"),
		NextHop:   types.StringValue("192.0.2.254"),
		Distance:  types.Int64Value(1),
		Interface: types.StringNull(),
	}
	rt := m.toRouting()
	if deref(rt.Name) != "to-lab" || deref(rt.StaticRouteNetwork) != "203.0.113.0/24" || deref(rt.StaticRouteNexthop) != "192.0.2.254" {
		t.Errorf("scalar mapping wrong: %+v", rt)
	}
	// The client always stamps the controller's discriminator.
	if deref(rt.Type) != "static-route" {
		t.Errorf("type should be stamped static-route, got %q", deref(rt.Type))
	}
	if rt.Enabled == nil || !*rt.Enabled || rt.StaticRouteDistance == nil || *rt.StaticRouteDistance != 1 {
		t.Errorf("enabled/distance mapping wrong: %v / %v", rt.Enabled, rt.StaticRouteDistance)
	}

	var out staticRouteResourceModel
	bhType := wire.RoutingStaticRouteType("blackhole")
	out.fromRouting(&wire.Routing{
		UnderscoreId: sp("rt9"), Name: sp("to-lab"), Enabled: boolPtrLit(false),
		StaticRouteNetwork: sp("203.0.113.0/24"), StaticRouteType: &bhType,
	})
	if out.ID.ValueString() != "rt9" || out.RouteType.ValueString() != "blackhole" {
		t.Errorf("fromRouting wrong: %+v", out)
	}
	if out.Enabled.ValueBool() {
		t.Errorf("enabled should round-trip false")
	}
	// An unset optional reads back null, not "".
	if !out.NextHop.IsNull() {
		t.Errorf("unset next_hop should read back null, got %v", out.NextHop)
	}
}

func TestPortForward_RoundTrip(t *testing.T) {
	m := &portForwardResourceModel{
		ID:              types.StringNull(),
		Name:            types.StringValue("https"),
		Enabled:         types.BoolValue(true),
		Interface:       types.StringValue("wan"),
		Protocol:        types.StringValue("tcp"),
		Source:          types.StringValue("any"),
		DestinationPort: types.StringValue("443"),
		ForwardIP:       types.StringValue("192.0.2.5"),
		ForwardPort:     types.StringValue("8443"),
		Log:             types.BoolValue(false),
	}
	pf := m.toPortForward()
	if deref(pf.Name) != "https" || deref(pf.PfwdInterface) != "wan" || string(derefEnum(pf.Proto)) != "tcp" {
		t.Errorf("scalar mapping wrong: %+v", pf)
	}
	if deref(pf.DstPort) != "443" || deref(pf.Fwd) != "192.0.2.5" || deref(pf.FwdPort) != "8443" {
		t.Errorf("forward fields mapping wrong: %+v", pf)
	}
	if pf.Enabled == nil || !*pf.Enabled || pf.Log == nil || *pf.Log {
		t.Errorf("enabled/log mapping wrong: %v / %v", pf.Enabled, pf.Log)
	}

	var out portForwardResourceModel
	proto := wire.PortforwardProto("tcp_udp")
	out.fromPortForward(&wire.Portforward{
		UnderscoreId: sp("pf9"), Name: sp("https"), Enabled: boolPtrLit(true), PfwdInterface: sp("both"),
		Proto: &proto, DstPort: sp("443"), Fwd: sp("192.0.2.5"),
	})
	if out.ID.ValueString() != "pf9" || out.Interface.ValueString() != "both" || out.Protocol.ValueString() != "tcp_udp" {
		t.Errorf("fromPortForward wrong: %+v", out)
	}
	// An unset optional reads back null.
	if !out.ForwardPort.IsNull() {
		t.Errorf("unset forward_port should read back null, got %v", out.ForwardPort)
	}
}

func TestDynamicDNS_RoundTrip(t *testing.T) {
	m := &dynamicDNSResourceModel{
		ID:        types.StringNull(),
		Service:   types.StringValue("dyndns"),
		Hostname:  types.StringValue("home.example.com"),
		Server:    types.StringNull(),
		Login:     types.StringValue("user"),
		Password:  types.StringValue("pw"),
		Interface: types.StringValue("wan"),
	}
	d := m.toDynamicDNS()
	if derefEnum(d.Service) != "dyndns" || derefStr(d.HostName) != "home.example.com" || derefStr(d.Login) != "user" || derefStr(d.XPassword) != "pw" {
		t.Errorf("scalar mapping wrong: %+v", d)
	}

	// Write-only password: a read that doesn't echo it keeps the configured value.
	out := dynamicDNSResourceModel{Password: types.StringValue("keep-me")}
	dyndns := wire.DynamicdnsService("dyndns")
	out.fromDynamicDNS(&wire.Dynamicdns{UnderscoreId: sp("dd9"), Service: &dyndns, HostName: sp("home.example.com"), Interface: sp("wan")})
	if out.ID.ValueString() != "dd9" || out.Service.ValueString() != "dyndns" {
		t.Errorf("fromDynamicDNS wrong: %+v", out)
	}
	if out.Password.ValueString() != "keep-me" {
		t.Errorf("write-only password should be preserved on read, got %q", out.Password.ValueString())
	}
	// An unset optional reads back null.
	if !out.Server.IsNull() {
		t.Errorf("unset server should read back null, got %v", out.Server)
	}
}

func TestWLANGroup_RoundTrip(t *testing.T) {
	m := &wlanGroupResourceModel{
		ID:   types.StringNull(),
		Name: types.StringValue("Guest APs"),
	}
	g := m.toWLANGroup()
	if derefStr(g.Name) != "Guest APs" {
		t.Errorf("name mapping wrong: %+v", g)
	}

	var out wlanGroupResourceModel
	out.fromWLANGroup(&wire.Wlangroup{UnderscoreId: sp("wg9"), Name: sp("Guest APs")})
	if out.ID.ValueString() != "wg9" || out.Name.ValueString() != "Guest APs" {
		t.Errorf("fromWLANGroup wrong: %+v", out)
	}
	if out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should read back false when omitted, got true")
	}

	// The protected default group reads attr_no_delete = true (destroy → no-op).
	protected := true
	out.fromWLANGroup(&wire.Wlangroup{UnderscoreId: sp("default"), Name: sp("Default"), AttrNoDelete: &protected})
	if !out.AttrNoDelete.ValueBool() {
		t.Errorf("attr_no_delete should round-trip true for the protected default group")
	}
}

func TestTrafficRule_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	devices, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: trafficTargetDeviceAttrTypes}, []trafficTargetDeviceObject{
		{Type: types.StringValue("CLIENT"), ClientMAC: types.StringValue("aa:bb:cc:00:11:22"), NetworkID: types.StringNull()},
	})
	diags.Append(d...)
	bw, d := types.ObjectValueFrom(ctx, bandwidthLimitAttrTypes, bandwidthLimitObject{
		Enabled: types.BoolValue(true), DownloadLimitKbps: types.Int64Value(5000), UploadLimitKbps: types.Int64Null(),
	})
	diags.Append(d...)
	if diags.HasError() {
		t.Fatalf("building nested values: %v", diags)
	}

	// One domain with a port and a port-range, exercising the nested converters.
	prList, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: trafficPortRangeAttrTypes}, []trafficPortRangeModel{
		{PortStart: types.Int64Value(8000), PortStop: types.Int64Value(8010)},
	})
	diags.Append(d...)
	domains, d := types.SetValueFrom(ctx, domainSetType.ElemType, []trafficDomainModel{
		{Domain: types.StringValue("netflix.com"), Ports: sliceToIntSet([]int{443}), PortRanges: prList},
	})
	diags.Append(d...)
	if diags.HasError() {
		t.Fatalf("building nested values: %v", diags)
	}

	m := &trafficRuleResourceModel{
		ID:             types.StringNull(),
		Enabled:        types.BoolValue(true),
		Action:         types.StringValue("SPEED_LIMIT"),
		MatchingTarget: types.StringValue("DOMAIN"),
		Description:    types.StringValue("throttle streaming"),
		Domains:        domains,
		IPAddresses:    types.SetNull(ipAddressSetType.ElemType),
		IPRanges:       types.SetNull(ipRangeSetType.ElemType),
		Regions:        types.SetNull(types.StringType),
		AppIDs:         types.SetNull(types.Int64Type),
		AppCategoryIDs: sliceToIntSet([]int{4, 13}),
		NetworkIDs:     types.SetNull(types.StringType),
		TargetDevices:  devices,
		BandwidthLimit: bw,
	}
	rule := m.toTrafficRule(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toTrafficRule diags: %v", diags)
	}
	if rule.Action != "SPEED_LIMIT" || rule.MatchingTarget != "DOMAIN" {
		t.Errorf("scalar mapping wrong: %+v", rule)
	}
	if rule.Domains == nil || len(*rule.Domains) != 1 || (*rule.Domains)[0].Domain != "netflix.com" {
		t.Errorf("domains mapping wrong: %v", rule.Domains)
	}
	dm := (*rule.Domains)[0]
	if dm.Ports == nil || len(*dm.Ports) != 1 || (*dm.Ports)[0] != 443 {
		t.Errorf("domain ports mapping wrong: %v", dm.Ports)
	}
	if dm.PortRanges == nil || len(*dm.PortRanges) != 1 || *(*dm.PortRanges)[0].PortStart != 8000 || *(*dm.PortRanges)[0].PortStop != 8010 {
		t.Errorf("domain port_ranges mapping wrong: %v", dm.PortRanges)
	}
	if rule.AppCategoryIds == nil || len(*rule.AppCategoryIds) != 2 {
		t.Errorf("app_category_ids mapping wrong: %v", rule.AppCategoryIds)
	}
	if len(rule.TargetDevices) != 1 || rule.TargetDevices[0].Type != "CLIENT" || deref(rule.TargetDevices[0].ClientMac) != "aa:bb:cc:00:11:22" {
		t.Errorf("target_devices mapping wrong: %+v", rule.TargetDevices)
	}
	if rule.BandwidthLimit == nil || rule.BandwidthLimit.DownloadLimitKbps == nil || *rule.BandwidthLimit.DownloadLimitKbps != 5000 {
		t.Errorf("bandwidth_limit mapping wrong: %+v", rule.BandwidthLimit)
	}
	// Unset upload stays nil (omitted on the wire).
	if rule.BandwidthLimit.UploadLimitKbps != nil {
		t.Errorf("unset upload must stay nil, got %v", rule.BandwidthLimit.UploadLimitKbps)
	}

	var out trafficRuleResourceModel
	out.fromTrafficRule(ctx, &wire.Trafficrules{
		UnderscoreId: sp("tr9"), Action: "BLOCK", MatchingTarget: "INTERNET", Enabled: boolPtrLit(true),
		TargetDevices: []wire.TrafficroutesTargetDevice{{Type: "ALL_CLIENTS"}},
	}, &diags)
	if out.ID.ValueString() != "tr9" || out.Action.ValueString() != "BLOCK" {
		t.Errorf("fromTrafficRule wrong: %+v", out)
	}
	var od []trafficTargetDeviceObject
	out.TargetDevices.ElementsAs(ctx, &od, false)
	if len(od) != 1 || od[0].Type.ValueString() != "ALL_CLIENTS" {
		t.Errorf("target_devices round-trip wrong: %+v", od)
	}
	// Unset criteria read back null.
	if !out.Domains.IsNull() || !out.BandwidthLimit.IsNull() {
		t.Errorf("unset domains/bandwidth_limit should read back null: %v / %v", out.Domains, out.BandwidthLimit)
	}
}

// TestTrafficRule_EmptyCriteriaReadBackNull pins the HIL W1 finding: the
// controller echoes empty arrays (not omitted fields) for unset match criteria,
// and a disabled default bandwidth_limit on non-speed-limit rules. Each must read
// back null so an omitted field round-trips — otherwise create reports "Provider
// produced inconsistent result after apply" (planned null vs an empty set/object).
func TestTrafficRule_EmptyCriteriaReadBackNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	var out trafficRuleResourceModel
	out.fromTrafficRule(ctx, &wire.Trafficrules{
		UnderscoreId: sp("tr-empty"), Action: "BLOCK", MatchingTarget: "INTERNET", Enabled: boolPtrLit(true),
		TargetDevices:  []wire.TrafficroutesTargetDevice{{Type: "ALL_CLIENTS"}},
		Domains:        &[]wire.TrafficroutesDomain{},
		IpAddresses:    &[]wire.TrafficroutesIpAddress{},
		IpRanges:       &[]wire.TrafficroutesIpRange{},
		Regions:        &[]string{},
		AppIds:         &[]int{},
		AppCategoryIds: &[]int{},
		NetworkIds:     &[]string{},
		BandwidthLimit: &wire.TrafficruleBandwidthLimit{Enabled: boolPtrLit(false)},
	}, &diags)
	if diags.HasError() {
		t.Fatalf("fromTrafficRule diags: %v", diags)
	}
	for field, isNull := range map[string]bool{
		"domains":          out.Domains.IsNull(),
		"ip_addresses":     out.IPAddresses.IsNull(),
		"ip_ranges":        out.IPRanges.IsNull(),
		"regions":          out.Regions.IsNull(),
		"app_ids":          out.AppIDs.IsNull(),
		"app_category_ids": out.AppCategoryIDs.IsNull(),
		"network_ids":      out.NetworkIDs.IsNull(),
		"bandwidth_limit":  out.BandwidthLimit.IsNull(),
	} {
		if !isNull {
			t.Errorf("%s: an empty/disabled wire value must read back null", field)
		}
	}
}

func TestTrafficRoute_RoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	domains, d := types.SetValueFrom(ctx, domainSetType.ElemType, []trafficDomainModel{
		{Domain: types.StringValue("netflix.com"), Ports: types.SetNull(types.Int64Type), PortRanges: types.ListNull(types.ObjectType{AttrTypes: trafficPortRangeAttrTypes})},
	})
	diags.Append(d...)
	if diags.HasError() {
		t.Fatalf("building domains: %v", diags)
	}

	m := &trafficRouteResourceModel{
		ID:                types.StringNull(),
		Enabled:           types.BoolValue(true),
		Description:       types.StringValue("netflix via vpn"),
		MatchingTarget:    types.StringValue("DOMAIN"),
		Domains:           domains,
		IPAddresses:       types.SetNull(ipAddressSetType.ElemType),
		IPRanges:          types.SetNull(ipRangeSetType.ElemType),
		Regions:           types.SetNull(types.StringType),
		NetworkID:         types.StringValue("vpn1"),
		TargetDevices:     types.ListNull(types.ObjectType{AttrTypes: trafficTargetDeviceAttrTypes}),
		KillSwitchEnabled: types.BoolValue(true),
	}
	route := m.toTrafficRoute(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toTrafficRoute diags: %v", diags)
	}
	if route.MatchingTarget != "DOMAIN" || route.NetworkId != "vpn1" {
		t.Errorf("scalar mapping wrong: %+v", route)
	}
	if route.Domains == nil || len(*route.Domains) != 1 || (*route.Domains)[0].Domain != "netflix.com" {
		t.Errorf("domains mapping wrong: %v", route.Domains)
	}
	if route.KillSwitchEnabled == nil || !*route.KillSwitchEnabled {
		t.Errorf("kill_switch mapping wrong: %v", route.KillSwitchEnabled)
	}
	// target_devices is a required (non-omitempty) array; an unset list serializes
	// as an empty slice, not null.
	if len(route.TargetDevices) != 0 {
		t.Errorf("null target_devices should map to empty slice, got %v", route.TargetDevices)
	}

	var out trafficRouteResourceModel
	out.fromTrafficRoute(ctx, &wire.Trafficroutes{
		UnderscoreId: sp("rr9"), MatchingTarget: "DOMAIN",
		Domains: &[]wire.TrafficroutesDomain{{Domain: "netflix.com"}}, NetworkId: "vpn2", Enabled: boolPtrLit(true),
	}, &diags)
	if out.ID.ValueString() != "rr9" || out.NetworkID.ValueString() != "vpn2" {
		t.Errorf("fromTrafficRoute wrong: %+v", out)
	}
	if !out.TargetDevices.IsNull() {
		t.Errorf("absent target_devices should read back null, got %v", out.TargetDevices)
	}
	if out.Domains.IsNull() {
		t.Errorf("domains should read back set, got null")
	}
}

func TestDevice_PortOverrideRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// Build a config map: port 5 with a PoE override + an excluded VLAN.
	obj := portOverrideObject{
		PortProfileID:            types.StringNull(),
		Name:                     types.StringValue("camera"),
		POEMode:                  types.StringValue("auto"),
		Forward:                  types.StringNull(),
		NativeNetworkID:          types.StringValue("net-iot"),
		TaggedVLANMgmt:           types.StringValue("custom"),
		ExcludedNetworkIDs:       sliceToStringSet([]string{"net-mgmt"}),
		VoiceNetworkID:           types.StringNull(),
		Autoneg:                  types.BoolValue(true),
		Speed:                    types.Int64Null(),
		FullDuplex:               types.BoolNull(),
		Isolation:                types.BoolNull(),
		PortSecurityEnabled:      types.BoolNull(),
		PortSecurityMACAddresses: types.SetNull(types.StringType),
		Dot1XCtrl:                types.StringNull(),
		Dot1XIdleTimeout:         types.Int64Null(),
		EgressRateLimitEnabled:   types.BoolNull(),
		EgressRateLimitKbps:      types.Int64Null(),
		STPPortMode:              types.BoolNull(),
		STPBPDUGuard:             types.BoolNull(),
		STPEdgeState:             types.StringNull(),
		LLDPMEDEnabled:           types.BoolNull(),
		PortKeepaliveEnabled:     types.BoolNull(),
		MulticastRouterMode:      types.StringNull(),
	}
	elem, d := types.ObjectValueFrom(ctx, portOverrideAttrTypes, obj)
	diags.Append(d...)
	poMap, d := types.MapValue(types.ObjectType{AttrTypes: portOverrideAttrTypes}, map[string]attr.Value{"5": elem})
	diags.Append(d...)
	if diags.HasError() {
		t.Fatalf("building map: %v", diags)
	}

	desired, keys := desiredPortOverrides(ctx, poMap, &diags)
	if diags.HasError() {
		t.Fatalf("desiredPortOverrides: %v", diags)
	}
	if len(keys) != 1 || keys[0] != 5 || len(desired) != 1 {
		t.Fatalf("keys/desired wrong: keys=%v desired=%+v", keys, desired)
	}
	po := desired[0]
	if po.PortIdx != 5 || po.Name != "camera" || po.PoEMode != "auto" || po.NativeNetworkConfID != "net-iot" {
		t.Errorf("port override mapping wrong: %+v", po)
	}
	if po.Autoneg == nil || !*po.Autoneg {
		t.Errorf("autoneg should map to &true, got %v", po.Autoneg)
	}
	if len(po.ExcludedNetworkConfIDs) != 1 || po.ExcludedNetworkConfIDs[0] != "net-mgmt" {
		t.Errorf("excluded_network_ids mapping wrong: %v", po.ExcludedNetworkConfIDs)
	}
	// Unset optionals stay nil so a per-port merge preserves the controller's value.
	if po.Speed != nil || po.STPPortMode != nil || po.PortSecurityMACAddress != nil {
		t.Errorf("unset optionals must be nil: speed=%v stp=%v macs=%v", po.Speed, po.STPPortMode, po.PortSecurityMACAddress)
	}

	// from-direction: build the map for the managed key (5) only, ignoring port 9.
	built := buildPortOverridesMap(ctx, []network.PortOverride{
		{PortIdx: 5, Name: "camera", PoEMode: "auto"},
		{PortIdx: 9, Name: "other"},
	}, []int{5}, nil, &diags)
	if diags.HasError() {
		t.Fatalf("buildPortOverridesMap: %v", diags)
	}
	if _, ok := built.Elements()["5"]; !ok {
		t.Errorf("managed port 5 missing from built map: %v", built)
	}
	if _, ok := built.Elements()["9"]; ok {
		t.Errorf("unmanaged port 9 must not leak into state: %v", built)
	}

	// A managed port the controller hasn't echoed yet is kept via the fallback
	// (the just-written desired value), so it can't vanish post-apply.
	kept := buildPortOverridesMap(ctx, nil, []int{5}, []network.PortOverride{{PortIdx: 5, Name: "camera"}}, &diags)
	if _, ok := kept.Elements()["5"]; !ok {
		t.Errorf("declared port 5 should be kept via fallback, got %v", kept)
	}

	// On Read (nil fallback), a managed port the controller no longer has is dropped.
	gone := buildPortOverridesMap(ctx, []network.PortOverride{{PortIdx: 9}}, []int{5}, nil, &diags)
	if _, ok := gone.Elements()["5"]; ok {
		t.Errorf("port 5 absent on the controller (no fallback) should drop from the map, got %v", gone)
	}
}

func TestDevice_ImportCapturesAllPorts(t *testing.T) {
	var diags diag.Diagnostics

	// Import must capture every live port override (the #57 bug: it captured none).
	dev := &wire.Device{}
	if err := json.Unmarshal([]byte(`{"port_overrides":[{"port_idx":3},{"port_idx":1},{"port_idx":2}]}`), dev); err != nil {
		t.Fatalf("unmarshal device: %v", err)
	}
	keys := allPortKeys(dev, &diags)
	if diags.HasError() {
		t.Fatalf("allPortKeys: %v", diags)
	}
	want := []int{1, 2, 3}
	if len(keys) != len(want) {
		t.Fatalf("allPortKeys = %v, want %v (every live override, sorted)", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("keys[%d] = %d, want %d", i, keys[i], want[i])
		}
	}

	// No overrides → capture nothing (empty, not an error).
	if got := allPortKeys(&wire.Device{}, &diags); len(got) != 0 {
		t.Errorf("device with no overrides should capture no ports, got %v", got)
	}

	// The captured keys build a full managed map (vs the old empty-on-import).
	built := buildPortOverridesMap(context.Background(),
		[]network.PortOverride{{PortIdx: 1}, {PortIdx: 2}, {PortIdx: 3}}, keys, nil, &diags)
	if n := len(built.Elements()); n != 3 {
		t.Errorf("imported map should hold all 3 ports, got %d: %v", n, built)
	}
}

func TestUnionInts(t *testing.T) {
	got := unionInts([]int{3, 1, 2}, []int{2, 5})
	want := []int{1, 2, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("unionInts len = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unionInts[%d] = %d, want %d (full %v)", i, got[i], want[i], got)
		}
	}
}
