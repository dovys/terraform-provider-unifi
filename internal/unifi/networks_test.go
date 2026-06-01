package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// registerProbe wires the auth + capability-probe endpoints NewClient needs.
func registerProbe(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: fakeToken, Path: "/"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/proxy/network/api/s/default/stat/sysinfo", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"10.4.57"}]}`))
	})
}

func newTestClient(t *testing.T, mux *http.ServeMux) (Client, *httptest.Server) {
	t.Helper()
	registerProbe(mux)
	srv := httptest.NewServer(mux)
	c, err := NewClient(context.Background(), Config{
		Host: srv.URL, Username: "admin", Password: "pw",
	})
	if err != nil {
		srv.Close()
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func TestNetworkCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"net1","name":"trusted","purpose":"corporate"}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"net1","name":"trusted","purpose":"corporate"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Existing doc carries controller-managed fields we don't model.
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"net1","name":"trusted","purpose":"corporate","attr_hidden_id":"LAN","networkgroup":"LAN"}]}`))
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

	created, err := c.CreateNetwork(ctx, &NetworkConf{Name: "trusted", Purpose: "corporate"})
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if created.ID != "net1" {
		t.Errorf("created id = %q, want net1", created.ID)
	}

	if _, err := c.UpdateNetwork(ctx, &NetworkConf{ID: "net1", Name: "trusted2", Purpose: "corporate"}); err != nil {
		t.Fatalf("UpdateNetwork: %v", err)
	}
	// GET-merge-PUT must preserve the unmodeled field and overwrite the owned one.
	if lastPut["attr_hidden_id"] != "LAN" {
		t.Errorf("PUT dropped unmodeled attr_hidden_id; got %v", lastPut["attr_hidden_id"])
	}
	if lastPut["name"] != "trusted2" {
		t.Errorf("PUT name = %v, want trusted2", lastPut["name"])
	}

	if err := c.DeleteNetwork(ctx, "net1"); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf/", func(w http.ResponseWriter, _ *http.Request) {
		// Legacy "unknown object" shape: HTTP 200, rc=error.
		_, _ = w.Write([]byte(`{"meta":{"rc":"error","msg":"api.err.UnknownObject"},"data":[]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	_, err := c.GetNetwork(context.Background(), "missing")
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"a","name":"lan"},{"_id":"b","name":"guest","purpose":"guest"}]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	nets, err := c.ListNetworks(context.Background())
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(nets) != 2 || nets[1].Name != "guest" {
		t.Fatalf("ListNetworks = %+v", nets)
	}
}

func TestListSites(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/network/api/self/sites", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"s1","name":"default","desc":"Default"}]}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	sites, err := c.ListSites(context.Background())
	if err != nil || len(sites) != 1 || sites[0].Name != "default" || sites[0].Desc != "Default" {
		t.Fatalf("ListSites = %+v, err %v", sites, err)
	}
}
