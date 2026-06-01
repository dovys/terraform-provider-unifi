package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestDeviceReadAndLookup(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/stat/device", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[
			{"_id":"d1","mac":"aa:bb:cc:00:00:01","name":"sw","type":"usw","adopted":true,"state":1},
			{"_id":"d2","mac":"AA:BB:CC:00:00:02","type":"uap","adopted":false}
		]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	devs, err := c.ListDevices(ctx)
	if err != nil || len(devs) != 2 {
		t.Fatalf("ListDevices = %+v, err %v", devs, err)
	}

	// MAC lookup is case-insensitive.
	got, err := c.GetDeviceByMAC(ctx, "aa:bb:cc:00:00:02")
	if err != nil || got.ID != "d2" {
		t.Fatalf("GetDeviceByMAC = %+v, err %v", got, err)
	}
	if got.Adopted == nil || *got.Adopted {
		t.Errorf("d2 should be unadopted, got %v", got.Adopted)
	}
	if _, err := c.GetDeviceByMAC(ctx, "ff:ff:ff:ff:ff:ff"); err != ErrNotFound {
		t.Errorf("unknown MAC should be ErrNotFound, got %v", err)
	}
	if _, err := c.GetDevice(ctx, "nope"); err != ErrNotFound {
		t.Errorf("unknown id should be ErrNotFound, got %v", err)
	}
}

func TestUpdateDevice_PortOverrideMerge(t *testing.T) {
	var putBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/stat/device", func(w http.ResponseWriter, r *http.Request) {
		// Device d1 has overrides on ports 1, 2, 3. Each carries setting_preference,
		// an unmodeled key the merge must preserve.
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","port_overrides":[
			{"port_idx":1,"poe_mode":"off","setting_preference":"manual"},
			{"port_idx":2,"name":"uplink","setting_preference":"auto"},
			{"port_idx":3,"poe_mode":"auto","setting_preference":"auto"}
		]}]}`))
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/device/d1", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &putBody)
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01"}]}`))
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	// Manage port 1 (set name; keep its other keys) and port 2 (was managed last
	// cycle) is dropped from desired → reverted. Port 3 is unmanaged → preserved.
	dev := &Device{ID: "d1", Name: "core-switch", PortOverrides: []PortOverride{
		{PortIdx: 1, Name: "wan-handoff"},
	}}
	if _, err := c.UpdateDevice(ctx, dev, []int{1, 2}); err != nil {
		t.Fatalf("UpdateDevice: %v", err)
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

func TestUpdateDevice_NoPortsLeavesArrayUntouched(t *testing.T) {
	var putBody map[string]any
	var statHits int
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/rest/device/d1", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &putBody)
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"d1"}]}`))
	})
	mux.HandleFunc("/proxy/network/api/s/default/stat/device", func(w http.ResponseWriter, r *http.Request) {
		statHits++
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"d1","led_override":"off"}]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	if _, err := c.UpdateDevice(context.Background(), &Device{ID: "d1", LEDOverride: "off"}, nil); err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	if putBody["led_override"] != "off" {
		t.Errorf("led_override not sent: %v", putBody["led_override"])
	}
	if _, ok := putBody["port_overrides"]; ok {
		t.Errorf("port_overrides must be untouched when no ports are managed; got %v", putBody["port_overrides"])
	}
	// One /stat/device hit: the authoritative re-read (not a pre-PUT override fetch,
	// which is skipped when no ports are managed).
	if statHits != 1 {
		t.Errorf("expected exactly one /stat/device read-back, got %d", statHits)
	}
}

func TestAdoptAndForgetDevice(t *testing.T) {
	var cmds []string
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/cmd/devmgr", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		cmds = append(cmds, body["cmd"].(string)+":"+body["mac"].(string))
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	if err := c.AdoptDevice(ctx, "AA:BB:CC:00:00:01"); err != nil {
		t.Fatalf("AdoptDevice: %v", err)
	}
	if err := c.ForgetDevice(ctx, "AA:BB:CC:00:00:01"); err != nil {
		t.Fatalf("ForgetDevice: %v", err)
	}
	// Both commands carry a lower-cased MAC.
	if len(cmds) != 2 || cmds[0] != "adopt:aa:bb:cc:00:00:01" || cmds[1] != "delete-device:aa:bb:cc:00:00:01" {
		t.Errorf("devmgr commands wrong: %v", cmds)
	}
}
