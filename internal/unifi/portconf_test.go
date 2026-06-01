package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestPortProfileCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/portconf", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"pc1","name":"Trunk","forward":"all"}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"pc1","name":"Trunk","forward":"all","poe_mode":"auto"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/portconf/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// attr_no_edit is unmodeled; it must survive the merge-PUT.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"pc1","name":"Trunk","forward":"all","poe_mode":"auto","attr_no_edit":true}]}`))
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

	profiles, err := c.ListPortProfiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].Forward != "all" {
		t.Fatalf("ListPortProfiles = %+v, err %v", profiles, err)
	}

	created, err := c.CreatePortProfile(ctx, &PortProfile{Name: "Trunk", Forward: "all", POEMode: "auto"})
	if err != nil || created.ID != "pc1" {
		t.Fatalf("CreatePortProfile = %+v, err %v", created, err)
	}

	on := true
	if _, err := c.UpdatePortProfile(ctx, &PortProfile{ID: "pc1", Name: "Uplink", Forward: "native", STPPortMode: &on}); err != nil {
		t.Fatalf("UpdatePortProfile: %v", err)
	}
	// GET-merge-PUT preserves the unmodeled attr_no_edit and overwrites owned fields.
	if lastPut["attr_no_edit"] != true {
		t.Errorf("merge-PUT dropped unmodeled attr_no_edit; got %v", lastPut["attr_no_edit"])
	}
	if lastPut["name"] != "Uplink" || lastPut["forward"] != "native" {
		t.Errorf("PUT owned fields wrong: name=%v forward=%v", lastPut["name"], lastPut["forward"])
	}
	if lastPut["stp_port_mode"] != true {
		t.Errorf("PUT stp_port_mode = %v, want true", lastPut["stp_port_mode"])
	}

	if err := c.DeletePortProfile(ctx, "pc1"); err != nil {
		t.Fatalf("DeletePortProfile: %v", err)
	}
}
