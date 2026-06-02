package access

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// fakeDoer is a stand-in session: it records the last call and returns a queued
// or default response (mirrors the Network/Protect test harness).
type fakeDoer struct {
	method, path string
	body         any
	calls        int

	resp  *transport.Response
	queue []*transport.Response
	err   error
}

func (f *fakeDoer) Do(_ context.Context, method, path string, body any) (*transport.Response, error) {
	f.method, f.path, f.body = method, path, body
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

// env wraps a data payload in the Access {code,msg,data} envelope with a numeric
// success code.
func env(data string) *transport.Response {
	return &transport.Response{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"msg":"success","data":` + data + `}`)}
}

func newTestClient(f *fakeDoer) *Client { return &Client{core: &core{session: f}} }

func ptr[T any](v T) *T { return &v }

// TestEnvelopeSuccessCodes asserts the loose success-code set: numeric, string,
// and empty all read as success; anything else is an error.
func TestEnvelopeSuccessCodes(t *testing.T) {
	cases := []struct {
		code string
		ok   bool
	}{
		{`0`, true}, {`200`, true}, {`"SUCCESS"`, true}, {`"success"`, true},
		{`""`, true}, {`null`, true},
		{`1`, false}, {`400`, false}, {`"ERROR"`, false}, {`"FAILED"`, false},
	}
	for _, c := range cases {
		e := envelope{Code: json.RawMessage(c.code)}
		if e.ok() != c.ok {
			t.Errorf("ok(code=%s) = %v, want %v", c.code, e.ok(), c.ok)
		}
	}
}

func TestDoorGroupsListPath(t *testing.T) {
	f := &fakeDoer{resp: env(`[{"unique_id":"g1","name":"Lobby","group_type":"door","door_count":3}]`)}
	groups, err := newTestClient(f).DoorGroups().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.method != http.MethodGet {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/access/api/v2/access/door_groups"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(groups) != 1 || groups[0].UniqueId == nil || *groups[0].UniqueId != "g1" {
		t.Fatalf("groups = %+v, want one group g1", groups)
	}
	if groups[0].DoorCount == nil || *groups[0].DoorCount != 3 {
		t.Errorf("door_count = %v, want 3", groups[0].DoorCount)
	}
}

func TestDoorGroupGetPath(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"g1","name":"Lobby","group_type":"door"}`)}
	g, err := newTestClient(f).DoorGroups().Get(context.Background(), "g1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "/proxy/access/api/v2/access/door_group/g1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if g.Name == nil || *g.Name != "Lobby" {
		t.Errorf("name = %v, want Lobby", g.Name)
	}
}

func TestDoorGroupDoorIDsPath(t *testing.T) {
	f := &fakeDoer{resp: env(`["d1","d2","d3"]`)}
	ids, err := newTestClient(f).DoorGroups().DoorIDs(context.Background(), "g1")
	if err != nil {
		t.Fatalf("DoorIDs: %v", err)
	}
	if want := "/proxy/access/api/v2/access/door_group/g1/door_ids"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(ids) != 3 || ids[0] != "d1" {
		t.Errorf("ids = %v, want [d1 d2 d3]", ids)
	}
}

func TestDoorGroupCreateSendsParams(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"g9","name":"New"}`)}
	g, err := newTestClient(f).DoorGroups().Create(context.Background(), wireParams("New", "door", "d1", "d2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.method != http.MethodPost || f.path != "/proxy/access/api/v2/access/door_group" {
		t.Errorf("Create = %s %s, want POST .../door_group", f.method, f.path)
	}
	body, _ := json.Marshal(f.body)
	if !strings.Contains(string(body), `"name":"New"`) || !strings.Contains(string(body), `"door_ids":["d1","d2"]`) {
		t.Errorf("body missing params: %s", body)
	}
	if strings.Contains(string(body), "floor_ids") {
		t.Errorf("nil params must be omitted, got: %s", body)
	}
	if g.UniqueId == nil || *g.UniqueId != "g9" {
		t.Errorf("created id = %v, want g9", g.UniqueId)
	}
}

func TestDoorGroupUpdatePath(t *testing.T) {
	f := &fakeDoer{resp: env(`{"unique_id":"g1","name":"Renamed"}`)}
	_, err := newTestClient(f).DoorGroups().Update(context.Background(), "g1", wireParams("Renamed", "door", "d1"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != http.MethodPut || f.path != "/proxy/access/api/v2/access/door_group/g1" {
		t.Errorf("Update = %s %s, want PUT .../door_group/g1", f.method, f.path)
	}
}

func TestDoorGroupDeleteSwallowsNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if err := newTestClient(f).DoorGroups().Delete(context.Background(), "gone"); err != nil {
		t.Errorf("Delete of absent group: %v, want nil", err)
	}
	if f.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", f.method)
	}
}

// TestEnvelopeErrorCodeSurfaces asserts a non-success envelope code becomes an
// error carrying the controller message (not silently swallowed).
func TestEnvelopeErrorCodeSurfaces(t *testing.T) {
	f := &fakeDoer{resp: &transport.Response{StatusCode: http.StatusOK, Body: []byte(`{"code":1001,"msg":"permission denied","data":null}`)}}
	_, err := newTestClient(f).DoorGroups().List(context.Background())
	if err == nil {
		t.Fatal("want error from non-success envelope code, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want it to carry the controller message", err)
	}
}

// TestEnvelopeNotFoundMessageMapsToErrNotFound asserts an envelope-level
// not-found (200 OK but error code + message) maps to ErrNotFound for
// remove-from-state.
func TestEnvelopeNotFoundMessageMapsToErrNotFound(t *testing.T) {
	f := &fakeDoer{resp: &transport.Response{StatusCode: http.StatusOK, Body: []byte(`{"code":404,"msg":"door group not found","data":null}`)}}
	if _, err := newTestClient(f).DoorGroups().Get(context.Background(), "missing"); err != ErrNotFound {
		t.Errorf("Get err = %v, want ErrNotFound", err)
	}
}

func TestGetMapsTransportNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if _, err := newTestClient(f).DoorGroups().Get(context.Background(), "x"); err != ErrNotFound {
		t.Errorf("Get err = %v, want ErrNotFound", err)
	}
}

func TestProxyPathTrimsLeadingSlash(t *testing.T) {
	c := &core{}
	if got := c.proxyPath("/api/v2/access/door_groups"); got != "/proxy/access/api/v2/access/door_groups" {
		t.Errorf("proxyPath = %q", got)
	}
	if got := c.proxyPath("api/v2/access/door_groups"); got != "/proxy/access/api/v2/access/door_groups" {
		t.Errorf("proxyPath = %q", got)
	}
}

func wireParams(name, groupType string, doorIDs ...string) wire.DoorGroupControllerDoorGroupParams {
	ids := append([]string{}, doorIDs...)
	return wire.DoorGroupControllerDoorGroupParams{Name: ptr(name), GroupType: ptr(groupType), DoorIds: &ids}
}
