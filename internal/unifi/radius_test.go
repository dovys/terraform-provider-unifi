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

func TestRadiusProfileCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/radiusprofile", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"rp1","name":"Default","attr_no_delete":true,"attr_no_edit":true}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"rp2","name":"Corp","auth_servers":[{"ip":"10.0.0.9","port":1812,"x_secret":"s3cret"}]}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/radiusprofile/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !strings.HasSuffix(r.URL.Path, "/rp2") {
				_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
				return
			}
			// attr_no_edit is controller-managed; it must survive the merge-PUT.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"rp2","name":"Corp","auth_servers":[{"ip":"10.0.0.9","port":1812,"x_secret":"s3cret"}],"attr_no_edit":false}]}`))
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

	profiles, err := c.ListRadiusProfiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].Name != "Default" {
		t.Fatalf("ListRadiusProfiles = %+v, err %v", profiles, err)
	}

	created, err := c.CreateRadiusProfile(ctx, &RadiusProfile{
		Name:        "Corp",
		AuthServers: []RadiusServer{{IP: "10.0.0.9", Port: 1812, XSecret: "s3cret"}},
	})
	if err != nil || created.ID != "rp2" {
		t.Fatalf("CreateRadiusProfile = %+v, err %v", created, err)
	}
	if len(created.AuthServers) != 1 || created.AuthServers[0].IP != "10.0.0.9" {
		t.Errorf("created auth_servers wrong: %+v", created.AuthServers)
	}

	if _, err := c.GetRadiusProfile(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRadiusProfile(nope) err = %v, want ErrNotFound", err)
	}

	// Update: replace the server list (full-replace) and flip the gateway toggle.
	if _, err := c.UpdateRadiusProfile(ctx, &RadiusProfile{
		ID:               "rp2",
		Name:             "Corp",
		AuthServers:      []RadiusServer{{IP: "10.0.0.10", Port: 1812, XSecret: "new"}},
		AcctServers:      []RadiusServer{},
		UseUSGAuthServer: bptr(false),
	}); err != nil {
		t.Fatalf("UpdateRadiusProfile: %v", err)
	}
	// GET-merge-PUT overwrites owned fields, preserves controller-managed ones.
	if lastPut["name"] != "Corp" {
		t.Errorf("PUT name = %v, want Corp", lastPut["name"])
	}
	if _, ok := lastPut["attr_no_edit"]; !ok {
		t.Errorf("merge-PUT dropped controller-managed attr_no_edit; body = %v", lastPut)
	}
	servers, ok := lastPut["auth_servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("PUT auth_servers wrong: %v", lastPut["auth_servers"])
	}
	if s := servers[0].(map[string]any); s["ip"] != "10.0.0.10" || s["x_secret"] != "new" {
		t.Errorf("PUT auth server entry wrong: %v", s)
	}
	// acct_servers is full-replace (non-omitempty): an explicit empty list must
	// reach the wire as [] so the servers are cleared.
	if acct, ok := lastPut["acct_servers"].([]any); !ok || len(acct) != 0 {
		t.Errorf("PUT acct_servers should be []; got %v", lastPut["acct_servers"])
	}

	if _, err := c.UpdateRadiusProfile(ctx, &RadiusProfile{Name: "x"}); err == nil {
		t.Errorf("UpdateRadiusProfile with empty id should error")
	}

	if err := c.DeleteRadiusProfile(ctx, "rp2"); err != nil {
		t.Fatalf("DeleteRadiusProfile: %v", err)
	}
}

func TestRadiusAccountCRUD(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/account", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"ac1","name":"alice","tunnel_type":13,"tunnel_medium_type":6,"vlan":"30"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/account/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"ac1","name":"alice","tunnel_type":13,"tunnel_medium_type":6,"vlan":"30"}]}`))
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

	tt, tm := 13, 6
	created, err := c.CreateRadiusAccount(ctx, &RadiusAccount{
		Name: "alice", XPassword: "pw", TunnelType: &tt, TunnelMediumType: &tm, VLAN: "30",
	})
	if err != nil || created.ID != "ac1" {
		t.Fatalf("CreateRadiusAccount = %+v, err %v", created, err)
	}
	if created.TunnelType == nil || *created.TunnelType != 13 || created.VLAN != "30" {
		t.Errorf("created account wrong: %+v", created)
	}

	if _, err := c.UpdateRadiusAccount(ctx, &RadiusAccount{ID: "ac1", Name: "alice", XPassword: "newpw", VLAN: "40"}); err != nil {
		t.Fatalf("UpdateRadiusAccount: %v", err)
	}
	if lastPut["x_password"] != "newpw" || lastPut["vlan"] != "40" {
		t.Errorf("PUT account fields wrong: %v", lastPut)
	}

	if err := c.DeleteRadiusAccount(ctx, "ac1"); err != nil {
		t.Fatalf("DeleteRadiusAccount: %v", err)
	}
}
