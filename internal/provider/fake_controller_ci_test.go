package provider

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// fakeController serves canned bodies keyed by "METHOD /path/suffix" (longest
// matching suffix wins) behind the provider handshake. A body of "<echo>"
// returns the request body; "<400>" fails with HTTP 400. Unrouted v2 paths
// 404; unrouted legacy paths return an empty list.
type fakeController struct {
	URL string

	mu     sync.Mutex
	routes map[string]string
	calls  []fakeCall
}

type fakeCall struct {
	Method, Path, Body string
}

func newFakeController(t *testing.T, routes map[string]string) *fakeController {
	t.Helper()
	fc := &fakeController{routes: routes}
	srv := httptest.NewServer(http.HandlerFunc(fc.serve))
	t.Cleanup(srv.Close)
	fc.URL = srv.URL
	return fc
}

func (fc *fakeController) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fc.mu.Lock()
	fc.calls = append(fc.calls, fakeCall{r.Method, r.URL.Path, string(body)})
	fc.mu.Unlock()

	switch {
	case r.URL.Path == "/api/auth/login":
		http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: bindTestJWT(), Path: "/"})
		_, _ = w.Write([]byte(`{}`))
		return
	case strings.HasSuffix(r.URL.Path, "/stat/sysinfo"):
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"11.0.104"}]}`))
		return
	case strings.HasPrefix(r.URL.Path, "/proxy/protect/"), strings.Contains(r.URL.Path, "userCertificates"):
		w.WriteHeader(http.StatusNotFound)
		return
	}
	best := ""
	for k := range fc.routes {
		m, suffix, _ := strings.Cut(k, " ")
		if m == r.Method && strings.HasSuffix(r.URL.Path, suffix) && len(k) > len(best) {
			best = k
		}
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case fc.routes[best] == "<echo>":
		_, _ = w.Write(body)
	case fc.routes[best] == "<400>":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"api.err.Invalid","message":"bad request"}`))
	case best != "":
		_, _ = w.Write([]byte(fc.routes[best]))
	case strings.Contains(r.URL.Path, "/v2/"):
		w.WriteHeader(http.StatusNotFound)
	default:
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}
}

// lastCall returns the most recent request with the method and path suffix.
func (fc *fakeController) lastCall(method, suffix string) (fakeCall, bool) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	for i := len(fc.calls) - 1; i >= 0; i-- {
		if c := fc.calls[i]; c.Method == method && strings.HasSuffix(c.Path, suffix) {
			return c, true
		}
	}
	return fakeCall{}, false
}

// network returns a Network client logged in to the fake.
func (fc *fakeController) network(t *testing.T) *network.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	cl := transport.NewClassic(fc.URL, "admin", "pw", hc, 0)
	if err := cl.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	return network.New(cl, "default")
}

func resourceSchema(t *testing.T, r resource.Resource) rschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// stateWith builds state with the given top-level attributes set and the rest null.
func stateWith(sch rschema.Schema, set map[string]tftypes.Value) tfsdk.State {
	ty := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	vals := map[string]tftypes.Value{}
	for name, at := range ty.AttributeTypes {
		if v, ok := set[name]; ok {
			vals[name] = v
		} else {
			vals[name] = tftypes.NewValue(at, nil)
		}
	}
	return tfsdk.State{Schema: sch, Raw: tftypes.NewValue(ty, vals)}
}

// readResource runs r.Read over state; removed reports RemoveResource.
func readResource(t *testing.T, r resource.Resource, state tfsdk.State) (out tfsdk.State, removed bool) {
	t.Helper()
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	return resp.State, resp.State.Raw.IsNull()
}
