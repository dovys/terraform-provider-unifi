package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Protect writes are PATCH of the owned fields. The generated wire read types
// carry no omitempty, so a full read object would clobber controller-managed
// state on write; instead each owned (sub-)field is built by merging the plan
// over the current value, and the result serialized through an omitempty
// overlay. These helpers express that merge against the framework attr types:
// a set plan attribute wins; otherwise the current controller value is kept
// (so a field the provider models but the practitioner didn't configure is
// preserved, not cleared).

func mergeStr(plan types.String, cur *string) *string {
	if !plan.IsNull() && !plan.IsUnknown() {
		v := plan.ValueString()
		return &v
	}
	return cur
}

func mergeBool(plan types.Bool, cur *bool) *bool {
	if !plan.IsNull() && !plan.IsUnknown() {
		v := plan.ValueBool()
		return &v
	}
	return cur
}

func mergeFloat(plan types.Int64, cur *float64) *float64 {
	if !plan.IsNull() && !plan.IsUnknown() {
		v := float64(plan.ValueInt64())
		return &v
	}
	return cur
}

// macCanonical normalizes a MAC to lowercase colon-separated form (the form the
// Network app already uses), so a `mac` attribute compares equal regardless of
// the input separator/case and whatever form the controller returns. The
// Protect wire returns MACs colon-free; without this, an import (which writes
// the wire form) would diff against a colon-form config and force a replace.
func macCanonical(mac string) string {
	hex := normalizeHexMAC(mac)
	if len(hex) != 12 {
		return strings.ToLower(strings.TrimSpace(mac)) // leave unrecognized input as-is (lowercased)
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}

func normalizeHexMAC(mac string) string {
	r := strings.NewReplacer(":", "", "-", "", ".", "", " ", "")
	return strings.ToLower(r.Replace(strings.TrimSpace(mac)))
}

// macNormalizer canonicalizes a configured `mac` value at plan time, so the
// immutable attribute compares equal to the state form (also canonical) no
// matter how the practitioner wrote it. It must run before RequiresReplace in
// the plan-modifier list.
type macNormalizer struct{}

func normalizeMAC() planmodifier.String { return macNormalizer{} }

func (macNormalizer) Description(_ context.Context) string {
	return "Normalizes the MAC to lowercase colon-separated form."
}
func (macNormalizer) MarkdownDescription(_ context.Context) string {
	return "Normalizes the MAC to lowercase colon-separated form."
}
func (macNormalizer) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = types.StringValue(macCanonical(req.ConfigValue.ValueString()))
}

func mergeStrSet(ctx context.Context, plan types.Set, cur []string, diags *diag.Diagnostics) []string {
	if !plan.IsNull() && !plan.IsUnknown() {
		return stringSetToSlice(ctx, plan, diags)
	}
	return cur
}

// ifaceStrings reads a Protect wire `*[]interface{}` (the generator's type for
// an untyped JSON string array, e.g. smart-detect object/audio types) into a
// []string, dropping non-string and nil entries.
func ifaceStrings(p *[]interface{}) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(*p))
	for _, v := range *p {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
