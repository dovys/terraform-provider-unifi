package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/console"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
)

// isNotFoundErr reports whether err is a client not-found sentinel, so Read can
// remove the resource from state (invariant 5: remove-on-404). It matches the
// console client and every substruct client (Network, Protect, Access) — each
// exposes its own distinct ErrNotFound, so all must be listed here.
func isNotFoundErr(err error) bool {
	return errors.Is(err, unifi.ErrNotFound) ||
		errors.Is(err, network.ErrNotFound) ||
		errors.Is(err, protect.ErrNotFound) ||
		errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, console.ErrNotFound)
}

// requireConsoleCert adds a diagnostic when the unifi-core certificate surface
// did not respond to the Configure-time probe (older firmware without
// /api/userCertificates). The console_certificate resource calls it so an
// unsupported console surfaces as a clear plan-time message instead of a raw 4xx
// (invariant 3).
func requireConsoleCert(c unifi.Client, diags *diag.Diagnostics) {
	if caps := c.Capabilities(); caps == nil || !caps.ConsoleCertSupported {
		diags.AddError(
			"Console certificate management is not available",
			"This resource manages the UniFi OS console TLS certificate, but the console's certificate API "+
				"(/api/userCertificates) did not respond on the configured console. This usually means the UniFi OS "+
				"firmware is too old. Upgrade UniFi OS, or remove unifi_console_certificate from your configuration.",
		)
	}
}

// clientFromProviderData extracts the configured unifi.Client from the value the
// provider's Configure stored. providerData is nil during early validation
// walks — callers return without error in that case.
func clientFromProviderData(providerData any, diags *diag.Diagnostics) (unifi.Client, bool) {
	if providerData == nil {
		return nil, false
	}
	c, ok := providerData.(unifi.Client)
	if !ok {
		diags.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected unifi.Client, got %T. This is a provider bug.", providerData),
		)
		return nil, false
	}
	return c, true
}

// requireProtect adds a diagnostic when the UniFi Protect application is not
// available on the configured console (per the Configure-time probe). Protect
// resources/data sources call it from Configure so an absent Protect surfaces
// as a clear plan-time message instead of a raw 4xx on the first call
// (invariant 3).
func requireProtect(c unifi.Client, diags *diag.Diagnostics) {
	if caps := c.Capabilities(); caps == nil || !caps.ProtectInstalled {
		diags.AddError(
			"UniFi Protect is not available",
			"This resource manages a UniFi Protect device, but the Protect application did not respond on the "+
				"configured console. Install or enable Protect on the console, or remove the Protect resources from your configuration.",
		)
	}
}

// --- type conversions between framework attribute values and Go pointers ---
//
// Pointer round-trips give three-state semantics: a null/unknown attribute maps
// to a nil pointer (leave the controller value), a set attribute to a non-nil
// pointer (write it, including an explicit false/0).

func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func boolToState(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

func intPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := int(v.ValueInt64())
	return &i
}

func intToState(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func int64PtrToState(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}

// f64PtrToState maps a wire *float64 onto a Float64 attribute, preserving
// fractional values (used for sensor thresholds where truncation would corrupt
// e.g. a 20.5°C setting).
func f64PtrToState(p *float64) types.Float64 {
	if p == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*p)
}

// float64ToState bridges the Protect wire types, which model integer-valued
// settings (mic volume, durations, brightness) as JSON numbers (*float64),
// against Int64 schema attributes. float64 is exact for integers through 2^53,
// so the round-trip is lossless for these values. The write direction uses
// mergeFloat (protect_common.go), which merges the plan over the current value.
func float64ToState(p *float64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// strOrEmpty returns the string value, or "" for null/unknown (so omitempty
// drops it from the request body).
func strOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// strPtrIfSet returns a pointer to the string only when it is set to a non-empty
// value, else nil — so omitempty drops it. Use it for fields the controller
// rejects when sent empty (e.g. wan_ip on a non-static WAN): an unset field must
// be omitted, not sent as "".
func strPtrIfSet(v types.String) *string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return nil
	}
	s := v.ValueString()
	return &s
}

// strToStateOptional maps an empty API string to null, so an unset optional
// attribute stays unset across reads.
func strToStateOptional(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// stringListToSlice reads a framework list of strings into a Go slice, dropping
// empty entries.
func stringListToSlice(ctx context.Context, l types.List, diags *diag.Diagnostics) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

// sliceToStringList builds a framework list of strings (null when empty).
func sliceToStringList(items []string) types.List {
	if len(items) == 0 {
		return types.ListNull(types.StringType)
	}
	elems := make([]attr.Value, 0, len(items))
	for _, s := range items {
		elems = append(elems, types.StringValue(s))
	}
	l, _ := types.ListValue(types.StringType, elems)
	return l
}

// stringSetToSlice reads a framework set of strings into a Go slice. Sets model
// unordered collections (MAC lists, band lists, id references) so controller
// reordering doesn't churn the plan.
func stringSetToSlice(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out
}

// sliceToStringSet builds a framework set of strings. A nil slice (the field was
// absent on the wire) maps to null; a non-nil empty slice (the controller
// returned an explicit `[]`) maps to an empty set — so an attribute configured
// as `[]` reads back as `[]`, not null, and doesn't trip the post-apply
// consistency check.
func sliceToStringSet(items []string) types.Set {
	if items == nil {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, 0, len(items))
	for _, s := range items {
		elems = append(elems, types.StringValue(s))
	}
	s, _ := types.SetValue(types.StringType, elems)
	return s
}

// intSetToSlice decodes a framework set of Int64 into a []int (nil for a
// null/unknown set), the int analogue of stringSetToSlice.
func intSetToSlice(ctx context.Context, s types.Set, diags *diag.Diagnostics) []int {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var vals []int64
	diags.Append(s.ElementsAs(ctx, &vals, false)...)
	out := make([]int, 0, len(vals))
	for _, v := range vals {
		out = append(out, int(v))
	}
	return out
}

// sliceToIntSet builds a framework set of Int64, mirroring sliceToStringSet's
// nil→null / non-nil-empty→empty-set distinction.
func sliceToIntSet(items []int) types.Set {
	if items == nil {
		return types.SetNull(types.Int64Type)
	}
	elems := make([]attr.Value, 0, len(items))
	for _, n := range items {
		elems = append(elems, types.Int64Value(int64(n)))
	}
	s, _ := types.SetValue(types.Int64Type, elems)
	return s
}
