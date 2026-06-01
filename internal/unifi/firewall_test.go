package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestFirewallGroupCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallgroup", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"fg1","name":"Servers","group_type":"address-group","group_members":["10.0.0.10"]}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallgroup/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"fg1","name":"Servers","group_type":"address-group","group_members":["10.0.0.10"],"attr_hidden":true}]}`))
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

	created, err := c.CreateFirewallGroup(ctx, &FirewallGroup{Name: "Servers", GroupType: "address-group", GroupMembers: []string{"10.0.0.10"}})
	if err != nil || created.ID != "fg1" {
		t.Fatalf("CreateFirewallGroup = %+v, err %v", created, err)
	}

	if _, err := c.UpdateFirewallGroup(ctx, &FirewallGroup{ID: "fg1", Name: "Servers2", GroupType: "address-group", GroupMembers: []string{"10.0.0.10", "10.0.0.11"}}); err != nil {
		t.Fatalf("UpdateFirewallGroup: %v", err)
	}
	if lastPut["attr_hidden"] != true {
		t.Errorf("GET-merge-PUT dropped unmodeled attr_hidden; got %v", lastPut["attr_hidden"])
	}
	if lastPut["name"] != "Servers2" {
		t.Errorf("PUT name = %v, want Servers2", lastPut["name"])
	}
	if members, ok := lastPut["group_members"].([]any); !ok || len(members) != 2 {
		t.Errorf("group_members should be the new 2-element list; got %v", lastPut["group_members"])
	}

	if err := c.DeleteFirewallGroup(ctx, "fg1"); err != nil {
		t.Fatalf("DeleteFirewallGroup: %v", err)
	}
}

func TestFirewallRuleCRUD_AndGetMergePUT(t *testing.T) {
	var lastPut map[string]any
	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallrule", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"fr1","name":"block","ruleset":"LAN_IN","rule_index":20000,"action":"drop"}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"fr1","name":"block","ruleset":"LAN_IN","rule_index":20000,"action":"drop"}]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallrule/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[{"_id":"fr1","name":"block","ruleset":"LAN_IN","rule_index":20000,"action":"drop","setting_preference":"manual"}]}`))
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

	rules, err := c.ListFirewallRules(ctx)
	if err != nil || len(rules) != 1 || rules[0].Ruleset != "LAN_IN" {
		t.Fatalf("ListFirewallRules = %+v, err %v", rules, err)
	}

	idx := 20000
	created, err := c.CreateFirewallRule(ctx, &FirewallRule{Name: "block", Ruleset: "LAN_IN", RuleIndex: &idx, Action: "drop"})
	if err != nil || created.ID != "fr1" {
		t.Fatalf("CreateFirewallRule = %+v, err %v", created, err)
	}

	accept := "accept"
	if _, err := c.UpdateFirewallRule(ctx, &FirewallRule{ID: "fr1", Name: "block", Ruleset: "LAN_IN", RuleIndex: &idx, Action: accept}); err != nil {
		t.Fatalf("UpdateFirewallRule: %v", err)
	}
	// GET-merge-PUT must preserve the unmodeled setting_preference and overwrite action.
	if lastPut["setting_preference"] != "manual" {
		t.Errorf("PUT dropped unmodeled setting_preference; got %v", lastPut["setting_preference"])
	}
	if lastPut["action"] != "accept" {
		t.Errorf("PUT action = %v, want accept", lastPut["action"])
	}

	if err := c.DeleteFirewallRule(ctx, "fr1"); err != nil {
		t.Fatalf("DeleteFirewallRule: %v", err)
	}
}
