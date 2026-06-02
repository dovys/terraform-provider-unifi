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
