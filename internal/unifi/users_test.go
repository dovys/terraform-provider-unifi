package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestUserCRUD_AndFindByMAC(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/user", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Controller stores MACs lower-case; the list backs GetUserByMAC.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"u1","mac":"aa:bb:cc:dd:ee:01","hostname":"nas"}]}`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[` + string(body) + `]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/user/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"u1","mac":"aa:bb:cc:dd:ee:01","hostname":"nas","oui":"Synology"}]}`))
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastPut)
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[` + string(body) + `]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	// GetUserByMAC is case-insensitive.
	got, err := c.GetUserByMAC(ctx, "AA:BB:CC:DD:EE:01")
	if err != nil || got.ID != "u1" {
		t.Fatalf("GetUserByMAC = %+v, err %v", got, err)
	}
	if _, err := c.GetUserByMAC(ctx, "00:00:00:00:00:99"); err != ErrNotFound {
		t.Errorf("GetUserByMAC for an unknown MAC should be ErrNotFound, got %v", err)
	}

	on := true
	created, err := c.CreateUser(ctx, &User{MAC: "aa:bb:cc:dd:ee:02", UseFixedIP: &on, FixedIP: "10.0.0.20"})
	if err != nil || created.FixedIP != "10.0.0.20" {
		t.Fatalf("CreateUser = %+v, err %v", created, err)
	}

	// Update is GET-merge-PUT: the unmodeled oui must survive, use_fixedip is set.
	if _, err := c.UpdateUser(ctx, &User{ID: "u1", MAC: "aa:bb:cc:dd:ee:01", UseFixedIP: &on, FixedIP: "10.0.0.10"}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if lastPut["oui"] != "Synology" {
		t.Errorf("GET-merge-PUT dropped unmodeled oui; got %v", lastPut["oui"])
	}
	if lastPut["use_fixedip"] != true || lastPut["fixed_ip"] != "10.0.0.10" {
		t.Errorf("reservation fields wrong on PUT: %v / %v", lastPut["use_fixedip"], lastPut["fixed_ip"])
	}

	// Revert (destroy): only use_fixedip=false rides along; mac must not be cleared.
	off := false
	if _, err := c.UpdateUser(ctx, &User{ID: "u1", UseFixedIP: &off}); err != nil {
		t.Fatalf("revert UpdateUser: %v", err)
	}
	if lastPut["use_fixedip"] != false {
		t.Errorf("revert should set use_fixedip=false; got %v", lastPut["use_fixedip"])
	}
	if lastPut["mac"] != "aa:bb:cc:dd:ee:01" {
		t.Errorf("revert must preserve the fetched mac (omitempty), got %v", lastPut["mac"])
	}
}
