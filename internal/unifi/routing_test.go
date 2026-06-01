package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStaticRouteCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/routing", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"rt1","name":"to-lab","type":"static-route","static-route_network":"192.168.50.0/24","static-route_nexthop":"10.0.0.254","static-route_type":"nexthop-route","enabled":true}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/routing/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !strings.HasSuffix(r.URL.Path, "/rt1") {
				_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
				return
			}
			// gateway_type is unmodeled; it must survive the merge-PUT.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"rt1","name":"to-lab","type":"static-route","static-route_network":"192.168.50.0/24","static-route_nexthop":"10.0.0.254","static-route_type":"nexthop-route","gateway_type":"default","enabled":true}]}`))
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastPut)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[` + string(body) + `]}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	created, err := c.CreateStaticRoute(ctx, &StaticRoute{
		Name: "to-lab", Type: "static-route", Network: "192.168.50.0/24",
		NextHop: "10.0.0.254", RouteType: "nexthop-route", Enabled: bptr(true),
	})
	if err != nil || created.ID != "rt1" {
		t.Fatalf("CreateStaticRoute = %+v, err %v", created, err)
	}
	if created.Network != "192.168.50.0/24" || created.NextHop != "10.0.0.254" {
		t.Errorf("created route fields wrong: %+v", created)
	}

	if _, err := c.GetStaticRoute(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetStaticRoute(nope) err = %v, want ErrNotFound", err)
	}

	dist := 5
	if _, err := c.UpdateStaticRoute(ctx, &StaticRoute{ID: "rt1", Name: "to-lab", Network: "192.168.50.0/24", NextHop: "10.0.0.1", Distance: &dist}); err != nil {
		t.Fatalf("UpdateStaticRoute: %v", err)
	}
	if lastPut["static-route_nexthop"] != "10.0.0.1" || lastPut["static-route_distance"] != float64(5) {
		t.Errorf("PUT route fields wrong: %v", lastPut)
	}
	// GET-merge-PUT preserves the unmodeled gateway_type.
	if lastPut["gateway_type"] != "default" {
		t.Errorf("merge-PUT dropped unmodeled gateway_type; got %v", lastPut["gateway_type"])
	}

	if _, err := c.UpdateStaticRoute(ctx, &StaticRoute{Name: "x"}); err == nil {
		t.Errorf("UpdateStaticRoute with empty id should error")
	}

	if err := c.DeleteStaticRoute(ctx, "rt1"); err != nil {
		t.Fatalf("DeleteStaticRoute: %v", err)
	}
}

func TestPortForwardCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/portforward", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"pf1","name":"https","pfwd_interface":"wan","proto":"tcp","dst_port":"443","fwd":"10.0.0.5","fwd_port":"443","enabled":true}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/portforward/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"pf1","name":"https","pfwd_interface":"wan","proto":"tcp","dst_port":"443","fwd":"10.0.0.5","fwd_port":"443","enabled":true}]}`))
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastPut)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[` + string(body) + `]}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	created, err := c.CreatePortForward(ctx, &PortForward{
		Name: "https", PfwdInterface: "wan", Proto: "tcp", DstPort: "443", Fwd: "10.0.0.5", FwdPort: "443", Enabled: bptr(true),
	})
	if err != nil || created.ID != "pf1" {
		t.Fatalf("CreatePortForward = %+v, err %v", created, err)
	}
	if created.Fwd != "10.0.0.5" || created.DstPort != "443" {
		t.Errorf("created port-forward fields wrong: %+v", created)
	}

	if _, err := c.UpdatePortForward(ctx, &PortForward{ID: "pf1", Name: "https", Fwd: "10.0.0.6", DstPort: "8443"}); err != nil {
		t.Fatalf("UpdatePortForward: %v", err)
	}
	if lastPut["fwd"] != "10.0.0.6" || lastPut["dst_port"] != "8443" {
		t.Errorf("PUT port-forward fields wrong: %v", lastPut)
	}

	if _, err := c.UpdatePortForward(ctx, &PortForward{Name: "x"}); err == nil {
		t.Errorf("UpdatePortForward with empty id should error")
	}

	if err := c.DeletePortForward(ctx, "pf1"); err != nil {
		t.Fatalf("DeletePortForward: %v", err)
	}
}
