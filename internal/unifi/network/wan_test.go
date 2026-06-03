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

func TestWANSLAGetByName(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`[{"_id":"s1","name":"primary"},{"_id":"s2","name":"backup"}]`)}
	sla, err := newTestClient(f).WAN().SLAs().GetByName(context.Background(), "backup")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if sla.UnderscoreId == nil || *sla.UnderscoreId != "s2" {
		t.Fatalf("got %+v, want s2", sla)
	}
	if _, err := newTestClient(&fakeDoer{resp: okJSON(`[{"_id":"s1","name":"primary"}]`)}).
		WAN().SLAs().GetByName(context.Background(), "nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
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

func TestNetworksGetByName(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"Default"},{"_id":"n2","name":"WAN1"}]}`)}
	n, err := newTestClient(f).Networks().GetByName(context.Background(), "WAN1")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if n.UnderscoreId == nil || *n.UnderscoreId != "n2" {
		t.Fatalf("got %+v, want n2", n)
	}
	if _, err := newTestClient(&fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"Default"}]}`)}).
		Networks().GetByName(context.Background(), "Nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestNetworksUpdateSparse proves the write is a single sparse PUT of only the
// provider-owned fields — no GET-then-merge. The legacy /rest PUT is a
// server-side partial merge, so the controller keeps every field the body omits
// (controller-managed and unmodeled alike); the provider never fetches and
// re-sends the full document.
func TestNetworksUpdateSparse(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"LAN-renamed"}]}`)}
	_, err := newTestClient(f).Networks().Update(context.Background(),
		&wire.Networkconf{UnderscoreId: ptr("n1"), Name: ptr("LAN-renamed")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("sparse update should be a single PUT (no GET-merge), got %d calls", f.calls)
	}
	if f.method != "PUT" {
		t.Errorf("method = %q, want PUT", f.method)
	}
	if want := "/proxy/network/api/s/default/rest/networkconf/n1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	// The body is the typed wire object; omitempty drops the unset fields, so
	// only _id and the owned name are serialized — nothing the controller would
	// reject (no attr_no_delete, no echoed GET document).
	raw, _ := json.Marshal(f.body)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("PUT body not an object: %v (%s)", err, raw)
	}
	if len(got) != 2 || got["_id"] != "n1" || got["name"] != "LAN-renamed" {
		t.Errorf("PUT body = %s, want only {_id, name}", raw)
	}
}
