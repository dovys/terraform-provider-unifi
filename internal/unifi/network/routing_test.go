package network

import (
	"context"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

func TestStaticRoutesList(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"r1","name":"to-lab","static-route_network":"10.9.0.0/24"}]}`)}
	routes, err := newTestClient(f).StaticRoutes().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/proxy/network/api/s/default/rest/routing"; f.path != want || f.method != "GET" {
		t.Errorf("got %s %s, want GET %s", f.method, f.path, want)
	}
	if len(routes) != 1 || routes[0].Name == nil || *routes[0].Name != "to-lab" {
		t.Fatalf("routes = %+v, want one to-lab", routes)
	}
}

func TestStaticRoutesUpdateRequiresID(t *testing.T) {
	f := &fakeDoer{}
	if _, err := newTestClient(f).StaticRoutes().Update(context.Background(), &wire.Routing{Name: ptr("x")}); err == nil {
		t.Fatal("Update without _id should error")
	}
	if f.calls != 0 {
		t.Errorf("no request expected, got %d", f.calls)
	}
}
