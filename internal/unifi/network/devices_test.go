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

func TestDevicesGetByMACScansRawStat(t *testing.T) {
	// wire.Device omits the read-only "mac" field, so GetByMAC matches on the
	// raw stat document then decodes the typed device.
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"AA:BB:CC:00:11:22","model":"US8"}]}`)}
	d, err := newTestClient(f).Devices().GetByMAC(context.Background(), "aa:bb:cc:00:11:22")
	if err != nil {
		t.Fatalf("GetByMAC: %v", err)
	}
	if d.UnderscoreId == nil || *d.UnderscoreId != "d1" {
		t.Fatalf("matched device = %+v, want _id d1", d)
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
	// Exactly two calls: the PUT then the authoritative re-read (no override prefetch).
	if f.calls != 2 {
		t.Fatalf("want 2 calls (PUT + re-read), got %d", f.calls)
	}
	// The first call is the PUT: it carries led_override and no port_overrides.
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
