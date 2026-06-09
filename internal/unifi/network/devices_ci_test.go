package network

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDevicesListUsesStatSurface(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","model":"US8P150"}]}`)}
	devs, err := newTestClient(f).Devices().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/proxy/network/api/s/default/stat/device"; f.path != want || f.method != "GET" {
		t.Errorf("got %s %s, want GET %s", f.method, f.path, want)
	}
	if len(devs) != 1 {
		t.Fatalf("devs = %d, want 1", len(devs))
	}
}

func TestDevicesGetByMAC(t *testing.T) {
	// GetByMAC matches case-insensitively on the typed wire.Device.Mac field
	// (a read-only stat field) while filtering the /stat/device list.
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"d0","mac":"00:00:00:00:00:00"},{"_id":"d1","mac":"AA:BB:CC:00:11:22","model":"US8"}]}`)}
	d, err := newTestClient(f).Devices().GetByMAC(context.Background(), "aa:bb:cc:00:11:22")
	if err != nil {
		t.Fatalf("GetByMAC: %v", err)
	}
	if d.UnderscoreId == nil || *d.UnderscoreId != "d1" {
		t.Fatalf("matched device = %+v, want _id d1", d)
	}
	if d.Mac == nil || *d.Mac != "AA:BB:CC:00:11:22" {
		t.Fatalf("matched device mac = %v, want AA:BB:CC:00:11:22", d.Mac)
	}
}

func TestDevicesGetByMACNotFound(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"AA:BB:CC:00:11:22"}]}`)}
	if _, err := newTestClient(f).Devices().GetByMAC(context.Background(), "de:ad:be:ef:00:00"); err != ErrNotFound {
		t.Fatalf("GetByMAC err = %v, want ErrNotFound", err)
	}
}

func TestDevicesAdoptPostsCmd(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[]}`)}
	if err := newTestClient(f).Devices().Adopt(context.Background(), "AA:BB:CC:00:11:22"); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if want := "/proxy/network/api/s/default/cmd/devmgr"; f.path != want || f.method != "POST" {
		t.Errorf("got %s %s, want POST %s", f.method, f.path, want)
	}
	got, _ := json.Marshal(f.body)
	if !strings.Contains(string(got), `"cmd":"adopt"`) || !strings.Contains(string(got), `"mac":"aa:bb:cc:00:11:22"`) {
		t.Errorf("cmd body = %s, want adopt + lowercased mac", got)
	}
}

func TestDevicesForgetPostsCmd(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[]}`)}
	if err := newTestClient(f).Devices().Forget(context.Background(), "AA:BB:CC:00:11:22"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	got, _ := json.Marshal(f.body)
	if !strings.Contains(string(got), `"cmd":"delete-device"`) || !strings.Contains(string(got), `"mac":"aa:bb:cc:00:11:22"`) {
		t.Errorf("cmd body = %s, want delete-device + lowercased mac", got)
	}
}

func TestPortOverridesOf(t *testing.T) {
	// wire.Device.PortOverrides round-trips into the curated subset: matching keys
	// map across, the int-backed speed enum and string enums serialize to their
	// values, and unmodeled keys (qos_profile, etc.) drop out.
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","port_overrides":[
		{"port_idx":1,"forward":"native","poe_mode":"auto","speed":1000,"autoneg":false,"setting_preference":"manual"}
	]}]}`)}
	d, err := newTestClient(f).Devices().Get(context.Background(), "d1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	pos, err := PortOverridesOf(d)
	if err != nil {
		t.Fatalf("PortOverridesOf: %v", err)
	}
	if len(pos) != 1 {
		t.Fatalf("pos = %d, want 1", len(pos))
	}
	po := pos[0]
	if po.PortIdx != 1 || po.Forward != "native" || po.PoEMode != "auto" {
		t.Errorf("string fields wrong: %+v", po)
	}
	if po.Speed == nil || *po.Speed != 1000 {
		t.Errorf("int-backed speed enum should round-trip to *int 1000, got %v", po.Speed)
	}
	if po.Autoneg == nil || *po.Autoneg {
		t.Errorf("autoneg should be &false, got %v", po.Autoneg)
	}
}

func TestUpdateConfig_PortOverrideMerge(t *testing.T) {
	// Device d1 has overrides on ports 1, 2, 3, each with setting_preference — an
	// unmodeled key the merge must preserve.
	stat := `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","port_overrides":[
		{"port_idx":1,"poe_mode":"off","setting_preference":"manual"},
		{"port_idx":2,"name":"uplink","setting_preference":"auto"},
		{"port_idx":3,"poe_mode":"auto","setting_preference":"auto"}
	]}]}`
	// Flow: GET /stat (portOverridesRaw) → PUT /rest/device/d1 → GET /stat (re-read).
	f := fakeQueue(stat, `{"meta":{"rc":"ok"},"data":[{"_id":"d1"}]}`, stat)

	// Manage port 1 (set its name; keep its other keys); port 2 was managed last
	// cycle but is dropped from desired → reverted; port 3 is unmanaged → preserved.
	cfg := DeviceConfig{Name: "core-switch", PortOverrides: []PortOverride{{PortIdx: 1, Name: "wan-handoff"}}}
	if _, err := newTestClient(f).Devices().UpdateConfig(context.Background(), "d1", cfg, []int{1, 2}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// The PUT is the 2nd call; inspect its body in wire form.
	raw, _ := json.Marshal(f.bodies[1])
	var putBody map[string]any
	if err := json.Unmarshal(raw, &putBody); err != nil {
		t.Fatalf("decode put body: %v", err)
	}
	if putBody["name"] != "core-switch" {
		t.Errorf("top-level name not sent: %v", putBody["name"])
	}
	po, ok := putBody["port_overrides"].([]any)
	if !ok {
		t.Fatalf("port_overrides missing/!array: %v", putBody["port_overrides"])
	}
	byIdx := map[float64]map[string]any{}
	for _, e := range po {
		m := e.(map[string]any)
		byIdx[m["port_idx"].(float64)] = m
	}
	// Port 1: name overlaid, but poe_mode + setting_preference preserved.
	if p1 := byIdx[1]; p1 == nil || p1["name"] != "wan-handoff" || p1["poe_mode"] != "off" || p1["setting_preference"] != "manual" {
		t.Errorf("port 1 merge wrong: %v", byIdx[1])
	}
	// Port 2: managed-then-dropped → must be absent (reverted).
	if _, present := byIdx[2]; present {
		t.Errorf("port 2 was dropped from desired and should be reverted (absent), got %v", byIdx[2])
	}
	// Port 3: unmanaged → preserved verbatim.
	if p3 := byIdx[3]; p3 == nil || p3["poe_mode"] != "auto" || p3["setting_preference"] != "auto" {
		t.Errorf("port 3 should be preserved untouched: %v", byIdx[3])
	}
}

func TestUpdateConfig_NoPortsLeavesArrayUntouched(t *testing.T) {
	// Scalar-only update: no port management → no pre-PUT /stat fetch, and
	// port_overrides is never sent.
	f := fakeQueue(`{"meta":{"rc":"ok"},"data":[{"_id":"d1"}]}`, `{"meta":{"rc":"ok"},"data":[{"_id":"d1","led_override":"off"}]}`)
	if _, err := newTestClient(f).Devices().UpdateConfig(context.Background(), "d1", DeviceConfig{LEDOverride: "off"}, nil); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	// A scalar-only update skips the port-table prefetch, so the first wire body
	// is the PUT itself — carrying led_override and no port_overrides.
	raw, _ := json.Marshal(f.bodies[0])
	var putBody map[string]any
	if err := json.Unmarshal(raw, &putBody); err != nil {
		t.Fatalf("decode put body: %v", err)
	}
	if putBody["led_override"] != "off" {
		t.Errorf("led_override not sent first: %v", putBody)
	}
	if _, ok := putBody["port_overrides"]; ok {
		t.Errorf("port_overrides must be untouched when no ports are managed; got %v", putBody["port_overrides"])
	}
}

// regression test for #101 — RestoreConfiguredScalars overlays the verbatim-stored
// fields (name, profile binding, and the VLAN *intent*: native/voice networks and
// the tagged-VLAN mode) over a lagging read-back, but leaves `forward` untouched —
// the controller DERIVES forward from the intent, so it is modeled Computed-only and
// a stale read of it can't trip the consistency check. Field dispositions verified
// live + against the controller source — see the func comment.
func TestRestoreConfiguredScalars(t *testing.T) {
	// Read lagged the write and echoes the pre-write values: the old name, no profile
	// binding, the controller's previously-derived forward, and the old VLAN intent.
	read := PortOverride{PortIdx: 1, Name: "old", Forward: "all", NativeNetworkConfID: "netDefault", TaggedVLANMgmt: "auto"}
	// The user configured a new name, a profile binding, and new VLAN intent (native +
	// voice networks, custom tagged mode). `forward` is Computed — never user-set —
	// but we plant a value here to prove restore ignores it.
	want := PortOverride{PortIdx: 1, Name: "new", PortconfID: "prof1", Forward: "customize", NativeNetworkConfID: "netServers", VoiceNetworkConfID: "netVoice", TaggedVLANMgmt: "custom"}
	read.RestoreConfiguredScalars(want)

	if read.Name != "new" {
		t.Errorf("configured Name should win over the lagged read: got %q, want %q", read.Name, "new")
	}
	if read.PortconfID != "prof1" {
		t.Errorf("configured PortconfID should be restored when the read drops it: got %q", read.PortconfID)
	}
	if read.NativeNetworkConfID != "netServers" {
		t.Errorf("native_networkconf_id is verbatim VLAN intent and should be restored: got %q, want %q", read.NativeNetworkConfID, "netServers")
	}
	if read.VoiceNetworkConfID != "netVoice" {
		t.Errorf("voice_networkconf_id is verbatim VLAN intent and should be restored: got %q, want %q", read.VoiceNetworkConfID, "netVoice")
	}
	if read.TaggedVLANMgmt != "custom" {
		t.Errorf("tagged_vlan_mgmt is a verbatim input and should be restored: got %q, want %q", read.TaggedVLANMgmt, "custom")
	}
	if read.Forward != "all" {
		t.Errorf("forward is controller-derived and must NOT be restored: got %q, want the read-back %q", read.Forward, "all")
	}
}

// Issue #108 — a disabled switch port carries no direct "disable" field. The
// controller DERIVES forward=disabled from tagged_vlan_mgmt=block_all with an
// EXPLICITLY empty native VLAN; sending native absent makes the controller inject
// the default network and derive forward=native instead (the live failure that
// motivated this). So the write MUST put native="" literally on the wire — which
// the struct's omitempty can't, hence applyDerivedDisable. This asserts the merged
// PUT body for a Disabled port is exactly {block_all, native:""}.
func TestUpdateConfig_PortDisableDerivesBlockAllEmptyNative(t *testing.T) {
	// Port 5 currently has a native VLAN — the case where naive block_all would
	// keep it and derive forward=native, not disabled.
	stat := `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","port_overrides":[
		{"port_idx":5,"native_networkconf_id":"netA","tagged_vlan_mgmt":"auto","forward":"native"}
	]}]}`
	f := fakeQueue(stat, `{"meta":{"rc":"ok"},"data":[{"_id":"d1"}]}`, stat)

	disable := true
	cfg := DeviceConfig{PortOverrides: []PortOverride{{PortIdx: 5, Disabled: &disable}}}
	if _, err := newTestClient(f).Devices().UpdateConfig(context.Background(), "d1", cfg, []int{5}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// The PUT is the 2nd wire call. Decode the raw body (not via PortOverride — we
	// must see whether native_networkconf_id is PRESENT as "", which omitempty hides).
	raw, _ := json.Marshal(f.bodies[1])
	var putBody struct {
		PortOverrides []map[string]json.RawMessage `json:"port_overrides"`
	}
	if err := json.Unmarshal(raw, &putBody); err != nil {
		t.Fatalf("decode put body: %v", err)
	}
	var p5 map[string]json.RawMessage
	for _, e := range putBody.PortOverrides {
		var idx int
		_ = json.Unmarshal(e["port_idx"], &idx)
		if idx == 5 {
			p5 = e
		}
	}
	if p5 == nil {
		t.Fatalf("port 5 missing from PUT: %s", raw)
	}
	if got, ok := p5["tagged_vlan_mgmt"]; !ok || string(got) != `"block_all"` {
		t.Errorf("tagged_vlan_mgmt = %s (present=%v), want \"block_all\"", got, ok)
	}
	got, ok := p5["native_networkconf_id"]
	if !ok {
		t.Fatalf("native_networkconf_id absent — controller would inject the default network and derive forward=native, NOT disabled")
	}
	if string(got) != `""` {
		t.Errorf("native_networkconf_id = %s, want \"\" (explicit empty)", got)
	}
}

// Issue #108 — an unmanaged disable (nil) leaves a port's fields untouched.
func TestApplyDerivedDisable_NoopWhenUnmanaged(t *testing.T) {
	doc := map[string]json.RawMessage{"port_idx": json.RawMessage(`3`), "native_networkconf_id": json.RawMessage(`"netA"`)}
	applyDerivedDisable(doc, PortOverride{PortIdx: 3}) // Disabled nil
	if string(doc["native_networkconf_id"]) != `"netA"` {
		t.Errorf("unmanaged port native VLAN was altered: %s", doc["native_networkconf_id"])
	}
	if _, ok := doc["tagged_vlan_mgmt"]; ok {
		t.Errorf("unmanaged port should not gain tagged_vlan_mgmt: %v", doc["tagged_vlan_mgmt"])
	}
}

// Issue #108 — disabled=false re-enables ONLY a port the controller currently has
// disabled (and only when the user manages no VLAN inline): it reverts to the
// default trunk (auto + native omitted → controller injects default → forward=all).
// An already-enabled port, or one whose VLAN the user owns, is left untouched.
func TestApplyDerivedDisable_ReenableOnlyWhenDisabled(t *testing.T) {
	enable := false
	t.Run("disabled port reverts to default trunk", func(t *testing.T) {
		doc := map[string]json.RawMessage{"port_idx": json.RawMessage(`5`), "forward": json.RawMessage(`"disabled"`),
			"tagged_vlan_mgmt": json.RawMessage(`"block_all"`), "native_networkconf_id": json.RawMessage(`""`)}
		applyDerivedDisable(doc, PortOverride{PortIdx: 5, Disabled: &enable})
		if string(doc["tagged_vlan_mgmt"]) != `"auto"` {
			t.Errorf("tagged_vlan_mgmt = %s, want \"auto\"", doc["tagged_vlan_mgmt"])
		}
		if _, ok := doc["native_networkconf_id"]; ok {
			t.Errorf("native must be omitted so the controller injects the default: %s", doc["native_networkconf_id"])
		}
	})
	t.Run("already-enabled port left untouched", func(t *testing.T) {
		doc := map[string]json.RawMessage{"port_idx": json.RawMessage(`6`), "forward": json.RawMessage(`"native"`),
			"tagged_vlan_mgmt": json.RawMessage(`"auto"`), "native_networkconf_id": json.RawMessage(`"netA"`)}
		applyDerivedDisable(doc, PortOverride{PortIdx: 6, Disabled: &enable})
		if string(doc["native_networkconf_id"]) != `"netA"` {
			t.Errorf("an enabled port's native VLAN must be preserved, got %s", doc["native_networkconf_id"])
		}
	})
}

func TestClientsGetByMAC(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"u1","mac":"de:ad:be:ef:00:01","name":"printer"}]}`)}
	u, err := newTestClient(f).Clients().GetByMAC(context.Background(), "DE:AD:BE:EF:00:01")
	if err != nil {
		t.Fatalf("GetByMAC: %v", err)
	}
	if want := "/proxy/network/api/s/default/rest/user"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if u.UnderscoreId == nil || *u.UnderscoreId != "u1" {
		t.Fatalf("user = %+v, want _id u1", u)
	}
}
