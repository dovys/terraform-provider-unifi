package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

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

	for _, name := range []string{"unifi_network_network", "unifi_network_wan", "unifi_network_dns_record"} {
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
	if conf.DHCPDDNS1 != "1.1.1.1" || conf.DHCPDDNS2 != "8.8.8.8" {
		t.Errorf("dhcp dns mapping wrong: %q %q", conf.DHCPDDNS1, conf.DHCPDDNS2)
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
	out.fromNetworkConf(&models.NetworkConf{
		ID: "net9", Name: "guests", Purpose: "guest", NetworkGroup: "LAN",
		VLANEnabled: &on, VLAN: &vid, DHCPDDNS1: "9.9.9.9",
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
		DNSServers:       sliceToStringList([]string{"1.1.1.1"}),
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
	if conf.WANDNSPreference != "manual" || conf.WANDNS1 != "1.1.1.1" {
		t.Errorf("manual DNS mapping wrong: pref=%q dns1=%q", conf.WANDNSPreference, conf.WANDNS1)
	}
	if conf.WANProviderCapabilities == nil || conf.WANProviderCapabilities.UploadKilobitsPerSecond == nil ||
		*conf.WANProviderCapabilities.UploadKilobitsPerSecond != 1000 {
		t.Errorf("provider caps wrong: %+v", conf.WANProviderCapabilities)
	}

	// No DNS servers → automatic.
	m.DNSServers = sliceToStringList(nil)
	conf = m.toNetworkConf(ctx, &diags)
	if conf.WANDNSPreference != "auto" || conf.WANDNS1 != "" {
		t.Errorf("auto DNS mapping wrong: pref=%q dns1=%q", conf.WANDNSPreference, conf.WANDNS1)
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
	out.fromRecord(&models.StaticDNSRecord{ID: "d1", Key: "k", RecordType: "CNAME", Value: "t", Enabled: boolPtrLit(false), TTL: &ttl})
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
	out.fromNetworkConf(&models.NetworkConf{Name: "wan", WANType: "pppoe", WANUsername: "u"})
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
