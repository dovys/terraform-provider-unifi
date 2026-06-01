package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func bptr(b bool) *bool { return &b }

func TestDNSRecordCRUD(t *testing.T) {
	var lastBody map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/v2/api/site/default/static-dns", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"_id":"d1","key":"a.example.com","record_type":"A","value":"10.0.0.5","enabled":true}]`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"d1","key":"a.example.com","record_type":"A","value":"10.0.0.5","enabled":true}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/static-dns/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
			_, _ = w.Write([]byte(`{"_id":"d1","key":"a.example.com","record_type":"A","value":"10.0.0.9","enabled":true}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	rec, err := c.CreateDNSRecord(ctx, &StaticDNSRecord{
		Key: "a.example.com", RecordType: "A", Value: "10.0.0.5", Enabled: bptr(true),
	})
	if err != nil {
		t.Fatalf("CreateDNSRecord: %v", err)
	}
	if rec.ID != "d1" {
		t.Errorf("created id = %q, want d1", rec.ID)
	}
	// Unset optionals must be omitted from the v2 body, not sent as zero.
	if _, ok := lastBody["ttl"]; ok {
		t.Errorf("ttl should be omitted when nil; body = %v", lastBody)
	}
	if lastBody["record_type"] != "A" {
		t.Errorf("record_type = %v, want A", lastBody["record_type"])
	}

	got, err := c.GetDNSRecord(ctx, "d1")
	if err != nil || got.Value != "10.0.0.5" {
		t.Fatalf("GetDNSRecord(d1) = %+v, err %v", got, err)
	}
	if _, err := c.GetDNSRecord(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDNSRecord(nope) err = %v, want ErrNotFound", err)
	}

	upd, err := c.UpdateDNSRecord(ctx, &StaticDNSRecord{
		ID: "d1", Key: "a.example.com", RecordType: "A", Value: "10.0.0.9", Enabled: bptr(true),
	})
	if err != nil {
		t.Fatalf("UpdateDNSRecord: %v", err)
	}
	if upd.Value != "10.0.0.9" {
		t.Errorf("updated value = %q, want 10.0.0.9", upd.Value)
	}
	// v2 write contract: _id rides in the body as well as the URL.
	if lastBody["_id"] != "d1" {
		t.Errorf("v2 PUT body missing _id; got %v", lastBody["_id"])
	}

	if err := c.DeleteDNSRecord(ctx, "d1"); err != nil {
		t.Fatalf("DeleteDNSRecord: %v", err)
	}
}

// When firmware would route local DNS to Integration, the client must refuse
// with a CapabilityError rather than silently misbehaving — the path is not
// implemented yet (deriveFlags keeps IntegrationDNSPolicies false today, so we
// force the router into that state to exercise the guard).
func TestDNSRecord_IntegrationGuard(t *testing.T) {
	mux := http.NewServeMux()
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	// Swap in a router whose flags route OpLocalDNS to Integration.
	cl := c.(*client)
	cl.router = NewRouter(FeatureFlags{IntegrationWrites: true, IntegrationDNSPolicies: true})

	_, err := cl.CreateDNSRecord(context.Background(), &StaticDNSRecord{Key: "x", RecordType: "A", Value: "1.2.3.4"})
	var capErr *CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("CreateDNSRecord err = %v, want *CapabilityError", err)
	}
}
