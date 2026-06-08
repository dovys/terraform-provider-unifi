package provider

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Regression coverage for binding a switch port to a unifi_network_port_profile
// via port_profile_id. The controller, once a profile governs a port, returns
// the bound override with its inline VLAN fields (forward, native/tagged VLAN)
// null — the profile owns them. The apply-time state for that port must reflect
// those authoritative controller values, never the values the provider wrote, so
// every Optional+Computed leaf resolves to a concrete (null) value and the apply
// produces a result consistent with the plan ("inconsistent result after apply"
// otherwise). Tests drive the resource interface (Create / Update) and an
// end-to-end ApplyResourceChange, the latter exercising the framework's
// post-apply consistency check the way Terraform core does.

// boundDeviceJSON is a /stat/device list where port 3 is bound to a profile:
// portconf_id set, every inline VLAN field null (the profile governs them).
const boundDeviceJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"US8","type":"usw","adopted":true,"port_overrides":[
	{"port_idx":3,"portconf_id":"prof123","forward":null,"native_networkconf_id":null,"tagged_vlan_mgmt":null}
]}]}`

// bindTestJWT builds the unsigned TOKEN the classic session reads claims from.
func bindTestJWT() string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	pb, _ := json.Marshal(map[string]any{"csrfToken": "c", "exp": time.Now().Add(time.Hour).Unix()})
	return h + "." + base64.RawURLEncoding.EncodeToString(pb) + ".sig"
}

// bindDeviceResource returns a device resource wired to a controller that serves
// the profile-bound device on /stat/device and accepts the /rest/device PUT.
func bindDeviceResource(t *testing.T) *deviceResource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: bindTestJWT(), Path: "/"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(boundDeviceJSON))
	}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	cl := transport.NewClassic(srv.URL, "admin", "pw", hc, 0)
	if err := cl.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	return &deviceResource{net: network.New(cl, "default")}
}

func deviceResourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&deviceResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// boundPlan builds a plan with mac set and port_overrides = {"3" =
// {port_profile_id = "prof123"}}. Every other override leaf is unknown — the
// shape the framework hands the provider for an unconfigured Optional+Computed
// nested attribute. id is supplied (e.g. on update); pass "" to leave it null.
func boundPlan(t *testing.T, sch rschema.Schema, id string) tfsdk.Plan {
	t.Helper()
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], tftypes.UnknownValue)
	}
	ovals["port_profile_id"] = tftypes.NewValue(tftypes.String, "prof123")
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"3": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	top := map[string]tftypes.Value{}
	for name, ty := range schemaType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "id":
			if id != "" {
				top[name] = tftypes.NewValue(ty, id)
			} else {
				top[name] = tftypes.NewValue(ty, tftypes.UnknownValue)
			}
		case "port_overrides":
			top[name] = poMap
		case "forget_on_destroy":
			top[name] = tftypes.NewValue(ty, nil)
		default:
			top[name] = tftypes.NewValue(ty, tftypes.UnknownValue)
		}
	}
	return tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}

// assertBoundPortConsistent fails if the bound port is missing or any of its
// leaves is still unknown — the two ways the post-apply state would be
// inconsistent with the plan.
func assertBoundPortConsistent(t *testing.T, m deviceResourceModel) {
	t.Helper()
	el, ok := m.PortOverrides.Elements()["3"]
	if !ok {
		t.Fatal("port 3 absent from post-apply state — a declared port must survive (else 'inconsistent result after apply')")
	}
	obj := el.(types.Object)
	if pp := obj.Attributes()["port_profile_id"].(types.String); pp.ValueString() != "prof123" {
		t.Errorf("port_profile_id = %q, want prof123", pp.ValueString())
	}
	for name, v := range obj.Attributes() {
		if v.IsUnknown() {
			t.Errorf("leaf %q is unknown in post-apply state — every Optional+Computed leaf must resolve to a concrete value", name)
		}
	}
	// The profile governs the inline VLAN fields, so the controller nulls them;
	// post-apply state must mirror that, not the written placeholder.
	for _, name := range []string{"forward", "native_network_id", "tagged_vlan_mgmt"} {
		if s := obj.Attributes()[name].(types.String); !s.IsNull() {
			t.Errorf("inline field %q = %q, want null (profile-governed)", name, s.ValueString())
		}
	}
}

// TestDevicePortProfileBind_CreateConsistent: creating a device that binds a
// port to a profile yields a consistent result — the bound port survives and
// every computed inline leaf resolves to the controller's null.
func TestDevicePortProfileBind_CreateConsistent(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResource(t)
	sch := deviceResourceSchema(t)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: boundPlan(t, sch, "")}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)
	assertBoundPortConsistent(t, m)
}

// TestDevicePortProfileBind_UpdateConsistent: rebinding a port that previously
// carried inline VLAN config to a profile yields a consistent result. Prior
// state has the inline fields populated; after the profile bind the controller
// nulls them and the post-apply state must follow.
func TestDevicePortProfileBind_UpdateConsistent(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResource(t)
	sch := deviceResourceSchema(t)

	// Prior state: port 3 had forward="all", native_network_id="netA", no profile.
	prior := stateWithInlinePort3(t, sch)
	plan := boundPlan(t, sch, "d1")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: prior}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)
	assertBoundPortConsistent(t, m)
}

// boundNoPortconfJSON is a /stat/device list where port 3 IS echoed but the
// document omits portconf_id — the realistic eventual-consistency race where the
// read-after-PUT hasn't yet reflected the binding the controller accepted (the
// binding lands in /rest/device first; the /stat/device list the read uses lags
// a refresh). A configured port_profile_id must still round-trip.
const boundNoPortconfJSON = `{"meta":{"rc":"ok"},"data":[{"_id":"d1","mac":"aa:bb:cc:00:00:01","model":"US8","type":"usw","adopted":true,"port_overrides":[
	{"port_idx":3,"forward":null}
]}]}`

// bindDeviceResourceServing returns a device resource whose controller serves
// the given /stat/device payload (and accepts the PUT).
func bindDeviceResourceServing(t *testing.T, stat string) *deviceResource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: bindTestJWT(), Path: "/"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(stat))
	}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	cl := transport.NewClassic(srv.URL, "admin", "pw", hc, 0)
	if err := cl.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	return &deviceResource{net: network.New(cl, "default")}
}

// TestDevicePortProfileBind_BindingRoundTripsWhenDocumentOmitsIt is the specific
// regression for the live failure: the user configures port_profile_id (a known
// plan value), the controller accepts the write, but the /stat/device read the
// provider maps from doesn't yet carry portconf_id. The configured value must
// round-trip into apply-time state — reading it back as null contradicts the
// plan and surfaces "Provider produced inconsistent result after apply:
// .port_overrides["3"].port_profile_id: was "prof123", but now null".
func TestDevicePortProfileBind_BindingRoundTripsWhenDocumentOmitsIt(t *testing.T) {
	ctx := context.Background()
	r := bindDeviceResourceServing(t, boundNoPortconfJSON)
	sch := deviceResourceSchema(t)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, resource.CreateRequest{Plan: boundPlan(t, sch, "")}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var m deviceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &m)...)

	el, ok := m.PortOverrides.Elements()["3"]
	if !ok {
		t.Fatal("port 3 absent from post-apply state")
	}
	pp := el.(types.Object).Attributes()["port_profile_id"].(types.String)
	if pp.IsNull() || pp.ValueString() != "prof123" {
		t.Errorf("port_profile_id = %v, want \"prof123\" — a configured binding must round-trip even when the read document omits portconf_id", pp)
	}
}

// stateWithInlinePort3 builds prior state where port 3 carries inline VLAN
// config (forward, native VLAN) and no profile — the pre-rebind shape.
func stateWithInlinePort3(t *testing.T, sch rschema.Schema) tfsdk.State {
	t.Helper()
	objTypes := map[string]tftypes.Type{}
	for k, ty := range portOverrideAttrTypes {
		objTypes[k] = ty.TerraformType(context.Background())
	}
	otype := tftypes.Object{AttributeTypes: objTypes}
	ovals := map[string]tftypes.Value{}
	for k := range portOverrideAttrTypes {
		ovals[k] = tftypes.NewValue(objTypes[k], nil)
	}
	ovals["forward"] = tftypes.NewValue(tftypes.String, "all")
	ovals["native_network_id"] = tftypes.NewValue(tftypes.String, "netA")
	po := tftypes.NewValue(otype, ovals)
	poMap := tftypes.NewValue(tftypes.Map{ElementType: otype}, map[string]tftypes.Value{"3": po})

	schemaType := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	top := map[string]tftypes.Value{}
	for name, ty := range schemaType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "id":
			top[name] = tftypes.NewValue(ty, "d1")
		case "port_overrides":
			top[name] = poMap
		default:
			top[name] = tftypes.NewValue(ty, nil)
		}
	}
	return tfsdk.State{Schema: sch, Raw: tftypes.NewValue(schemaType, top)}
}

// TestDevicePortProfileBind_ApplyResourceChange drives a full
// ApplyResourceChange through the provider server, which runs the framework's
// post-apply consistency check. A surfaced error diagnostic is the bug.
func TestDevicePortProfileBind_ApplyResourceChange(t *testing.T) {
	t.Setenv("UNIFI_DISABLE_CLIENT_CACHE", "1")
	ctx := context.Background()
	url := bindController(t)

	srv := providerserver.NewProtocol6(New("test")())()
	gpr, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	configureBindProvider(t, srv, gpr, url)

	objType := gpr.ResourceSchemas["unifi_network_device"].ValueType().(tftypes.Object)
	config := bindConfigValue(objType)
	prior := tftypes.NewValue(objType, nil)
	configDV, _ := tfprotov6.NewDynamicValue(objType, config)
	priorDV, _ := tfprotov6.NewDynamicValue(objType, prior)

	planResp, err := srv.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName: "unifi_network_device", PriorState: &priorDV, Config: &configDV, ProposedNewState: &configDV,
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	assertNoErrorDiags(t, "plan", planResp.Diagnostics)

	applyResp, err := srv.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName: "unifi_network_device", PriorState: &priorDV, Config: &configDV, PlannedState: planResp.PlannedState,
	})
	if err != nil {
		t.Fatalf("ApplyResourceChange: %v", err)
	}
	assertNoErrorDiags(t, "apply", applyResp.Diagnostics)
}

// bindController starts a controller satisfying the provider handshake (login,
// sysinfo) and serving the profile-bound device; Protect and the cert surface
// report absent (404).
func bindController(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: bindTestJWT(), Path: "/"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/stat/sysinfo"):
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"version":"8.0.28"}]}`))
		case strings.HasPrefix(r.URL.Path, "/proxy/protect/"), strings.Contains(r.URL.Path, "userCertificates"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/stat/device"), strings.Contains(r.URL.Path, "/rest/device/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(boundDeviceJSON))
		default:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func configureBindProvider(t *testing.T, srv tfprotov6.ProviderServer, gpr *tfprotov6.GetProviderSchemaResponse, url string) {
	t.Helper()
	ps := gpr.Provider.ValueType().(tftypes.Object)
	pv := map[string]tftypes.Value{}
	for name, ty := range ps.AttributeTypes {
		switch name {
		case "host":
			pv[name] = tftypes.NewValue(ty, url)
		case "username":
			pv[name] = tftypes.NewValue(ty, "admin")
		case "password":
			pv[name] = tftypes.NewValue(ty, "pw")
		case "allow_insecure":
			pv[name] = tftypes.NewValue(ty, true)
		default:
			pv[name] = tftypes.NewValue(ty, nil)
		}
	}
	dv, _ := tfprotov6.NewDynamicValue(ps, tftypes.NewValue(ps, pv))
	resp, err := srv.ConfigureProvider(context.Background(), &tfprotov6.ConfigureProviderRequest{Config: &dv})
	if err != nil {
		t.Fatalf("ConfigureProvider: %v", err)
	}
	assertNoErrorDiags(t, "configure", resp.Diagnostics)
}

// bindConfigValue is the device config binding port 3 to a profile (inline VLAN
// fields omitted from config — the user delegates them to the profile).
func bindConfigValue(objType tftypes.Object) tftypes.Value {
	poType := objType.AttributeTypes["port_overrides"].(tftypes.Map).ElementType.(tftypes.Object)
	pov := map[string]tftypes.Value{}
	for k, ty := range poType.AttributeTypes {
		pov[k] = tftypes.NewValue(ty, nil)
	}
	pov["port_profile_id"] = tftypes.NewValue(tftypes.String, "prof123")
	po := tftypes.NewValue(poType, pov)
	poMap := tftypes.NewValue(objType.AttributeTypes["port_overrides"], map[string]tftypes.Value{"3": po})
	top := map[string]tftypes.Value{}
	for name, ty := range objType.AttributeTypes {
		switch name {
		case "mac":
			top[name] = tftypes.NewValue(ty, "aa:bb:cc:00:00:01")
		case "port_overrides":
			top[name] = poMap
		default:
			top[name] = tftypes.NewValue(ty, nil)
		}
	}
	return tftypes.NewValue(objType, top)
}

func assertNoErrorDiags(t *testing.T, phase string, diags []*tfprotov6.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("%s error diagnostic: %s — %s", phase, d.Summary, d.Detail)
		}
	}
}
