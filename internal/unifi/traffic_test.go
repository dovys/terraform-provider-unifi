package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestTrafficRuleCRUD(t *testing.T) {
	var lastBody map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/v2/api/site/default/trafficrules", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"_id":"tr1","action":"BLOCK","matching_target":"DOMAIN","domains":["ads.example.com"],"enabled":true,"target_devices":[{"type":"ALL_CLIENTS"}]}]`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"tr1","action":"BLOCK","matching_target":"DOMAIN","domains":["ads.example.com"],"enabled":true,"target_devices":[{"type":"ALL_CLIENTS"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/trafficrules/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"tr1","action":"SPEED_LIMIT","matching_target":"DOMAIN","enabled":true}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	created, err := c.CreateTrafficRule(ctx, &TrafficRule{
		Action: "BLOCK", MatchingTarget: "DOMAIN", Domains: []string{"ads.example.com"},
		Enabled: bptr(true), TargetDevices: []TrafficTargetDevice{{Type: "ALL_CLIENTS"}},
	})
	if err != nil || created.ID != "tr1" {
		t.Fatalf("CreateTrafficRule = %+v, err %v", created, err)
	}
	if lastBody["action"] != "BLOCK" || lastBody["matching_target"] != "DOMAIN" {
		t.Errorf("POST body wrong: %v", lastBody)
	}
	// target_devices must serialize as a list of objects.
	td, ok := lastBody["target_devices"].([]any)
	if !ok || len(td) != 1 {
		t.Fatalf("target_devices wrong: %v", lastBody["target_devices"])
	}
	if td[0].(map[string]any)["type"] != "ALL_CLIENTS" {
		t.Errorf("target device type wrong: %v", td[0])
	}

	got, err := c.GetTrafficRule(ctx, "tr1")
	if err != nil || got.Action != "BLOCK" || len(got.Domains) != 1 {
		t.Fatalf("GetTrafficRule(tr1) = %+v, err %v", got, err)
	}
	if _, err := c.GetTrafficRule(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTrafficRule(nope) err = %v, want ErrNotFound", err)
	}

	dn := 5000
	up := 1000
	on := true
	if _, err := c.UpdateTrafficRule(ctx, &TrafficRule{
		ID: "tr1", Action: "SPEED_LIMIT", MatchingTarget: "DOMAIN",
		BandwidthLimit: &TrafficBandwidthLimit{Enabled: &on, DownloadLimitKbps: &dn, UploadLimitKbps: &up},
	}); err != nil {
		t.Fatalf("UpdateTrafficRule: %v", err)
	}
	// v2 write contract: _id rides in the body as well as the URL.
	if lastBody["_id"] != "tr1" {
		t.Errorf("v2 PUT body missing _id; got %v", lastBody["_id"])
	}
	bl, ok := lastBody["bandwidth_limit"].(map[string]any)
	if !ok || bl["download_limit_kbps"] != float64(5000) {
		t.Errorf("bandwidth_limit wrong: %v", lastBody["bandwidth_limit"])
	}

	if _, err := c.UpdateTrafficRule(ctx, &TrafficRule{Action: "BLOCK"}); err == nil {
		t.Errorf("UpdateTrafficRule with empty id should error")
	}

	if err := c.DeleteTrafficRule(ctx, "tr1"); err != nil {
		t.Fatalf("DeleteTrafficRule: %v", err)
	}
}

func TestTrafficRouteCRUD(t *testing.T) {
	var lastBody map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/v2/api/site/default/trafficroutes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"_id":"rr1","matching_target":"DOMAIN","domains":["netflix.com"],"network_id":"vpn1","enabled":true}]`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"rr1","matching_target":"DOMAIN","domains":["netflix.com"],"network_id":"vpn1","enabled":true}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/trafficroutes/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"rr1","matching_target":"DOMAIN","network_id":"vpn2","enabled":true}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	created, err := c.CreateTrafficRoute(ctx, &TrafficRoute{
		MatchingTarget: "DOMAIN", Domains: []string{"netflix.com"}, NetworkID: "vpn1", Enabled: bptr(true),
	})
	if err != nil || created.ID != "rr1" || created.NetworkID != "vpn1" {
		t.Fatalf("CreateTrafficRoute = %+v, err %v", created, err)
	}

	if _, err := c.GetTrafficRoute(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTrafficRoute(nope) err = %v, want ErrNotFound", err)
	}

	if _, err := c.UpdateTrafficRoute(ctx, &TrafficRoute{ID: "rr1", MatchingTarget: "DOMAIN", NetworkID: "vpn2"}); err != nil {
		t.Fatalf("UpdateTrafficRoute: %v", err)
	}
	if lastBody["_id"] != "rr1" || lastBody["network_id"] != "vpn2" {
		t.Errorf("v2 PUT body wrong: %v", lastBody)
	}

	if _, err := c.UpdateTrafficRoute(ctx, &TrafficRoute{MatchingTarget: "IP"}); err == nil {
		t.Errorf("UpdateTrafficRoute with empty id should error")
	}

	if err := c.DeleteTrafficRoute(ctx, "rr1"); err != nil {
		t.Fatalf("DeleteTrafficRoute: %v", err)
	}
}
