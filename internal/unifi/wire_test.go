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
