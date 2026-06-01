package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

// isNotFoundErr reports whether err is the client's not-found sentinel, so Read
// can remove the resource from state (invariant 5: remove-on-404).
func isNotFoundErr(err error) bool {
	return errors.Is(err, unifi.ErrNotFound)
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

// strOrEmpty returns the string value, or "" for null/unknown (so omitempty
// drops it from the request body).
func strOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
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
