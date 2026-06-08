package protect

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// fakeDoer is a stand-in session: it records the last call and returns a queued
// or default response (mirrors the Network package's test harness).
type fakeDoer struct {
	method, path string
	body         any

	resp  *transport.Response
	queue []*transport.Response
	err   error
}

func (f *fakeDoer) Do(_ context.Context, method, path string, body any) (*transport.Response, error) {
	f.method, f.path, f.body = method, path, body
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

func okJSON(body string) *transport.Response {
	return &transport.Response{StatusCode: http.StatusOK, Body: []byte(body)}
}

func newTestClient(f *fakeDoer) *Client { return &Client{core: &core{session: f}} }

func ptr[T any](v T) *T { return &v }

func TestCamerasListPath(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`[{"id":"c1","mac":"aabbccddeeff","name":"Front"}]`)}
	cams, err := newTestClient(f).Cameras().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.method != http.MethodGet {
		t.Errorf("method = %q, want GET", f.method)
	}
	if want := "/proxy/protect/api/cameras"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if len(cams) != 1 || cams[0].Id != "c1" {
		t.Fatalf("cams = %+v, want one camera c1", cams)
	}
}

func TestCameraGetByID(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"id":"c1","mac":"aabbccddeeff"}`)}
	cam, err := newTestClient(f).Cameras().Get(context.Background(), "c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "/proxy/protect/api/cameras/c1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	if cam.Id != "c1" {
		t.Errorf("id = %q, want c1", cam.Id)
	}
}

func TestCameraGetByMACNormalizes(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`[{"id":"c1","mac":"aabbccddeeff"},{"id":"c2","mac":"112233445566"}]`)}
	// Mixed case + colons must match the colon-free lowercase wire value.
	cam, err := newTestClient(f).Cameras().GetByMAC(context.Background(), "11:22:33:44:55:66")
	if err != nil {
		t.Fatalf("GetByMAC: %v", err)
	}
	if cam.Id != "c2" {
		t.Errorf("id = %q, want c2", cam.Id)
	}
	if _, err := newTestClient(f).Cameras().GetByMAC(context.Background(), "00:00:00:00:00:00"); err != ErrNotFound {
		t.Errorf("absent MAC err = %v, want ErrNotFound", err)
	}
}

// TestCameraUpdateSendsOwnedOverlayOnly asserts PATCH carries only the modeled
// fields (omitempty overlay) — never the full read object, whose no-omitempty
// telemetry would clobber controller state.
func TestCameraUpdateSendsOwnedOverlayOnly(t *testing.T) {
	type cameraConfig struct {
		Name      *string  `json:"name,omitempty"`
		MicVolume *float32 `json:"micVolume,omitempty"`
	}
	f := &fakeDoer{resp: okJSON(`{"id":"c1","name":"Front Door"}`)}
	_, err := newTestClient(f).Cameras().Update(context.Background(), "c1", cameraConfig{Name: ptr("Front Door")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", f.method)
	}
	if want := "/proxy/protect/api/cameras/c1"; f.path != want {
		t.Errorf("path = %q, want %q", f.path, want)
	}
	got, _ := json.Marshal(f.body)
	if !strings.Contains(string(got), `"name":"Front Door"`) {
		t.Errorf("body missing name: %s", got)
	}
	if strings.Contains(string(got), "micVolume") {
		t.Errorf("nil owned fields must be omitted, got: %s", got)
	}
}

func TestCameraUpdateRequiresID(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{}`)}
	if _, err := newTestClient(f).Cameras().Update(context.Background(), "", struct{}{}); err == nil {
		t.Fatal("Update with empty id: want error, got nil")
	}
}

func TestCameraDeleteSwallowsNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if err := newTestClient(f).Cameras().Delete(context.Background(), "gone"); err != nil {
		t.Errorf("Delete of absent camera: %v, want nil", err)
	}
	if f.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", f.method)
	}
}

func TestGetMapsNotFound(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusNotFound}}
	if _, err := newTestClient(f).Cameras().Get(context.Background(), "missing"); err != ErrNotFound {
		t.Errorf("Get err = %v, want ErrNotFound", err)
	}
}

func TestNotFoundByMessage(t *testing.T) {
	f := &fakeDoer{err: &transport.APIError{StatusCode: http.StatusBadRequest, Message: "Camera does not exist"}}
	if _, err := newTestClient(f).Lights().Get(context.Background(), "x"); err != ErrNotFound {
		t.Errorf("message-based not-found err = %v, want ErrNotFound", err)
	}
}

func TestNVRSingletonPaths(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"id":"nvr1","version":"7.1.69","name":"Console"}`)}
	c := newTestClient(f)

	nvr, err := c.NVR().Get(context.Background())
	if err != nil {
		t.Fatalf("NVR Get: %v", err)
	}
	if want := "/proxy/protect/api/nvr"; f.path != want {
		t.Errorf("NVR Get path = %q, want %q", f.path, want)
	}
	if nvr.Version == nil || *nvr.Version != "7.1.69" {
		t.Errorf("version = %v, want 7.1.69", nvr.Version)
	}

	type nvrConfig struct {
		Name *string `json:"name,omitempty"`
	}
	if _, err := c.NVR().Update(context.Background(), nvrConfig{Name: ptr("Lobby")}); err != nil {
		t.Fatalf("NVR Update: %v", err)
	}
	if f.method != http.MethodPatch || f.path != "/proxy/protect/api/nvr" {
		t.Errorf("NVR Update = %s %s, want PATCH /proxy/protect/api/nvr", f.method, f.path)
	}

	v, err := c.NVR().Version(context.Background())
	if err != nil || v != "7.1.69" {
		t.Errorf("Version = %q, %v; want 7.1.69, nil", v, err)
	}
}

func TestLiveviewCRUDPaths(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`{"id":"lv1","name":"Lobby"}`)}
	c := newTestClient(f)

	type liveviewConfig struct {
		Name   *string `json:"name,omitempty"`
		Layout *int    `json:"layout,omitempty"`
	}
	if _, err := c.Liveviews().Create(context.Background(), liveviewConfig{Name: ptr("Lobby"), Layout: ptr(4)}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.method != http.MethodPost || f.path != "/proxy/protect/api/liveviews" {
		t.Errorf("Create = %s %s, want POST /proxy/protect/api/liveviews", f.method, f.path)
	}

	if _, err := c.Liveviews().Update(context.Background(), "lv1", liveviewConfig{Name: ptr("Lobby 2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.method != http.MethodPatch || f.path != "/proxy/protect/api/liveviews/lv1" {
		t.Errorf("Update path = %s %s", f.method, f.path)
	}

	if err := c.Liveviews().Delete(context.Background(), "lv1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if f.method != http.MethodDelete || f.path != "/proxy/protect/api/liveviews/lv1" {
		t.Errorf("Delete path = %s %s", f.method, f.path)
	}
}

func TestLiveviewGetByName(t *testing.T) {
	f := &fakeDoer{resp: okJSON(`[{"id":"lv1","name":"Lobby"},{"id":"lv2","name":"Garage"}]`)}
	lv, err := newTestClient(f).Liveviews().GetByName(context.Background(), "Garage")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if lv.Id != "lv2" {
		t.Errorf("id = %q, want lv2", lv.Id)
	}
}
