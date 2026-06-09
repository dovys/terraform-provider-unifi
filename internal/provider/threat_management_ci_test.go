package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestThreatManagementRefresh decodes a stored `ips` document into the model and
// asserts every modeled writable field maps across, including the category /
// network sets and the boolean flags (an explicit false must survive, not become
// null).
func TestThreatManagementRefresh(t *testing.T) {
	doc := `{
		"_id":"abc","site_id":"s","key":"ips",
		"ips_mode":"ids",
		"enabled_categories":["emerging-malware","botcc"],
		"enabled_networks":["net-1","net-2"],
		"advanced_filtering_preference":"manual",
		"restrict_torrents":true,
		"honeypot_enabled":false,
		"memory_optimized":true,
		"content_filtering_blocking_page_enabled":false
	}`
	var d diag.Diagnostics
	var m threatManagementModel
	r := &threatManagementResource{}
	r.refresh(&m, json.RawMessage(doc), &d)
	if d.HasError() {
		t.Fatalf("refresh: %v", d)
	}
	if m.ID.ValueString() != threatManagementID {
		t.Errorf("id = %q, want %q", m.ID.ValueString(), threatManagementID)
	}
	if m.Mode.ValueString() != "ids" {
		t.Errorf("mode = %q, want ids", m.Mode.ValueString())
	}
	if m.AdvancedFiltering.ValueString() != "manual" {
		t.Errorf("advanced_filtering = %q, want manual", m.AdvancedFiltering.ValueString())
	}
	if !m.RestrictTorrents.ValueBool() {
		t.Errorf("restrict_torrents = false, want true")
	}
	// An explicit false on the wire must read back as false (not null), or it
	// would churn the plan / trip the consistency check.
	if m.HoneypotEnabled.IsNull() || m.HoneypotEnabled.ValueBool() {
		t.Errorf("honeypot_enabled = %v, want explicit false", m.HoneypotEnabled)
	}
	if m.ContentFilteringBlocking.IsNull() || m.ContentFilteringBlocking.ValueBool() {
		t.Errorf("content_filtering = %v, want explicit false", m.ContentFilteringBlocking)
	}
	cats := setStrings(t, m.EnabledCategories)
	if len(cats) != 2 || !cats["emerging-malware"] || !cats["botcc"] {
		t.Errorf("enabled_categories = %v, want {emerging-malware, botcc}", cats)
	}
	nets := setStrings(t, m.EnabledNetworks)
	if len(nets) != 2 || !nets["net-1"] || !nets["net-2"] {
		t.Errorf("enabled_networks = %v, want {net-1, net-2}", nets)
	}
}

// TestThreatManagementRefreshAbsentFields confirms a sparse stored document (only
// ips_mode set) leaves the unmentioned fields null rather than zero-valued, so an
// unmanaged flag doesn't show up as a fabricated `false` in state.
func TestThreatManagementRefreshAbsentFields(t *testing.T) {
	var d diag.Diagnostics
	var m threatManagementModel
	r := &threatManagementResource{}
	r.refresh(&m, json.RawMessage(`{"_id":"x","key":"ips","ips_mode":"disabled"}`), &d)
	if d.HasError() {
		t.Fatalf("refresh: %v", d)
	}
	if m.Mode.ValueString() != "disabled" {
		t.Errorf("mode = %q, want disabled", m.Mode.ValueString())
	}
	if !m.RestrictTorrents.IsNull() {
		t.Errorf("restrict_torrents = %v, want null (absent on wire)", m.RestrictTorrents)
	}
	if !m.HoneypotEnabled.IsNull() {
		t.Errorf("honeypot_enabled = %v, want null (absent on wire)", m.HoneypotEnabled)
	}
	if !m.EnabledCategories.IsNull() {
		t.Errorf("enabled_categories = %v, want null (absent on wire)", m.EnabledCategories)
	}
}

// TestThreatManagementToPayloadSparse is the shallow-merge write contract: a model
// with only mode + categories set must serialize to exactly those keys — every
// unset field omitted — so the controller keeps its stored value for the rest.
func TestThreatManagementToPayloadSparse(t *testing.T) {
	ctx := context.Background()
	var d diag.Diagnostics
	m := &threatManagementModel{
		Mode:                     types.StringValue("ips"),
		EnabledCategories:        sliceToStringSet([]string{"botcc"}),
		EnabledNetworks:          types.SetNull(types.StringType),
		AdvancedFiltering:        types.StringNull(),
		RestrictTorrents:         types.BoolNull(),
		HoneypotEnabled:          types.BoolNull(),
		MemoryOptimized:          types.BoolNull(),
		ContentFilteringBlocking: types.BoolNull(),
	}
	r := &threatManagementResource{}
	payload := r.toPayload(ctx, m, &d)
	if d.HasError() {
		t.Fatalf("toPayload: %v", d)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"ips_mode":"ips"`, `"enabled_categories":["botcc"]`} {
		if !strings.Contains(got, want) {
			t.Errorf("payload %s missing %s", got, want)
		}
	}
	for _, omit := range []string{"enabled_networks", "advanced_filtering_preference", "restrict_torrents", "honeypot_enabled", "memory_optimized", "content_filtering_blocking_page_enabled"} {
		if strings.Contains(got, omit) {
			t.Errorf("payload %s must omit unset field %s", got, omit)
		}
	}
}

// TestThreatManagementToPayloadExplicitFalse confirms an explicitly-set false
// flag is written (it is owned), distinguishing it from an omitted/null flag.
func TestThreatManagementToPayloadExplicitFalse(t *testing.T) {
	ctx := context.Background()
	var d diag.Diagnostics
	m := &threatManagementModel{
		Mode:                     types.StringNull(),
		EnabledCategories:        types.SetNull(types.StringType),
		EnabledNetworks:          types.SetNull(types.StringType),
		AdvancedFiltering:        types.StringNull(),
		RestrictTorrents:         types.BoolValue(false),
		HoneypotEnabled:          types.BoolNull(),
		MemoryOptimized:          types.BoolNull(),
		ContentFilteringBlocking: types.BoolNull(),
	}
	r := &threatManagementResource{}
	payload := r.toPayload(ctx, m, &d)
	if d.HasError() {
		t.Fatalf("toPayload: %v", d)
	}
	b, _ := json.Marshal(payload)
	if !strings.Contains(string(b), `"restrict_torrents":false`) {
		t.Errorf("payload %s must send explicit restrict_torrents=false", b)
	}
}

// TestThreatManagementToPayloadEmptySetClears asserts the null-vs-empty contract
// the write overlay must honor: a configured empty set serializes as an explicit
// `[]` (clear the controller's list), while a null set is omitted (leave it
// unmanaged). Plain `[]string` + omitempty conflated the two — an empty set was
// silently dropped, so it never cleared and the read-back churned the plan.
func TestThreatManagementToPayloadEmptySetClears(t *testing.T) {
	ctx := context.Background()
	var d diag.Diagnostics
	m := &threatManagementModel{
		Mode:                     types.StringNull(),
		EnabledCategories:        sliceToStringSet([]string{}), // configured, empty → clear
		EnabledNetworks:          types.SetNull(types.StringType),
		AdvancedFiltering:        types.StringNull(),
		RestrictTorrents:         types.BoolNull(),
		HoneypotEnabled:          types.BoolNull(),
		MemoryOptimized:          types.BoolNull(),
		ContentFilteringBlocking: types.BoolNull(),
	}
	r := &threatManagementResource{}
	payload := r.toPayload(ctx, m, &d)
	if d.HasError() {
		t.Fatalf("toPayload: %v", d)
	}
	got := string(mustJSON(t, payload))
	if !strings.Contains(got, `"enabled_categories":[]`) {
		t.Errorf("payload %s must send a configured empty set as [] to clear", got)
	}
	if strings.Contains(got, "enabled_networks") {
		t.Errorf("payload %s must omit the null enabled_networks (unmanaged)", got)
	}
}

// TestThreatManagementSchemaValidators exercises the schema's validators and
// plan-modifiers (Definition of Done): the enum validators reject out-of-set
// values and accept members, the category set gates each element against the
// allowlist, and every Optional+Computed knob wires UseStateForUnknown.
func TestThreatManagementSchemaValidators(t *testing.T) {
	var resp resource.SchemaResponse
	NewThreatManagementResource().(*threatManagementResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}

	assertStringEnum(t, resp.Schema.Attributes["mode"], "mode", "ipsInline", "bogus-mode")
	assertStringEnum(t, resp.Schema.Attributes["advanced_filtering_preference"], "advanced_filtering_preference", "manual", "nope")

	catAttr, ok := resp.Schema.Attributes["enabled_categories"].(schema.SetAttribute)
	if !ok {
		t.Fatalf("enabled_categories is not a SetAttribute")
	}
	if d := runSetValidators(catAttr.Validators, sliceToStringSet([]string{"botcc"})); d.HasError() {
		t.Errorf("enabled_categories rejected a valid category: %v", d)
	}
	if d := runSetValidators(catAttr.Validators, sliceToStringSet([]string{"not-a-category"})); !d.HasError() {
		t.Errorf("enabled_categories accepted an unknown category")
	}

	for _, name := range []string{
		"mode", "enabled_categories", "enabled_networks", "advanced_filtering_preference",
		"restrict_torrents", "honeypot_enabled", "memory_optimized", "content_filtering_blocking_page_enabled",
	} {
		if planModifierCount(resp.Schema.Attributes[name]) == 0 {
			t.Errorf("%s has no plan modifier (expected UseStateForUnknown)", name)
		}
	}
}

// assertStringEnum runs a StringAttribute's validators against a known-good and a
// known-bad value, asserting the bad one is rejected and the good one passes.
func assertStringEnum(t *testing.T, a schema.Attribute, name, good, bad string) {
	t.Helper()
	sa, ok := a.(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a StringAttribute", name)
	}
	run := func(v string) diag.Diagnostics {
		req := validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringValue(v)}
		resp := &validator.StringResponse{}
		for _, vd := range sa.Validators {
			vd.ValidateString(context.Background(), req, resp)
		}
		return resp.Diagnostics
	}
	if d := run(good); d.HasError() {
		t.Errorf("%s rejected valid %q: %v", name, good, d)
	}
	if d := run(bad); !d.HasError() {
		t.Errorf("%s accepted invalid %q", name, bad)
	}
}

func runSetValidators(vs []validator.Set, val types.Set) diag.Diagnostics {
	req := validator.SetRequest{Path: path.Root("enabled_categories"), ConfigValue: val}
	resp := &validator.SetResponse{}
	for _, v := range vs {
		v.ValidateSet(context.Background(), req, resp)
	}
	return resp.Diagnostics
}

func planModifierCount(a schema.Attribute) int {
	switch t := a.(type) {
	case schema.StringAttribute:
		return len(t.PlanModifiers)
	case schema.SetAttribute:
		return len(t.PlanModifiers)
	case schema.BoolAttribute:
		return len(t.PlanModifiers)
	default:
		return 0
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestThreatManagementCategoriesComplete guards the validator member set against
// the controller's documented `enabled_categories` enum (../unifi-specs
// objects/setting.json, ips section: 59 categories).
func TestThreatManagementCategoriesComplete(t *testing.T) {
	if len(ipsCategories) != 59 {
		t.Errorf("ipsCategories has %d entries, want 59 (the controller's documented enum)", len(ipsCategories))
	}
	seen := map[string]bool{}
	for _, c := range ipsCategories {
		if seen[c] {
			t.Errorf("duplicate category %q", c)
		}
		seen[c] = true
	}
}

// TestThreatManagementVerifyEnabled covers the subscription check: a configured
// non-"disabled" mode that reads back "disabled", or a configured non-empty
// category/network set that reads back empty — the controller's silent refusal
// without an active CyberSecure subscription — must produce a diagnostic; every
// honored or non-enabling case must not.
func TestThreatManagementVerifyEnabled(t *testing.T) {
	nullSet := types.SetNull(types.StringType)
	cats := func(ss ...string) types.Set { return sliceToStringSet(ss) }
	mk := func(mode string, c types.Set) *threatManagementModel {
		m := &threatManagementModel{EnabledCategories: c, EnabledNetworks: nullSet}
		if mode == "" {
			m.Mode = types.StringNull()
		} else {
			m.Mode = types.StringValue(mode)
		}
		return m
	}
	cases := []struct {
		name        string
		want, store *threatManagementModel
		wantError   bool
	}{
		{"unconfigured mode", mk("", nullSet), mk("disabled", nullSet), false},
		{"disabled honored", mk("disabled", nullSet), mk("disabled", nullSet), false},
		{"ids honored", mk("ids", nullSet), mk("ids", nullSet), false},
		{"ids refused (no subscription)", mk("ids", nullSet), mk("disabled", nullSet), true},
		{"ipsInline refused", mk("ipsInline", nullSet), mk("disabled", nullSet), true},
		{"categories honored", mk("ids", cats("botcc")), mk("ids", cats("botcc")), false},
		{"category-only refused (mode stays disabled)", mk("disabled", cats("botcc")), mk("disabled", cats()), true},
		{"empty configured categories not a refusal", mk("disabled", cats()), mk("disabled", cats()), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d diag.Diagnostics
			verifyThreatManagementEnabled(tc.want, tc.store, &d)
			if got := d.HasError(); got != tc.wantError {
				t.Errorf("HasError() = %v, want %v (diags: %v)", got, tc.wantError, d)
			}
		})
	}
}

// TestThreatManagementWantsEnable covers the plan-time gate predicate: only a
// non-"disabled" mode or a non-empty category/network selection counts as
// "enabling", so the disabled/plumbing path skips the IPS-capability probe.
func TestThreatManagementWantsEnable(t *testing.T) {
	nullSet := types.SetNull(types.StringType)
	cases := []struct {
		name string
		m    threatManagementModel
		want bool
	}{
		{"disabled, no sets", threatManagementModel{Mode: types.StringValue("disabled"), EnabledCategories: nullSet, EnabledNetworks: nullSet}, false},
		{"null mode, empty sets", threatManagementModel{Mode: types.StringNull(), EnabledCategories: sliceToStringSet([]string{}), EnabledNetworks: nullSet}, false},
		{"ids mode", threatManagementModel{Mode: types.StringValue("ids"), EnabledCategories: nullSet, EnabledNetworks: nullSet}, true},
		{"disabled but categories set", threatManagementModel{Mode: types.StringValue("disabled"), EnabledCategories: sliceToStringSet([]string{"botcc"}), EnabledNetworks: nullSet}, true},
		{"disabled but networks set", threatManagementModel{Mode: types.StringValue("disabled"), EnabledCategories: nullSet, EnabledNetworks: sliceToStringSet([]string{"net-1"})}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := threatManagementWantsEnable(&tc.m); got != tc.want {
				t.Errorf("threatManagementWantsEnable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func setStrings(t *testing.T, s types.Set) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	var vals []string
	if diags := s.ElementsAs(context.Background(), &vals, false); diags.HasError() {
		t.Fatalf("ElementsAs: %v", diags)
	}
	for _, v := range vals {
		out[v] = true
	}
	return out
}
