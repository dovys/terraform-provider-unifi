package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

var (
	_ resource.Resource                = (*protectAutomationResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectAutomationResource)(nil)
	_ resource.ResourceWithImportState = (*protectAutomationResource)(nil)
)

func NewProtectAutomationResource() resource.Resource { return &protectAutomationResource{} }

type protectAutomationResource struct {
	protect *protect.Client
}

type protectAutomationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	HeaderName  types.String `tfsdk:"header_name"`
	HeaderValue types.String `tfsdk:"header_value"` // write-only: lives in config, never state
}

func (r *protectAutomationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_automation"
}

func (r *protectAutomationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one webhook header on an **existing** UniFi Protect automation's `HTTP_REQUEST` action " +
			"(`/proxy/protect/api/automations`) — e.g. a rotating auth header (`X-Token`) sourced from a secret manager. " +
			"Import/update-only: the automation is created in the Protect UI; `terraform destroy` only removes it from state " +
			"(no per-id delete — `DELETE` wipes all automations). The target `HTTP_REQUEST` action must be unique. `header_value` " +
			"is **write-only** (never stored in or read back from state, so out-of-band changes aren't detected); rotate it with " +
			"`terraform apply -replace`. **Writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Protect automation object id (24-hex). Import key is `<id>/<header_name>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"header_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the webhook header on the `HTTP_REQUEST` action (e.g. `X-Token`). " +
					"Other headers on the action are left untouched.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"header_value": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				WriteOnly:           true,
				MarkdownDescription: "Value to set for `header_name`. Write-only: sent on apply but never stored in or read back from state.",
			},
		},
	}
}

func (r *protectAutomationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		requireProtect(c, &resp.Diagnostics)
		r.protect = c.Protect()
	}
}

func (r *protectAutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config protectAutomationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)     // header_value is null here (write-only)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...) // write-only header_value lives here
	if resp.Diagnostics.HasError() {
		return
	}
	// The automation must already exist — there is no create API. Without an id
	// (or an import block) there is nothing to configure.
	if plan.ID.IsUnknown() || plan.ID.IsNull() || strings.TrimSpace(plan.ID.ValueString()) == "" {
		resp.Diagnostics.AddError("Missing automation id",
			"Set `id` to an existing Protect automation (created in the Protect UI), or adopt it with an `import {}` block.")
		return
	}
	r.applyConfig(ctx, &plan, config.HeaderValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectAutomationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectAutomationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	auto, err := r.protect.Automations().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read automation", err.Error())
		return
	}
	// Only id is reconciled: header_name is the user's selector and header_value
	// is write-only, so neither is read back from the controller.
	state.ID = types.StringValue(auto.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectAutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config protectAutomationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.applyConfig(ctx, &plan, config.HeaderValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond state removal: the automations route has no safe
// per-id delete (DELETE wipes all automations), and the object is user-owned.
func (r *protectAutomationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes `<id>/<header_name>` so the imported resource reaches a clean
// plan — the controller can't tell us which header the user owns. A bare id also
// works but leaves header_name to the first apply.
func (r *protectAutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, headerName, ok := strings.Cut(strings.TrimSpace(req.ID), "/")
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strings.TrimSpace(id))...)
	if ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("header_name"), strings.TrimSpace(headerName))...)
	}
}

// applyConfig sets header_name=headerValue on the automation's sole HTTP_REQUEST
// action and batch-PATCHes the full actions[] back — the controller replaces the
// array wholesale, so the other actions must be echoed unchanged.
func (r *protectAutomationResource) applyConfig(ctx context.Context, plan *protectAutomationResourceModel, headerValue types.String, diags *diag.Diagnostics) {
	auto, err := r.protect.Automations().Get(ctx, plan.ID.ValueString())
	if err != nil {
		diags.AddError("Failed to read automation before update", err.Error())
		return
	}
	actions, err := setHeaderOnHTTPAction(auto.Actions, plan.HeaderName.ValueString(), headerValue.ValueString())
	if err != nil {
		diags.AddError("Cannot apply automation header", err.Error())
		return
	}
	updated, err := r.protect.Automations().Update(ctx, plan.ID.ValueString(), automationConfig{Actions: actions})
	if err != nil {
		diags.AddError("Failed to apply automation header", err.Error())
		return
	}
	plan.ID = types.StringValue(updated.Id)
}

// automationConfig is the owned-field overlay sent on the batch PATCH. Only
// actions[] is owned; every other automation field is left untouched.
type automationConfig struct {
	Actions []interface{} `json:"actions"`
}

// setHeaderOnHTTPAction returns actions[] as untyped maps with name=value set on
// the sole HTTP_REQUEST action's metadata.headers (an array of [name, value]
// tuples; replaced if present, else appended). Errors unless exactly one
// HTTP_REQUEST action exists.
func setHeaderOnHTTPAction(actions []wire.AutomationAction, name, value string) ([]interface{}, error) {
	httpIdx := -1
	for i := range actions {
		if actions[i].Type != nil && string(*actions[i].Type) == "HTTP_REQUEST" {
			if httpIdx != -1 {
				return nil, fmt.Errorf("automation has multiple HTTP_REQUEST actions; cannot disambiguate which webhook header to manage")
			}
			httpIdx = i
		}
	}
	if httpIdx == -1 {
		return nil, fmt.Errorf("automation has no HTTP_REQUEST action to set a header on")
	}

	out := make([]interface{}, len(actions))
	for i := range actions {
		m := actionToMap(actions[i])
		if i == httpIdx {
			meta, _ := m["metadata"].(map[string]interface{})
			if meta == nil {
				meta = map[string]interface{}{}
			}
			meta["headers"] = upsertHeader(meta["headers"], name, value)
			m["metadata"] = meta
		}
		out[i] = m
	}
	return out, nil
}

// actionToMap renders a wire action as the untyped map the controller accepts,
// carrying every field back unchanged (metadata is itself untyped).
func actionToMap(a wire.AutomationAction) map[string]interface{} {
	m := map[string]interface{}{}
	if a.Type != nil {
		m["type"] = string(*a.Type)
	}
	if a.Action != nil {
		m["action"] = *a.Action
	}
	if a.Order != nil {
		m["order"] = *a.Order
	}
	if a.Metadata != nil {
		meta := map[string]interface{}{}
		for k, v := range *a.Metadata {
			meta[k] = v
		}
		m["metadata"] = meta
	}
	return m
}

// upsertHeader replaces or appends the [name, value] tuple in a headers array
// (untyped []interface{} of two-element []interface{} tuples).
func upsertHeader(raw interface{}, name, value string) []interface{} {
	pair := []interface{}{name, value}
	existing, _ := raw.([]interface{})
	out := make([]interface{}, 0, len(existing)+1)
	replaced := false
	for _, h := range existing {
		tuple, ok := h.([]interface{})
		if ok && len(tuple) >= 1 {
			if hn, ok := tuple[0].(string); ok && hn == name {
				out = append(out, pair)
				replaced = true
				continue
			}
		}
		out = append(out, h)
	}
	if !replaced {
		out = append(out, pair)
	}
	return out
}
