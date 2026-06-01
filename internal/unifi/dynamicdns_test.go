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

func TestDynamicDNSCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/dynamicdns", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"dd1","service":"dyndns","host_name":"home.example.com","login":"user","interface":"wan"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/dynamicdns/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !strings.HasSuffix(r.URL.Path, "/dd1") {
				_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"dd1","service":"dyndns","host_name":"home.example.com","login":"user","interface":"wan"}]}`))
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

	created, err := c.CreateDynamicDNS(ctx, &DynamicDNS{
		Service: "dyndns", HostName: "home.example.com", Login: "user", XPassword: "pw", Interface: "wan",
	})
	if err != nil || created.ID != "dd1" {
		t.Fatalf("CreateDynamicDNS = %+v, err %v", created, err)
	}
	if created.HostName != "home.example.com" || created.Service != "dyndns" {
		t.Errorf("created ddns fields wrong: %+v", created)
	}

	if _, err := c.GetDynamicDNS(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDynamicDNS(nope) err = %v, want ErrNotFound", err)
	}

	if _, err := c.UpdateDynamicDNS(ctx, &DynamicDNS{ID: "dd1", Service: "dyndns", HostName: "home2.example.com", XPassword: "newpw"}); err != nil {
		t.Fatalf("UpdateDynamicDNS: %v", err)
	}
	if lastPut["host_name"] != "home2.example.com" || lastPut["x_password"] != "newpw" {
		t.Errorf("PUT ddns fields wrong: %v", lastPut)
	}

	if _, err := c.UpdateDynamicDNS(ctx, &DynamicDNS{Service: "x"}); err == nil {
		t.Errorf("UpdateDynamicDNS with empty id should error")
	}

	if err := c.DeleteDynamicDNS(ctx, "dd1"); err != nil {
		t.Fatalf("DeleteDynamicDNS: %v", err)
	}
}

func TestWLANGroupCRUD(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/wlangroup", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wg1","name":"Default","attr_no_delete":true}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wg2","name":"Guest APs"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/wlangroup/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// attr_no_edit is controller-managed; it must survive the merge-PUT.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"wg2","name":"Guest APs","attr_no_edit":false}]}`))
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

	groups, err := c.ListWLANGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Name != "Default" {
		t.Fatalf("ListWLANGroups = %+v, err %v", groups, err)
	}
	if groups[0].AttrNoDelete == nil || !*groups[0].AttrNoDelete {
		t.Errorf("default group attr_no_delete = %v, want true", groups[0].AttrNoDelete)
	}

	created, err := c.CreateWLANGroup(ctx, &WLANGroup{Name: "Guest APs"})
	if err != nil || created.ID != "wg2" {
		t.Fatalf("CreateWLANGroup = %+v, err %v", created, err)
	}

	if _, err := c.UpdateWLANGroup(ctx, &WLANGroup{ID: "wg2", Name: "Renamed"}); err != nil {
		t.Fatalf("UpdateWLANGroup: %v", err)
	}
	if lastPut["name"] != "Renamed" {
		t.Errorf("PUT name = %v, want Renamed", lastPut["name"])
	}
	if _, ok := lastPut["attr_no_edit"]; !ok {
		t.Errorf("merge-PUT dropped controller-managed attr_no_edit; body = %v", lastPut)
	}

	if _, err := c.UpdateWLANGroup(ctx, &WLANGroup{Name: "x"}); err == nil {
		t.Errorf("UpdateWLANGroup with empty id should error")
	}

	if err := c.DeleteWLANGroup(ctx, "wg2"); err != nil {
		t.Fatalf("DeleteWLANGroup: %v", err)
	}
}
