package network

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

func TestWANMagicPause(t *testing.T) {
	f := &fakeDoer{resp: okJSON("")}
	if err := newTestClient(f).WAN().MagicPause(context.Background()); err != nil {
		t.Fatalf("MagicPause: %v", err)
	}
	if f.method != "PUT" {
		t.Errorf("method = %q, want PUT", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/wan/magic/pause"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
}

func TestWANListNetworkGroups(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"wan_network_groups":[{"name":"WAN","network_id":"n1","priority":1},{"name":"WAN2","network_id":"n2"}]}`)}
	groups, err := newTestClient(f).WAN().ListNetworkGroups(context.Background())
	if err != nil {
		t.Fatalf("ListNetworkGroups: %v", err)
	}
	if f.method != "GET" {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/wan/networkgroups"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(groups) != 2 || groups[0].Name == nil || *groups[0].Name != "WAN" {
		t.Fatalf("groups = %+v, want [WAN, WAN2] unwrapped", groups)
	}
}

func TestWANSetMagicConfigWireBytes(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"enabled":true}`)}
	_, err := newTestClient(f).WAN().SetMagicConfig(context.Background(), &wire.WanMagicConfiguration{Enabled: ptr(true)})
	if err != nil {
		t.Fatalf("SetMagicConfig: %v", err)
	}
	if f.method != "PUT" {
		t.Errorf("method = %q, want PUT", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/wan/magic/configuration"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	got, _ := json.Marshal(f.body)
	if !strings.Contains(string(got), `"enabled":true`) {
		t.Errorf("body missing enabled: %s", got)
	}
	if strings.Contains(string(got), `"data_usage_limit_alert"`) {
		t.Errorf("nil fields must be omitted, got: %s", got)
	}
}

func TestWANSLAUpdateSendsID(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"_id":"s1","name":"primary"}`)}
	_, err := newTestClient(f).WAN().SLAs().Update(context.Background(),
		&wire.WanSlas{UnderscoreId: ptr("s1"), Name: ptr("primary")})
	if err != nil {
		t.Fatalf("SLA Update: %v", err)
	}
	if want := "/proxy/network/v2/api/site/default/wan-slas/s1"; f.path != want || f.method != "PUT" {
		t.Errorf("got %s %s, want PUT %s", f.method, f.path, want)
	}
}

func TestNetworksList(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"LAN","purpose":"corporate"}]}`)}
	nets, err := newTestClient(f).Networks().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/proxy/network/api/s/default/rest/networkconf"; f.path != want || f.method != "GET" {
		t.Errorf("got %s %s, want GET %s", f.method, f.path, want)
	}
	if len(nets) != 1 || nets[0].Name == nil || *nets[0].Name != "LAN" {
		t.Fatalf("nets = %+v, want one LAN", nets)
	}
}

// TestNetworksUpdateMerge proves GET-merge-PUT: the live doc is fetched, the
// owned fields overlaid, and the merged document PUT back — controller-managed
// fields the provider doesn't model (here attr_no_delete) survive.
func TestNetworksUpdateMerge(t *testing.T) {
	f := fakeQueue(
		`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"LAN","attr_no_delete":true,"vlan":10}]}`,
		`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"LAN-renamed","attr_no_delete":true,"vlan":10}]}`,
	)
	_, err := newTestClient(f).Networks().Update(context.Background(),
		&wire.Networkconf{UnderscoreId: ptr("n1"), Name: ptr("LAN-renamed")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.calls != 2 {
		t.Fatalf("merge should GET then PUT (2 calls), got %d", f.calls)
	}
	if f.method != "PUT" {
		t.Errorf("last call = %q, want PUT", f.method)
	}
	if want := "/proxy/network/api/s/default/rest/networkconf/n1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	doc, ok := f.body.(map[string]json.RawMessage)
	if !ok {
		t.Fatalf("PUT body should be the merged raw doc, got %T", f.body)
	}
	if string(doc["name"]) != `"LAN-renamed"` {
		t.Errorf("owned field not overlaid: name = %s", doc["name"])
	}
	if string(doc["attr_no_delete"]) != "true" {
		t.Errorf("unmodeled controller field not preserved: attr_no_delete = %s", doc["attr_no_delete"])
	}
}
