package network

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// fakeDoer records the last request and returns a canned response, so tests can
// assert the path, method, and the exact bytes the facade hands the transport
// (the wire contract) without a live session.
type fakeDoer struct {
	method, path string // the most recent call
	body         any
	calls        int
	bodies       []any // every call's body, in order (for multi-call flows)

	resp  *transport.Response   // returned when queue is empty
	queue []*transport.Response // popped front-to-back (for multi-call flows)
	err   error
}

func (f *fakeDoer) Do(_ context.Context, method, path string, body any) (*transport.Response, error) {
	f.method, f.path, f.body = method, path, body
	f.bodies = append(f.bodies, body)
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(f.queue) > 0 {
		r := f.queue[0]
		f.queue = f.queue[1:]
		return r, nil
	}
	return f.resp, nil
}

func ptr[T any](v T) *T { return &v }

func newTestClient(f *fakeDoer) *Client {
	return &Client{core: &core{session: f, site: "default"}}
}

func okJSON(body string) *transport.Response {
	return &transport.Response{StatusCode: 200, Body: []byte(body)}
}

// fakeQueue builds a fake whose calls return the given JSON bodies in order
// (for multi-call flows like GET-merge-PUT).
func fakeQueue(bodies ...string) *fakeDoer {
	f := &fakeDoer{}
	for _, b := range bodies {
		f.queue = append(f.queue, okJSON(b))
	}
	return f
}

func TestDNSRecordsList(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`[{"_id":"r1","key":"a.example.com","record_type":"A","value":"10.0.0.1"}]`)}
	recs, err := newTestClient(f).DNS().Records().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.method != "GET" {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/static-dns"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(recs) != 1 || recs[0].Key != "a.example.com" {
		t.Fatalf("records = %+v, want one with key a.example.com", recs)
	}
}

func TestDNSRecordsCreateWireBytes(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"_id":"new","key":"a.example.com","record_type":"A","value":"10.0.0.1"}`)}
	in := &wire.StaticDns{
		Key:        "a.example.com",
		Value:      "10.0.0.1",
		RecordType: ptr(wire.StaticDnsRecordTypeA),
		// Enabled left nil — must be omitted, not sent as false.
	}
	if _, err := newTestClient(f).DNS().Records().Create(context.Background(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.method != "POST" {
		t.Errorf("method = %q, want POST", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/static-dns"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	got, err := json.Marshal(f.body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	b := string(got)
	if !strings.Contains(b, `"key":"a.example.com"`) || !strings.Contains(b, `"record_type":"A"`) {
		t.Errorf("body missing required fields: %s", b)
	}
	if strings.Contains(b, `"enabled"`) {
		t.Errorf("nil Enabled must be omitted, got: %s", b)
	}
	if strings.Contains(b, `"_id"`) {
		t.Errorf("nil _id must be omitted on create, got: %s", b)
	}
}

func TestDNSRecordsUpdateSendsIDInURLAndBody(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"_id":"r1","key":"a.example.com","record_type":"A","value":"10.0.0.2"}`)}
	in := &wire.StaticDns{UnderscoreId: ptr("r1"), Key: "a.example.com", Value: "10.0.0.2", RecordType: ptr(wire.StaticDnsRecordTypeA)}
	if _, err := newTestClient(f).DNS().Records().Update(context.Background(), in); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != "PUT" {
		t.Errorf("method = %q, want PUT", f.method)
	}
	if want := "/proxy/network/v2/api/site/default/static-dns/r1"; f.path != want {
		t.Errorf("path = %q, want %q (_id in URL)", f.path, want)
	}
	got, _ := json.Marshal(f.body)
	if !strings.Contains(string(got), `"_id":"r1"`) {
		t.Errorf("_id must also be in the body, got: %s", got)
	}
}

func TestDNSRecordsUpdateRequiresID(t *testing.T) {
	f := &fakeDoer{}
	_, err := newTestClient(f).DNS().Records().Update(context.Background(), &wire.StaticDns{Key: "x", Value: "y"})
	if err == nil {
		t.Fatal("Update without _id should error")
	}
	if f.calls != 0 {
		t.Errorf("no request should be made when _id is missing, got %d calls", f.calls)
	}
}

func TestDNSRecordsDeleteSwallowsNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: 404}}
	if err := newTestClient(f).DNS().Records().Delete(context.Background(), "gone"); err != nil {
		t.Errorf("Delete of an absent record should be nil, got %v", err)
	}
	if want := "/proxy/network/v2/api/site/default/static-dns/gone"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
}

// TestLegacyCollectionSurface proves the legacy /rest skeleton: path shape and
// the {meta,data} envelope unwrap (the surface R1+ legacy groups build on).
func TestLegacyCollectionSurface(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"meta":{"rc":"ok"},"data":[{"_id":"n1","name":"LAN"}]}`)}
	c := newTestClient(f)
	coll := LegacyCollection[wire.Networkconf]{core: c.core, typ: "networkconf"}

	items, err := coll.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.method != "GET" {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/network/api/s/default/rest/networkconf"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	// rc=error surfaces as an error.
	f.resp = okJSON(`{"meta":{"rc":"error","msg":"api.err.CannotBeEmpty"},"data":[]}`)
	if _, err := coll.List(context.Background()); err == nil {
		t.Error("rc=error should surface as an error")
	}
}
