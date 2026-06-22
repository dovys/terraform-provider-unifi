package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

func httpAction(headers ...[]string) wire.AutomationAction {
	typ := wire.AutomationActionType("HTTP_REQUEST")
	hs := make([]interface{}, len(headers))
	for i, h := range headers {
		hs[i] = []interface{}{h[0], h[1]}
	}
	meta := map[string]interface{}{
		"url":     "https://hooks.example.com/ring",
		"method":  "POST",
		"headers": hs,
	}
	return wire.AutomationAction{Type: &typ, Metadata: &meta}
}

// TestSetHeaderReplacesExisting confirms an existing header pair is overwritten
// (not duplicated) and that the other action and the action's url/method are
// carried back unchanged (whole-array replace).
func TestSetHeaderReplacesExisting(t *testing.T) {
	setLight := wire.AutomationActionType("SET_LIGHT")
	actions := []wire.AutomationAction{
		{Type: &setLight},
		httpAction([]string{"X-Webhook-Token", "old"}, []string{"Content-Type", "application/json"}),
	}
	out, err := setHeaderOnHTTPAction(actions, "X-Webhook-Token", "new")
	if err != nil {
		t.Fatalf("setHeaderOnHTTPAction: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d actions, want 2 (other action preserved)", len(out))
	}
	meta := out[1].(map[string]interface{})["metadata"].(map[string]interface{})
	if meta["url"] != "https://hooks.example.com/ring" || meta["method"] != "POST" {
		t.Errorf("action url/method not carried back: %+v", meta)
	}
	headers := meta["headers"].([]interface{})
	if len(headers) != 2 {
		t.Fatalf("got %d headers, want 2 (replace not append)", len(headers))
	}
	tok := headers[0].([]interface{})
	if tok[0] != "X-Webhook-Token" || tok[1] != "new" {
		t.Errorf("token header = %v, want [X-Webhook-Token new]", tok)
	}
	ct := headers[1].([]interface{})
	if ct[0] != "Content-Type" {
		t.Errorf("unrelated header dropped: %v", headers)
	}
}

// TestSetHeaderAppendsNew adds a header absent from the action.
func TestSetHeaderAppendsNew(t *testing.T) {
	actions := []wire.AutomationAction{httpAction([]string{"Content-Type", "application/json"})}
	out, err := setHeaderOnHTTPAction(actions, "X-Webhook-Token", "secret")
	if err != nil {
		t.Fatalf("setHeaderOnHTTPAction: %v", err)
	}
	headers := out[0].(map[string]interface{})["metadata"].(map[string]interface{})["headers"].([]interface{})
	if len(headers) != 2 {
		t.Fatalf("got %d headers, want 2 (append)", len(headers))
	}
	last := headers[1].([]interface{})
	if last[0] != "X-Webhook-Token" || last[1] != "secret" {
		t.Errorf("appended header = %v, want [X-Webhook-Token secret]", last)
	}
}

// TestSetHeaderTupleRoundTrip checks the overlay serializes headers as JSON
// two-element string arrays (the wire tuple shape), not as an object.
func TestSetHeaderTupleRoundTrip(t *testing.T) {
	actions := []wire.AutomationAction{httpAction()}
	out, err := setHeaderOnHTTPAction(actions, "X-Webhook-Token", "secret")
	if err != nil {
		t.Fatalf("setHeaderOnHTTPAction: %v", err)
	}
	raw, err := json.Marshal(automationConfig{Actions: out})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(raw); !strings.Contains(got, `"headers":[["X-Webhook-Token","secret"]]`) {
		t.Errorf("serialized headers = %s, want [name,value] tuple array", got)
	}
}

// TestSetHeaderRejectsAmbiguous errors when zero or multiple HTTP_REQUEST
// actions exist (can't pick which webhook header to manage).
func TestSetHeaderRejectsAmbiguous(t *testing.T) {
	setLight := wire.AutomationActionType("SET_LIGHT")
	if _, err := setHeaderOnHTTPAction([]wire.AutomationAction{{Type: &setLight}}, "X", "y"); err == nil {
		t.Error("no HTTP_REQUEST action: want error, got nil")
	}
	two := []wire.AutomationAction{httpAction(), httpAction()}
	if _, err := setHeaderOnHTTPAction(two, "X", "y"); err == nil {
		t.Error("multiple HTTP_REQUEST actions: want error, got nil")
	}
}
