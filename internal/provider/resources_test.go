package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
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

	for _, name := range []string{"unifi_network_network", "unifi_network_wan", "unifi_network_dns_record", "unifi_network_wlan", "unifi_network_ap_group", "unifi_network_firewall_group", "unifi_network_firewall_rule", "unifi_network_dhcp_reservation", "unifi_network_port_profile"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %q not registered", name)
		}
	}
	for _, name := range []string{"unifi_network_network", "unifi_network_networks", "unifi_network_site", "unifi_network_application_info"} {
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
		Subnet:            types.StringValue("10.0.30.1/24"),
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
	if conf.VLAN == nil || *conf.VLAN != 30 || conf.VLANEnabled == nil || !*conf.VLANEnabled {
		t.Errorf("VLAN mapping wrong: VLAN=%v Enabled=%v", conf.VLAN, conf.VLANEnabled)
	}
	if derefStr(conf.DHCPDDNS1) != "1.1.1.1" || derefStr(conf.DHCPDDNS2) != "8.8.8.8" {
		t.Errorf("dhcp dns mapping wrong: %v %v", conf.DHCPDDNS1, conf.DHCPDDNS2)
	}
	if conf.DHCPDDNSEnabled == nil || !*conf.DHCPDDNSEnabled {
		t.Errorf("DHCPDDNSEnabled should be true when servers are set")
	}

	// No VLAN ID → vlan disabled explicitly (not left nil).
	m.VLANID = types.Int64Null()
	conf = m.toNetworkConf(ctx, &diags)
	if conf.VLAN != nil || conf.VLANEnabled == nil || *conf.VLANEnabled {
		t.Errorf("untagged network: VLAN=%v Enabled=%v, want nil/false", conf.VLAN, conf.VLANEnabled)
	}

	// Wire → state.
	on := true
	vid := 40
	var out networkResourceModel
	out.fromNetworkConf(&unifi.NetworkConf{
		ID: "net9", Name: "guests", Purpose: "guest", NetworkGroup: "LAN",
		VLANEnabled: &on, VLAN: &vid, DHCPDDNS1: sp("9.9.9.9"),
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
	if conf.Purpose != "wan" {
		t.Errorf("purpose = %q, want wan", conf.Purpose)
	}
	if conf.WANDNSPreference != "manual" || derefStr(conf.WANDNS1) != "1.1.1.1" || derefStr(conf.WANDNS2) != "8.8.8.8" {
		t.Errorf("manual DNS mapping wrong: pref=%q dns1=%v dns2=%v", conf.WANDNSPreference, conf.WANDNS1, conf.WANDNS2)
	}
	if conf.WANProviderCapabilities == nil || conf.WANProviderCapabilities.UploadKilobitsPerSecond == nil ||
		*conf.WANProviderCapabilities.UploadKilobitsPerSecond != 1000 {
		t.Errorf("provider caps wrong: %+v", conf.WANProviderCapabilities)
	}

	// Narrowing the list to one server must explicitly clear the freed slot
	// (non-nil &"") so GET-merge-PUT overwrites the stale wan_dns2 rather than
	// leaving it — otherwise the controller echoes a perpetual extra server.
	m.DNSServers = sliceToStringList([]string{"1.1.1.1"})
	conf = m.toNetworkConf(ctx, &diags)
	if derefStr(conf.WANDNS1) != "1.1.1.1" || conf.WANDNS2 == nil || *conf.WANDNS2 != "" {
		t.Errorf("narrowed DNS must clear slot 2: dns1=%v dns2=%v", conf.WANDNS1, conf.WANDNS2)
	}

	// A null list (unconfigured) is left untouched — the controller keeps its
	// own preference rather than being forced to auto.
	m.DNSServers = sliceToStringList(nil)
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WANDNSPreference != "" || conf.WANDNS1 != nil {
		t.Errorf("null DNS must not touch the slots: pref=%q dns1=%v", conf.WANDNSPreference, conf.WANDNS1)
	}

	// Only upload set: download must round-trip back to null (not 0), or the
	// plan churns forever.
	m.UploadKilobits = types.Int64Value(1000)
	m.DownloadKilobits = types.Int64Null()
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WANProviderCapabilities == nil || conf.WANProviderCapabilities.DownloadKilobitsPerSecond != nil {
		t.Errorf("unset download must stay nil on the wire: %+v", conf.WANProviderCapabilities)
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

func TestDNSRecord_RoundTrip(t *testing.T) {
	m := &dnsRecordResourceModel{
		ID:       types.StringNull(),
		Name:     types.StringValue("host.example.com"),
		Type:     types.StringValue("A"),
		Value:    types.StringValue("10.0.0.5"),
		Enabled:  types.BoolValue(true),
		TTL:      types.Int64Null(),
		Port:     types.Int64Null(),
		Priority: types.Int64Null(),
		Weight:   types.Int64Null(),
	}
	rec := m.toRecord()
	if rec.Key != "host.example.com" || rec.RecordType != "A" || rec.Value != "10.0.0.5" {
		t.Errorf("toRecord wrong: %+v", rec)
	}
	if rec.Enabled == nil || !*rec.Enabled {
		t.Errorf("enabled should be &true")
	}
	if rec.TTL != nil || rec.Port != nil {
		t.Errorf("unset int optionals must be nil: ttl=%v port=%v", rec.TTL, rec.Port)
	}

	var out dnsRecordResourceModel
	ttl := 300
	out.fromRecord(&unifi.StaticDNSRecord{ID: "d1", Key: "k", RecordType: "CNAME", Value: "t", Enabled: boolPtrLit(false), TTL: &ttl})
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
	if conf.WANUsername != "user@isp" || conf.XWANPassword != "s3cret" {
		t.Errorf("pppoe creds wrong: %q / %q", conf.WANUsername, conf.XWANPassword)
	}
	if conf.WANPPPoEUsernameEnabled == nil || !*conf.WANPPPoEUsernameEnabled ||
		conf.WANPPPoEPasswordEnabled == nil || !*conf.WANPPPoEPasswordEnabled {
		t.Errorf("pppoe enable flags should be true when creds set")
	}
	if conf.MACOverride != "02:00:00:00:00:01" || conf.MACOverrideEnabled == nil || !*conf.MACOverrideEnabled {
		t.Errorf("mac override should be enabled with value: %+v", conf.MACOverrideEnabled)
	}

	// Static uplink fields.
	m = &wanResourceModel{
		Name: types.StringValue("wan"), WANType: types.StringValue("static"),
		IP: types.StringValue("203.0.113.2"), Netmask: types.StringValue("255.255.255.0"),
		Gateway: types.StringValue("203.0.113.1"),
	}
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WANIP != "203.0.113.2" || conf.WANNetmask != "255.255.255.0" || conf.WANGateway != "203.0.113.1" {
		t.Errorf("static WAN mapping wrong: %+v", conf)
	}

	// Write-only password: a read that doesn't echo it must keep the prior value.
	out := wanResourceModel{PPPoEPassword: types.StringValue("keep-me")}
	out.fromNetworkConf(&unifi.NetworkConf{Name: "wan", WANType: "pppoe", WANUsername: "u"})
	if out.PPPoEPassword.ValueString() != "keep-me" {
		t.Errorf("write-only password should be preserved on read, got %q", out.PPPoEPassword.ValueString())
	}
}

func TestNetwork_DHCPOptionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := &networkResourceModel{
		Name: types.StringValue("lan"), Purpose: types.StringValue("corporate"),
		DHCPLeaseTime:  types.Int64Value(86400),
		DHCPNTPEnabled: types.BoolValue(true), DHCPNTPServer1: types.StringValue("10.0.0.1"),
		DHCPGuardEnabled: types.BoolValue(true), MDNSEnabled: types.BoolValue(true),
	}
	conf := m.toNetworkConf(ctx, &diags)
	if conf.DHCPDLeaseTime == nil || *conf.DHCPDLeaseTime != 86400 {
		t.Errorf("lease time wrong: %v", conf.DHCPDLeaseTime)
	}
	if conf.DHCPDNTPEnabled == nil || !*conf.DHCPDNTPEnabled || conf.DHCPDNTP1 != "10.0.0.1" {
		t.Errorf("dhcp ntp wrong: %v / %q", conf.DHCPDNTPEnabled, conf.DHCPDNTP1)
	}
	if conf.DHCPGuardEnabled == nil || !*conf.DHCPGuardEnabled || conf.MDNSEnabled == nil || !*conf.MDNSEnabled {
		t.Errorf("guard/mdns flags wrong")
	}

	var out networkResourceModel
	out.fromNetworkConf(conf)
	if out.DHCPLeaseTime.ValueInt64() != 86400 || out.DHCPNTPServer1.ValueString() != "10.0.0.1" {
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
	}
	conf := m.toWLANConf(ctx, &diags)
	if diags.HasError() {
		t.Fatalf("toWLANConf diags: %v", diags)
	}
	if conf.Name != "Home" || conf.Security != "wpapsk" || conf.NetworkConfID != "net1" {
		t.Errorf("scalar mapping wrong: %+v", conf)
	}
	if conf.XPassphrase != "hunter2hunter2" {
		t.Errorf("passphrase mapping wrong: %q", conf.XPassphrase)
	}
	if conf.Enabled == nil || !*conf.Enabled {
		t.Errorf("enabled should be &true")
	}
	if len(conf.WLANBands) != 2 || len(conf.APGroupIDs) != 1 || len(conf.MACFilterList) != 1 {
		t.Errorf("set mapping wrong: bands=%v aps=%v macs=%v", conf.WLANBands, conf.APGroupIDs, conf.MACFilterList)
	}
	if conf.GroupRekey == nil || *conf.GroupRekey != 3600 {
		t.Errorf("group_rekey mapping wrong: %v", conf.GroupRekey)
	}

	// Wire → state. The controller commonly echoes an empty x_passphrase on
	// read; the passphrase must stay whatever the config set rather than wiping.
	out := *m // carry the configured passphrase
	out.fromWLANConf(&unifi.WLANConf{
		ID: "wlan9", Name: "Home", Security: "wpapsk", WPAMode: "wpa2",
		XPassphrase: "", NetworkConfID: "net1", WLANBands: []string{"2g", "5g"},
	})
	if out.ID.ValueString() != "wlan9" {
		t.Errorf("id round-trip wrong: %q", out.ID.ValueString())
	}
	if out.Passphrase.ValueString() != "hunter2hunter2" {
		t.Errorf("passphrase must be preserved when the controller echoes empty, got %q", out.Passphrase.ValueString())
	}

	// When the controller does return a passphrase, adopt it.
	out.fromWLANConf(&unifi.WLANConf{ID: "wlan9", Name: "Home", XPassphrase: "fromserver1234"})
	if out.Passphrase.ValueString() != "fromserver1234" {
		t.Errorf("passphrase should adopt the controller value, got %q", out.Passphrase.ValueString())
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
	if g.Name != "Upstairs" || g.DeviceMACs == nil || len(g.DeviceMACs) != 0 {
		t.Errorf("null device_macs must become an empty (non-nil) slice: %+v", g.DeviceMACs)
	}

	m.DeviceMACs = sliceToStringSet([]string{"aa:bb:cc:00:11:22", "de:ad:be:ef:00:01"})
	g = m.toAPGroup(ctx, &diags)
	if len(g.DeviceMACs) != 2 {
		t.Errorf("device_macs mapping wrong: %v", g.DeviceMACs)
	}

	var out apGroupResourceModel
	out.fromAPGroup(&unifi.APGroup{ID: "ag5", Name: "All APs", DeviceMACs: []string{"aa:bb:cc:00:11:22"}})
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
	out.fromAPGroup(&unifi.APGroup{ID: "default", Name: "All APs", AttrNoDelete: &protected})
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
	if g.Name != "Servers" || g.GroupType != "address-group" || g.GroupMembers == nil || len(g.GroupMembers) != 0 {
		t.Errorf("null members must become an empty (non-nil) slice: %+v", g)
	}

	m.Members = sliceToStringSet([]string{"10.0.0.10", "10.0.0.0/24"})
	g = m.toFirewallGroup(ctx, &diags)
	if len(g.GroupMembers) != 2 {
		t.Errorf("members mapping wrong: %v", g.GroupMembers)
	}

	var out firewallGroupResourceModel
	out.fromFirewallGroup(&unifi.FirewallGroup{ID: "fg5", Name: "Ports", GroupType: "port-group", GroupMembers: []string{"443"}})
	if out.ID.ValueString() != "fg5" || out.Type.ValueString() != "port-group" {
		t.Errorf("fromFirewallGroup wrong: %+v", out)
	}

	// An empty group (controller echoes `[]`) must read back as an empty set, not
	// null — otherwise a `members = []` config trips the post-apply consistency check.
	var empty firewallGroupResourceModel
	empty.fromFirewallGroup(&unifi.FirewallGroup{ID: "fg6", Name: "Empty", GroupType: "address-group", GroupMembers: []string{}})
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
	if r.Name != "block-iot-to-lan" || r.Ruleset != "LAN_IN" || r.Action != "drop" {
		t.Errorf("scalar mapping wrong: %+v", r)
	}
	if r.RuleIndex == nil || *r.RuleIndex != 20010 {
		t.Errorf("rule_index mapping wrong: %v", r.RuleIndex)
	}
	if r.Enabled == nil || !*r.Enabled || r.StateNew == nil || !*r.StateNew {
		t.Errorf("bool mapping wrong: enabled=%v state_new=%v", r.Enabled, r.StateNew)
	}
	// Unset optionals stay nil/empty so GET-merge-PUT leaves the controller's value.
	if r.SrcAddress != "" || r.ProtocolExcepted != nil || r.DstFirewallGroupIDs != nil {
		t.Errorf("unset optionals must be empty/nil: src_address=%q excepted=%v dst_groups=%v", r.SrcAddress, r.ProtocolExcepted, r.DstFirewallGroupIDs)
	}
	if len(r.SrcFirewallGroupIDs) != 1 || r.SrcFirewallGroupIDs[0] != "fg-iot" {
		t.Errorf("src_firewall_group_ids mapping wrong: %v", r.SrcFirewallGroupIDs)
	}
	if r.DstNetworkConfID != "net-lan" {
		t.Errorf("dst_network_id mapping wrong: %q", r.DstNetworkConfID)
	}

	var out firewallRuleResourceModel
	idx := 20010
	out.fromFirewallRule(&unifi.FirewallRule{
		ID: "fr9", Name: "block-iot-to-lan", Ruleset: "LAN_IN", RuleIndex: &idx, Action: "drop",
		Enabled: boolPtrLit(true), SettingPreference: "manual", DstNetworkConfID: "net-lan",
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
	rules := []unifi.FirewallRule{
		{ID: "a", Name: "first", Ruleset: "LAN_IN", RuleIndex: &i1},
		{ID: "b", Name: "second", Ruleset: "LAN_IN", RuleIndex: &i2},
		{ID: "c", Name: "wan", Ruleset: "WAN_IN", RuleIndex: &i1},
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
		IPAddress:             types.StringValue("10.0.0.10"),
		NetworkID:             types.StringValue("net1"),
		Name:                  types.StringValue("nas"),
		LocalDNSRecord:        types.StringNull(),
		LocalDNSRecordEnabled: types.BoolNull(),
	}
	u := m.toUser()
	if u.MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("mac should be lower-cased on the wire, got %q", u.MAC)
	}
	if u.UseFixedIP == nil || !*u.UseFixedIP {
		t.Errorf("toUser must always assert use_fixedip=true for a reservation")
	}
	if u.FixedIP != "10.0.0.10" || u.NetworkID != "net1" || u.Name != "nas" {
		t.Errorf("scalar mapping wrong: %+v", u)
	}

	// fromUser must NOT overwrite mac (immutable key, controller lowercases it),
	// so a mixed-case config doesn't churn.
	m.fromUser(&unifi.User{ID: "u9", MAC: "aa:bb:cc:dd:ee:01", FixedIP: "10.0.0.10", Hostname: "nas", NetworkID: "net1"})
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
		// leave the rest null so GET-merge-PUT preserves the controller's value
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
	if p.Name != "Trunk" || p.Forward != "customize" || p.NativeNetworkConfID != "net-mgmt" {
		t.Errorf("scalar mapping wrong: %+v", p)
	}
	// Explicit false on autoneg must be a non-nil *bool so the merge writes it.
	if p.Autoneg == nil || *p.Autoneg != false || p.STPPortMode == nil || *p.STPPortMode != false {
		t.Errorf("explicit false bool must be non-nil: autoneg=%v stp_port_mode=%v", p.Autoneg, p.STPPortMode)
	}
	if p.Speed == nil || *p.Speed != 1000 {
		t.Errorf("speed mapping wrong: %v", p.Speed)
	}
	if len(p.ExcludedNetworkConfIDs) != 1 || p.ExcludedNetworkConfIDs[0] != "net-iot" {
		t.Errorf("excluded_network_ids mapping wrong: %v", p.ExcludedNetworkConfIDs)
	}
	// Unset scalar optionals stay nil/empty so the merge-PUT leaves the controller's value.
	if p.FullDuplex != nil || p.Dot1XCtrl != "" {
		t.Errorf("unset scalar optionals must be empty/nil: full_duplex=%v dot1x=%q", p.FullDuplex, p.Dot1XCtrl)
	}
	// The owned MAC list, unset (null config), must send a concrete empty slice
	// (not nil) so an explicit `[]` clears it on the wire instead of reading null.
	if p.PortSecurityMACAddress == nil || len(p.PortSecurityMACAddress) != 0 {
		t.Errorf("owned port_security_mac_address must be a non-nil empty slice when unset, got %v", p.PortSecurityMACAddress)
	}

	// An explicit empty exclusion set must reach the wire as a concrete empty slice.
	m.ExcludedNetworkIDs = sliceToStringSet([]string{})
	if cleared := m.toPortProfile(ctx, &diags); cleared.ExcludedNetworkConfIDs == nil || len(cleared.ExcludedNetworkConfIDs) != 0 {
		t.Errorf("explicit empty excluded_network_ids must marshal as []; got %v", cleared.ExcludedNetworkConfIDs)
	}

	var out portProfileResourceModel
	out.fromPortProfile(&unifi.PortProfile{
		ID: "pc7", Name: "Uplink", Forward: "all", POEMode: "passthrough",
		Autoneg: boolPtrLit(true), ExcludedNetworkConfIDs: []string{},
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
