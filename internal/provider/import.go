package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// importByNameOrID resolves a resource import identifier and stores it in the
// "id" attribute. It accepts either the raw controller `_id` (the resource's
// state id, which always works) or a friendly `name=<value>` form, which it
// resolves to the `_id` via lookup. The UniFi UI does not surface `_id`, so the
// name form is the discoverable key for `import {}` blocks / `terraform import`.
//
// kind is the human label used in diagnostics (e.g. "network", "WAN SLA").
func importByNameOrID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, kind string, lookup func(context.Context, string) (string, error)) {
	id := strings.TrimSpace(req.ID)
	if name, ok := importName(id); ok {
		if name == "" {
			resp.Diagnostics.AddError("Invalid import ID", `Import by name needs a value, e.g. "name=WAN1".`)
			return
		}
		resolved, err := lookup(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError(
				"Could not resolve import name",
				fmt.Sprintf("No %s found with name %q (%s). Import by the controller _id instead, or check the name.", kind, name, err),
			)
			return
		}
		id = resolved
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// importName reports whether the import id is the friendly `name=<value>` form
// and returns the trimmed value. A bare id (the raw controller `_id`) yields
// ("", false).
func importName(id string) (string, bool) {
	if v, ok := strings.CutPrefix(id, "name="); ok {
		return strings.TrimSpace(v), true
	}
	return "", false
}
