package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// findHandler returns the handler for a key (test convenience).
func findHandler(t *testing.T, key string) settingKeyHandler {
	t.Helper()
	for _, h := range settingKeyHandlers() {
		if h.key == key {
			return h
		}
	}
	t.Fatalf("no settingKeyHandler for %q", key)
	return settingKeyHandler{}
}

// TestSettingsPresentKeys is the no-drift guarantee: settingsPresentKeys (which
// drives both Read and apply) reports only the blocks that are set, so managing
// one section never reads or writes another.
func TestSettingsPresentKeys(t *testing.T) {
	ctx := context.Background()
	var d diag.Diagnostics

	// Only ntp is declared; country and locale are null (unmanaged).
	ntp := findHandler(t, "ntp").fromRaw(ctx, json.RawMessage(`{"setting_preference":"manual","ntp_server_1":"0.pool.ntp.org"}`), &d)
	if d.HasError() {
		t.Fatalf("fromRaw ntp: %v", d)
	}
	m := &networkSettingsModel{
		NTP:     ntp,
		Country: types.ObjectNull(countrySettingsAttrTypes),
		Locale:  types.ObjectNull(localeSettingsAttrTypes),
	}
	got := settingsPresentKeys(m)
	if len(got) != 1 || got[0] != "ntp" {
		t.Fatalf("present keys = %v, want [ntp] (country/locale must stay unmanaged)", got)
	}

	// Empty model → nothing managed.
	empty := &networkSettingsModel{
		NTP:     types.ObjectNull(ntpSettingsAttrTypes),
		Country: types.ObjectNull(countrySettingsAttrTypes),
		Locale:  types.ObjectNull(localeSettingsAttrTypes),
	}
	if got := settingsPresentKeys(empty); len(got) != 0 {
		t.Fatalf("present keys for empty model = %v, want none", got)
	}
}

// TestSettingsRoundTrip decodes each section's flat controller document and
// re-encodes the owned overlay, asserting the configured fields survive and nil
// fields are omitted (the shallow-merge write contract).
func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		key       string
		doc       string
		wantSends []string // substrings the write payload must contain
		wantOmits []string // substrings the write payload must NOT contain
	}{
		{
			key:       "ntp",
			doc:       `{"_id":"s1","site_id":"x","key":"ntp","setting_preference":"manual","ntp_server_1":"0.pool.ntp.org"}`,
			wantSends: []string{`"setting_preference":"manual"`, `"ntp_server_1":"0.pool.ntp.org"`},
			wantOmits: []string{"ntp_server_2", "ntp_server_3", "ntp_server_4", "_id", "site_id"},
		},
		{
			key:       "country",
			doc:       `{"_id":"s2","key":"country","code":840}`,
			wantSends: []string{`"code":840`},
			wantOmits: []string{"_id", "site_id", "key"},
		},
		{
			key:       "locale",
			doc:       `{"_id":"s3","key":"locale","timezone":"America/New_York"}`,
			wantSends: []string{`"timezone":"America/New_York"`},
			wantOmits: []string{"_id", "key"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			h := findHandler(t, tc.key)
			var d diag.Diagnostics
			obj := h.fromRaw(ctx, json.RawMessage(tc.doc), &d)
			if d.HasError() {
				t.Fatalf("fromRaw: %v", d)
			}
			if obj.IsNull() {
				t.Fatal("fromRaw produced a null object for a populated section")
			}
			payload := h.toPayload(ctx, obj, &d)
			if d.HasError() {
				t.Fatalf("toPayload: %v", d)
			}
			if payload == nil {
				t.Fatal("toPayload returned nil for a present block")
			}
			b, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			got := string(b)
			for _, want := range tc.wantSends {
				if !strings.Contains(got, want) {
					t.Errorf("payload %s missing %s", got, want)
				}
			}
			for _, omit := range tc.wantOmits {
				if strings.Contains(got, omit) {
					t.Errorf("payload %s must omit %s", got, omit)
				}
			}
		})
	}
}

// TestSettingsToPayloadNilForUnmanaged confirms a null/unknown block yields a nil
// overlay, so apply skips unmanaged sections entirely.
func TestSettingsToPayloadNilForUnmanaged(t *testing.T) {
	ctx := context.Background()
	var d diag.Diagnostics
	for _, h := range settingKeyHandlers() {
		if p := h.toPayload(ctx, types.ObjectNull(h.attrTypes), &d); p != nil {
			t.Errorf("%s: toPayload(null) = %v, want nil", h.key, p)
		}
	}
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
}
