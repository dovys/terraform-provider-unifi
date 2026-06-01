package models

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
