package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestWLANCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/wlanconf", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wlan1","name":"Home"}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wlan1","name":"Home","security":"wpapsk"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/wlanconf/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Existing doc carries a controller-managed field we don't own.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wlan1","name":"Home","x_iapp_key":"keepme","networkconf_id":"net1"}]}`))
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

	created, err := c.CreateWLAN(ctx, &WLANConf{Name: "Home", Security: "wpapsk"})
	if err != nil {
		t.Fatalf("CreateWLAN: %v", err)
	}
	if created.ID != "wlan1" {
		t.Errorf("created id = %q, want wlan1", created.ID)
	}

	if _, err := c.UpdateWLAN(ctx, &WLANConf{ID: "wlan1", Name: "Home2"}); err != nil {
		t.Fatalf("UpdateWLAN: %v", err)
	}
	// GET-merge-PUT must preserve the unmodeled field and overwrite the owned one.
	if lastPut["x_iapp_key"] != "keepme" {
		t.Errorf("PUT dropped unmodeled x_iapp_key; got %v", lastPut["x_iapp_key"])
	}
	if lastPut["name"] != "Home2" {
		t.Errorf("PUT name = %v, want Home2", lastPut["name"])
	}

	if err := c.DeleteWLAN(ctx, "wlan1"); err != nil {
		t.Fatalf("DeleteWLAN: %v", err)
	}
}

func TestAPGroupCRUD_V2(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/v2/api/site/default/apgroups", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"_id":"ag1","name":"All APs","device_macs":["aa:bb:cc:00:11:22"],"attr_no_delete":true}]`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"_id":"ag2","name":"Upstairs","device_macs":[]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/apgroups/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastPut)
			_, _ = w.Write(body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	groups, err := c.ListAPGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Name != "All APs" {
		t.Fatalf("ListAPGroups = %+v, err %v", groups, err)
	}

	created, err := c.CreateAPGroup(ctx, &APGroup{Name: "Upstairs", DeviceMACs: []string{}})
	if err != nil || created.ID != "ag2" {
		t.Fatalf("CreateAPGroup = %+v, err %v", created, err)
	}

	if _, err := c.UpdateAPGroup(ctx, &APGroup{ID: "ag2", Name: "Upstairs", DeviceMACs: []string{"de:ad:be:ef:00:01"}}); err != nil {
		t.Fatalf("UpdateAPGroup: %v", err)
	}
	// v2 write contract: _id rides in the body, not just the URL.
	if lastPut["_id"] != "ag2" {
		t.Errorf("v2 PUT body missing _id; got %v", lastPut["_id"])
	}

	if err := c.DeleteAPGroup(ctx, "ag2"); err != nil {
		t.Fatalf("DeleteAPGroup: %v", err)
	}
}
