package unifi

import (
	"encoding/json"
	"testing"
)

// toMap marshals v and decodes it back into a generic map so tests can assert
// exactly which keys hit the wire (omitempty behavior, three-state pointers).
func toMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestStaticDNSRecord_Wire(t *testing.T) {
	// A minimal record: only the three required fields go on the wire; optional
	// pointers stay off so the controller keeps its defaults.
	m := toMap(t, StaticDNSRecord{Key: "host.example.com", RecordType: "A", Value: "10.0.0.5"})
	for _, k := range []string{"key", "record_type", "value"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing required key %q in %v", k, m)
		}
	}
	for _, k := range []string{"_id", "enabled", "ttl", "port", "priority", "weight"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// An explicit false on a pointer field must be sent (distinct from unset).
	f := false
	m = toMap(t, StaticDNSRecord{Key: "h", RecordType: "A", Value: "1.1.1.1", Enabled: &f})
	if v, ok := m["enabled"]; !ok || v != false {
		t.Errorf("enabled=&false should marshal to enabled:false; got %v (present=%v)", v, ok)
	}
}

func TestNetworkConf_Wire(t *testing.T) {
	m := toMap(t, NetworkConf{Name: "trusted", Purpose: "corporate"})
	if m["name"] != "trusted" || m["purpose"] != "corporate" {
		t.Errorf("name/purpose wrong: %v", m)
	}
	// Every optional pointer/string is omitempty — none should appear unset.
	for _, k := range []string{"_id", "site_id", "vlan_enabled", "vlan", "ip_subnet", "dhcpd_enabled", "enabled", "wan_type", "attr_no_delete"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// VLAN-enabled network: explicit false/true pointers and the VLAN id ride along.
	on := true
	vid := 30
	m = toMap(t, NetworkConf{Name: "iot", Purpose: "corporate", VLANEnabled: &on, VLAN: &vid})
	if m["vlan_enabled"] != true {
		t.Errorf("vlan_enabled = %v, want true", m["vlan_enabled"])
	}
	if m["vlan"] != float64(30) { // JSON numbers decode to float64
		t.Errorf("vlan = %v, want 30", m["vlan"])
	}
}

func TestWANProviderCapabilities_Wire(t *testing.T) {
	up, down := 1000, 50000
	m := toMap(t, NetworkConf{
		Name: "wan", Purpose: "wan",
		WANProviderCapabilities: &WANProviderCapabilities{UploadKilobitsPerSecond: &up, DownloadKilobitsPerSecond: &down},
	})
	caps, ok := m["wan_provider_capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("wan_provider_capabilities not an object: %v", m["wan_provider_capabilities"])
	}
	if caps["upload_kilobits_per_second"] != float64(1000) || caps["download_kilobits_per_second"] != float64(50000) {
		t.Errorf("provider caps wrong: %v", caps)
	}

	// Only one direction set: the other must be omitted (not sent as 0).
	m = toMap(t, NetworkConf{
		Name: "wan", Purpose: "wan",
		WANProviderCapabilities: &WANProviderCapabilities{UploadKilobitsPerSecond: &up},
	})
	caps = m["wan_provider_capabilities"].(map[string]any)
	if _, ok := caps["download_kilobits_per_second"]; ok {
		t.Errorf("unset download must be omitted; got %v", caps["download_kilobits_per_second"])
	}
}

func TestWLANConf_Wire(t *testing.T) {
	// A minimal SSID: only name (non-omitempty) goes on the wire; every optional
	// pointer/string/slice stays off so GET-merge-PUT keeps the controller's value.
	m := toMap(t, WLANConf{Name: "Home"})
	if m["name"] != "Home" {
		t.Errorf("name = %v, want Home", m["name"])
	}
	for _, k := range []string{
		"_id", "site_id", "enabled", "networkconf_id", "security", "x_passphrase",
		"wpa3_support", "wlan_bands", "ap_group_ids", "mac_filter_enabled", "is_guest", "group_rekey",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// Explicit false on a pointer field must be sent (distinct from unset).
	f := false
	m = toMap(t, WLANConf{Name: "Home", Enabled: &f, HideSSID: &f})
	if v, ok := m["enabled"]; !ok || v != false {
		t.Errorf("enabled=&false should marshal to enabled:false; got %v (present=%v)", v, ok)
	}
	if v, ok := m["hide_ssid"]; !ok || v != false {
		t.Errorf("hide_ssid=&false should marshal to hide_ssid:false; got %v (present=%v)", v, ok)
	}
}

func TestAPGroup_Wire(t *testing.T) {
	// device_macs is intentionally non-omitempty: an empty group still sends [] so
	// clearing the members actually empties the group. Everything else is omitempty.
	m := toMap(t, APGroup{Name: "Upstairs", DeviceMACs: []string{}})
	if m["name"] != "Upstairs" {
		t.Errorf("name = %v, want Upstairs", m["name"])
	}
	if v, ok := m["device_macs"]; !ok {
		t.Errorf("device_macs must always be sent (non-omitempty); got %v", m)
	} else if arr, ok := v.([]any); !ok || len(arr) != 0 {
		t.Errorf("device_macs should marshal to []; got %v", v)
	}
	for _, k := range []string{"_id", "site_id", "for_wlanconf", "attr_no_delete", "attr_hidden_id"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}
}

func TestFirewallGroup_Wire(t *testing.T) {
	// group_members is non-omitempty: an empty group still sends [].
	m := toMap(t, FirewallGroup{Name: "Servers", GroupType: "address-group", GroupMembers: []string{}})
	if m["name"] != "Servers" || m["group_type"] != "address-group" {
		t.Errorf("name/group_type wrong: %v", m)
	}
	if v, ok := m["group_members"]; !ok {
		t.Errorf("group_members must always be sent (non-omitempty); got %v", m)
	} else if arr, ok := v.([]any); !ok || len(arr) != 0 {
		t.Errorf("group_members should marshal to []; got %v", v)
	}
	if _, ok := m["_id"]; ok {
		t.Errorf("unset _id must be omitted; got %v", m["_id"])
	}
}

func TestFirewallRule_Wire(t *testing.T) {
	// A minimal rule: required fields ride along; every optional pointer/string is
	// omitempty so GET-merge-PUT leaves the controller's value.
	idx := 20000
	m := toMap(t, FirewallRule{Name: "block", Ruleset: "LAN_IN", RuleIndex: &idx, Action: "drop"})
	if m["name"] != "block" || m["ruleset"] != "LAN_IN" || m["action"] != "drop" || m["rule_index"] != float64(20000) {
		t.Errorf("required fields wrong: %v", m)
	}
	for _, k := range []string{
		"_id", "site_id", "enabled", "protocol", "logging", "ipsec",
		"state_new", "src_address", "dst_port", "src_firewallgroup_ids", "setting_preference",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// Explicit false on a state pointer must be sent (distinct from unset).
	f := false
	m = toMap(t, FirewallRule{Name: "x", Ruleset: "LAN_IN", RuleIndex: &idx, Action: "accept", Logging: &f, StateNew: &f})
	if v, ok := m["logging"]; !ok || v != false {
		t.Errorf("logging=&false should marshal to logging:false; got %v (present=%v)", v, ok)
	}
	if v, ok := m["state_new"]; !ok || v != false {
		t.Errorf("state_new=&false should marshal to state_new:false; got %v (present=%v)", v, ok)
	}
}

func TestUser_Wire(t *testing.T) {
	// A reservation: mac + use_fixedip + fixed_ip ride along; telemetry/optional
	// fields stay off so GET-merge-PUT leaves the controller's value.
	on := true
	m := toMap(t, User{MAC: "aa:bb:cc:dd:ee:01", UseFixedIP: &on, FixedIP: "10.0.0.10", NetworkID: "net1"})
	if m["mac"] != "aa:bb:cc:dd:ee:01" || m["use_fixedip"] != true || m["fixed_ip"] != "10.0.0.10" {
		t.Errorf("reservation fields wrong: %v", m)
	}
	for _, k := range []string{"_id", "site_id", "name", "local_dns_record", "local_dns_record_enabled", "hostname"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A revert PUT carries use_fixedip=false but NOT mac (omitempty), so the
	// merge can't clobber the controller's stored MAC.
	off := false
	m = toMap(t, User{ID: "u1", UseFixedIP: &off})
	if v, ok := m["use_fixedip"]; !ok || v != false {
		t.Errorf("use_fixedip=&false should marshal to false; got %v (present=%v)", v, ok)
	}
	if _, ok := m["mac"]; ok {
		t.Errorf("empty mac must be omitted so a revert PUT preserves it; got %v", m["mac"])
	}
}

func TestUserGroup_Wire(t *testing.T) {
	// name always rides along; the rate caps and attr_no_delete are omitempty so a
	// minimal group (no caps) leaves the controller's defaults untouched.
	m := toMap(t, UserGroup{Name: "Default"})
	if m["name"] != "Default" {
		t.Errorf("name = %v, want Default", m["name"])
	}
	for _, k := range []string{"_id", "site_id", "qos_rate_max_up", "qos_rate_max_down", "attr_no_delete"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A capped group sends both rate fields, including an explicit 0 (the
	// controller's unlimited sentinel is distinct from unset).
	zero := 0
	up := 5000
	m = toMap(t, UserGroup{Name: "Capped", QOSRateMaxUp: &up, QOSRateMaxDown: &zero})
	if m["qos_rate_max_up"] != float64(5000) {
		t.Errorf("qos_rate_max_up = %v, want 5000", m["qos_rate_max_up"])
	}
	if v, ok := m["qos_rate_max_down"]; !ok || v != float64(0) {
		t.Errorf("qos_rate_max_down=&0 should marshal to 0; got %v (present=%v)", v, ok)
	}
}

func TestRadiusProfile_Wire(t *testing.T) {
	// name always rides along; the optional scalars/pointers are omitempty so a
	// minimal profile leaves the controller's defaults untouched. The two server
	// lists are NON-omitempty (full-replace), so they always appear.
	m := toMap(t, RadiusProfile{Name: "Corp"})
	if m["name"] != "Corp" {
		t.Errorf("name = %v, want Corp", m["name"])
	}
	for _, k := range []string{"auth_servers", "acct_servers"} {
		if _, ok := m[k]; !ok {
			t.Errorf("full-replace list %q must always be present (non-omitempty); got %v", k, m)
		}
	}
	for _, k := range []string{"_id", "site_id", "use_usg_auth_server", "use_usg_acct_server", "interim_update_enabled", "interim_update_interval", "vlan_enabled", "vlan_wlan_mode", "attr_no_delete", "attr_no_edit"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A populated profile: the nested server object carries ip/port/x_secret, and
	// an explicit false on the gateway toggle is sent (distinct from unset).
	f := false
	m = toMap(t, RadiusProfile{
		Name:             "Corp",
		AuthServers:      []RadiusServer{{IP: "10.0.0.9", Port: 1812, XSecret: "s"}},
		AcctServers:      []RadiusServer{},
		UseUSGAuthServer: &f,
	})
	servers, ok := m["auth_servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("auth_servers should be a 1-element list; got %v", m["auth_servers"])
	}
	s := servers[0].(map[string]any)
	if s["ip"] != "10.0.0.9" || s["port"] != float64(1812) || s["x_secret"] != "s" {
		t.Errorf("nested server wrong: %v", s)
	}
	if acct, ok := m["acct_servers"].([]any); !ok || len(acct) != 0 {
		t.Errorf("explicit empty acct_servers should marshal to []; got %v", m["acct_servers"])
	}
	if v, ok := m["use_usg_auth_server"]; !ok || v != false {
		t.Errorf("use_usg_auth_server=&false should marshal to false; got %v (present=%v)", v, ok)
	}
}

func TestRadiusAccount_Wire(t *testing.T) {
	// name always rides along; password/tunnel/vlan are omitempty so a minimal
	// account stays minimal.
	m := toMap(t, RadiusAccount{Name: "alice"})
	if m["name"] != "alice" {
		t.Errorf("name = %v, want alice", m["name"])
	}
	for _, k := range []string{"_id", "site_id", "x_password", "tunnel_type", "tunnel_medium_type", "vlan"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A dynamic-VLAN account: password + tunnel attrs + vlan ride along.
	tt, tm := 13, 6
	m = toMap(t, RadiusAccount{Name: "alice", XPassword: "pw", TunnelType: &tt, TunnelMediumType: &tm, VLAN: "30"})
	if m["x_password"] != "pw" || m["tunnel_type"] != float64(13) || m["vlan"] != "30" {
		t.Errorf("account fields wrong: %v", m)
	}
}

func TestStaticRoute_Wire(t *testing.T) {
	// name always rides along; every optional string/pointer is omitempty so a
	// minimal route leaves the controller's defaults untouched.
	m := toMap(t, StaticRoute{Name: "to-lab"})
	if m["name"] != "to-lab" {
		t.Errorf("name = %v, want to-lab", m["name"])
	}
	for _, k := range []string{"_id", "site_id", "enabled", "type", "static-route_network", "static-route_nexthop", "static-route_distance", "static-route_type", "static-route_interface"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A nexthop route: the hyphenated controller keys and an explicit distance 0
	// (a valid admin distance, distinct from unset) ride along.
	zero := 0
	on := true
	m = toMap(t, StaticRoute{
		Name: "to-lab", Enabled: &on, Type: "static-route",
		Network: "192.168.50.0/24", NextHop: "10.0.0.254", RouteType: "nexthop-route", Distance: &zero,
	})
	if m["static-route_network"] != "192.168.50.0/24" || m["static-route_nexthop"] != "10.0.0.254" {
		t.Errorf("route keys wrong: %v", m)
	}
	if m["type"] != "static-route" || m["static-route_type"] != "nexthop-route" {
		t.Errorf("type discriminators wrong: %v", m)
	}
	if v, ok := m["static-route_distance"]; !ok || v != float64(0) {
		t.Errorf("distance=&0 should marshal to 0; got %v (present=%v)", v, ok)
	}
}

func TestPortForward_Wire(t *testing.T) {
	// name always rides along; the optional scalars/pointers are omitempty.
	m := toMap(t, PortForward{Name: "https"})
	if m["name"] != "https" {
		t.Errorf("name = %v, want https", m["name"])
	}
	for _, k := range []string{"_id", "site_id", "enabled", "pfwd_interface", "proto", "src", "dst_port", "fwd", "fwd_port", "log"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A populated rule: an explicit false log is sent (distinct from unset).
	f := false
	on := true
	m = toMap(t, PortForward{
		Name: "https", Enabled: &on, PfwdInterface: "wan", Proto: "tcp",
		DstPort: "443", Fwd: "10.0.0.5", FwdPort: "443", Log: &f,
	})
	if m["dst_port"] != "443" || m["fwd"] != "10.0.0.5" || m["proto"] != "tcp" {
		t.Errorf("forward fields wrong: %v", m)
	}
	if v, ok := m["log"]; !ok || v != false {
		t.Errorf("log=&false should marshal to false; got %v (present=%v)", v, ok)
	}
}

func TestDynamicDNS_Wire(t *testing.T) {
	// Every field is omitempty (no non-omitempty key), so an empty struct sends
	// nothing; a populated one sends exactly the set keys.
	m := toMap(t, DynamicDNS{})
	for _, k := range []string{"_id", "site_id", "service", "host_name", "server", "login", "x_password", "interface"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	m = toMap(t, DynamicDNS{Service: "dyndns", HostName: "home.example.com", Login: "u", XPassword: "pw", Interface: "wan"})
	if m["service"] != "dyndns" || m["host_name"] != "home.example.com" || m["x_password"] != "pw" {
		t.Errorf("ddns fields wrong: %v", m)
	}
}

func TestWLANGroup_Wire(t *testing.T) {
	// name always rides along; the protection flags are omitempty.
	m := toMap(t, WLANGroup{Name: "Guest APs"})
	if m["name"] != "Guest APs" {
		t.Errorf("name = %v, want Guest APs", m["name"])
	}
	for _, k := range []string{"_id", "site_id", "attr_no_delete", "attr_no_edit"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}
}

func TestTrafficRule_Wire(t *testing.T) {
	// action/matching_target ride along; every criteria slice and nested object is
	// omitempty so a minimal rule sends only what's set (v2 bare-JSON surface).
	m := toMap(t, TrafficRule{Action: "BLOCK", MatchingTarget: "INTERNET"})
	if m["action"] != "BLOCK" || m["matching_target"] != "INTERNET" {
		t.Errorf("action/matching_target wrong: %v", m)
	}
	for _, k := range []string{"_id", "enabled", "domains", "ip_addresses", "ip_ranges", "regions", "app_ids", "app_category_ids", "network_ids", "target_devices", "bandwidth_limit", "description"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// A speed-limit rule: nested target_devices + bandwidth_limit serialize as objects.
	on := true
	dn := 5000
	m = toMap(t, TrafficRule{
		Action: "SPEED_LIMIT", MatchingTarget: "DOMAIN", Domains: []string{"x.example.com"},
		TargetDevices:  []TrafficTargetDevice{{Type: "CLIENT", ClientMAC: "aa:bb:cc:00:11:22"}},
		BandwidthLimit: &TrafficBandwidthLimit{Enabled: &on, DownloadLimitKbps: &dn},
	})
	td := m["target_devices"].([]any)
	if td[0].(map[string]any)["client_mac"] != "aa:bb:cc:00:11:22" {
		t.Errorf("target_devices nested wrong: %v", td)
	}
	bl := m["bandwidth_limit"].(map[string]any)
	if bl["download_limit_kbps"] != float64(5000) {
		t.Errorf("bandwidth_limit nested wrong: %v", bl)
	}
	// upload omitted (nil pointer), download present.
	if _, ok := bl["upload_limit_kbps"]; ok {
		t.Errorf("unset upload_limit_kbps must be omitted; got %v", bl["upload_limit_kbps"])
	}
}

func TestTrafficRoute_Wire(t *testing.T) {
	m := toMap(t, TrafficRoute{MatchingTarget: "DOMAIN", Domains: []string{"netflix.com"}, NetworkID: "vpn1"})
	if m["matching_target"] != "DOMAIN" || m["network_id"] != "vpn1" {
		t.Errorf("route fields wrong: %v", m)
	}
	for _, k := range []string{"_id", "enabled", "ip_addresses", "ip_ranges", "regions", "target_devices", "kill_switch_enabled", "description"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}
}

func TestPortProfile_Wire(t *testing.T) {
	// name always rides along; the scalar optionals are omitempty so GET-merge-PUT
	// leaves the controller's value untouched.
	m := toMap(t, PortProfile{Name: "Trunk", Forward: "all"})
	if m["name"] != "Trunk" || m["forward"] != "all" {
		t.Errorf("name/forward wrong: %v", m)
	}
	for _, k := range []string{
		"_id", "site_id", "poe_mode", "autoneg", "speed", "stp_port_mode",
		"stp_edge_state", "native_networkconf_id", "multicast_router_mode",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// The two owned lists are NON-omitempty: an explicit empty set must serialize
	// to [] so a merge-PUT clears the value (mirrors FirewallGroup.GroupMembers).
	m = toMap(t, PortProfile{Name: "x", ExcludedNetworkConfIDs: []string{}, PortSecurityMACAddress: []string{}})
	for _, k := range []string{"excluded_networkconf_ids", "port_security_mac_address"} {
		v, ok := m[k]
		if !ok {
			t.Errorf("owned list %q must always be sent; got absent", k)
			continue
		}
		if arr, ok := v.([]any); !ok || len(arr) != 0 {
			t.Errorf("empty owned list %q should marshal to []; got %v", k, v)
		}
	}

	// Explicit false on a bool pointer must be sent (distinct from unset).
	f := false
	m = toMap(t, PortProfile{Name: "x", Autoneg: &f, STPPortMode: &f})
	if v, ok := m["autoneg"]; !ok || v != false {
		t.Errorf("autoneg=&false should marshal to false; got %v (present=%v)", v, ok)
	}
	if v, ok := m["stp_port_mode"]; !ok || v != false {
		t.Errorf("stp_port_mode=&false should marshal to false; got %v (present=%v)", v, ok)
	}
}

func TestDevice_Wire(t *testing.T) {
	// A config PUT carries only owned fields; telemetry stays off. mac is omitempty
	// so a config write can't blank the natural key.
	m := toMap(t, Device{ID: "d1", Name: "sw", LEDOverride: "off"})
	if m["name"] != "sw" || m["led_override"] != "off" {
		t.Errorf("owned fields wrong: %v", m)
	}
	for _, k := range []string{"mac", "site_id", "adopted", "model", "type", "version", "ip", "state", "port_overrides"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}
}

func TestPortOverride_Wire(t *testing.T) {
	// port_idx is non-omitempty (the map key), even for port 0; every other field
	// is omitempty so a per-port merge leaves unmodeled keys alone.
	m := toMap(t, PortOverride{PortIdx: 0, Name: "p0"})
	if v, ok := m["port_idx"]; !ok || v != float64(0) {
		t.Errorf("port_idx must always be sent (incl. 0); got %v (present=%v)", v, ok)
	}
	for _, k := range []string{"portconf_id", "poe_mode", "forward", "autoneg", "speed", "stp_port_mode", "excluded_networkconf_ids"} {
		if _, ok := m[k]; ok {
			t.Errorf("unset key %q must be omitted; got %v", k, m[k])
		}
	}

	// Explicit false on a bool pointer is sent (distinct from unset).
	f := false
	m = toMap(t, PortOverride{PortIdx: 5, Autoneg: &f})
	if v, ok := m["autoneg"]; !ok || v != false {
		t.Errorf("autoneg=&false should marshal to false; got %v (present=%v)", v, ok)
	}
}
