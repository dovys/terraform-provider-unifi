package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

var (
	_ resource.Resource                = (*networkSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*networkSettingsResource)(nil)
)

// networkSettingsID is the fixed state id of the site-settings singleton. There
// is exactly one settings object per site, so the id is constant rather than a
// controller-assigned _id (each section carries its own _id internally).
const networkSettingsID = "settings"

func NewNetworkSettingsResource() resource.Resource { return &networkSettingsResource{} }

type networkSettingsResource struct {
	net *network.Client
}

// networkSettingsModel is one resource covering many independent setting
// sections, each an Optional nested block. A null block is UNMANAGED: Read never
// populates it and apply never writes it, so declaring `ntp` does not pull `mgmt`
// (or any other key) into state. Removing a previously-declared block stops
// managing that section (it is left as the controller last had it — not
// reverted), which is the one semantic that differs from revert-on-remove.
type networkSettingsModel struct {
	ID types.String `tfsdk:"id"`

	NTP     types.Object `tfsdk:"ntp"`
	Country types.Object `tfsdk:"country"`
	Locale  types.Object `tfsdk:"locale"`
}

// --- per-key TF object models + their attribute-type maps ---

type ntpSettingsObject struct {
	SettingPreference types.String `tfsdk:"setting_preference"`
	Server1           types.String `tfsdk:"ntp_server_1"`
	Server2           types.String `tfsdk:"ntp_server_2"`
	Server3           types.String `tfsdk:"ntp_server_3"`
	Server4           types.String `tfsdk:"ntp_server_4"`
}

var ntpSettingsAttrTypes = map[string]attr.Type{
	"setting_preference": types.StringType,
	"ntp_server_1":       types.StringType,
	"ntp_server_2":       types.StringType,
	"ntp_server_3":       types.StringType,
	"ntp_server_4":       types.StringType,
}

type countrySettingsObject struct {
	Code types.Int64 `tfsdk:"code"`
}

var countrySettingsAttrTypes = map[string]attr.Type{"code": types.Int64Type}

type localeSettingsObject struct {
	Timezone types.String `tfsdk:"timezone"`
}

var localeSettingsAttrTypes = map[string]attr.Type{"timezone": types.StringType}

// --- per-key JSON overlays (omitempty: read the flat section doc, write only
// the configured fields per the shallow-merge contract) ---

type ntpSettingPayload struct {
	SettingPreference *string `json:"setting_preference,omitempty"`
	Server1           *string `json:"ntp_server_1,omitempty"`
	Server2           *string `json:"ntp_server_2,omitempty"`
	Server3           *string `json:"ntp_server_3,omitempty"`
	Server4           *string `json:"ntp_server_4,omitempty"`
}

type countrySettingPayload struct {
	Code *int `json:"code,omitempty"`
}

type localeSettingPayload struct {
	Timezone *string `json:"timezone,omitempty"`
}

// settingKeyHandler binds a model block to its controller key and the conversions
// between the framework object and the wire section. The Read/apply loops iterate
// the handler table so growing the resource to a new key is one table entry plus
// its schema block — the partial-read/partial-write mechanic is shared.
type settingKeyHandler struct {
	key       string
	attrTypes map[string]attr.Type
	block     func(*networkSettingsModel) *types.Object
	// toPayload returns the omitempty write overlay for a present block, or nil
	// for a null/unknown (unmanaged) block — so apply skips unmanaged sections.
	toPayload func(context.Context, types.Object, *diag.Diagnostics) any
	// fromRaw decodes a flat section document into the block's object value.
	fromRaw func(context.Context, json.RawMessage, *diag.Diagnostics) types.Object
}

func settingKeyHandlers() []settingKeyHandler {
	return []settingKeyHandler{
		{
			key:       "ntp",
			attrTypes: ntpSettingsAttrTypes,
			block:     func(m *networkSettingsModel) *types.Object { return &m.NTP },
			toPayload: func(ctx context.Context, o types.Object, d *diag.Diagnostics) any {
				if o.IsNull() || o.IsUnknown() {
					return nil
				}
				var obj ntpSettingsObject
				d.Append(o.As(ctx, &obj, basetypes.ObjectAsOptions{})...)
				return &ntpSettingPayload{
					SettingPreference: strPtrOrNil(obj.SettingPreference),
					Server1:           strPtrOrNil(obj.Server1),
					Server2:           strPtrOrNil(obj.Server2),
					Server3:           strPtrOrNil(obj.Server3),
					Server4:           strPtrOrNil(obj.Server4),
				}
			},
			fromRaw: func(ctx context.Context, raw json.RawMessage, d *diag.Diagnostics) types.Object {
				var p ntpSettingPayload
				if err := json.Unmarshal(raw, &p); err != nil {
					d.AddError("Failed to decode ntp setting", err.Error())
					return types.ObjectNull(ntpSettingsAttrTypes)
				}
				return objectFrom(ctx, ntpSettingsAttrTypes, ntpSettingsObject{
					SettingPreference: strToStateOptional(deref(p.SettingPreference)),
					Server1:           strToStateOptional(deref(p.Server1)),
					Server2:           strToStateOptional(deref(p.Server2)),
					Server3:           strToStateOptional(deref(p.Server3)),
					Server4:           strToStateOptional(deref(p.Server4)),
				}, d)
			},
		},
		{
			key:       "country",
			attrTypes: countrySettingsAttrTypes,
			block:     func(m *networkSettingsModel) *types.Object { return &m.Country },
			toPayload: func(ctx context.Context, o types.Object, d *diag.Diagnostics) any {
				if o.IsNull() || o.IsUnknown() {
					return nil
				}
				var obj countrySettingsObject
				d.Append(o.As(ctx, &obj, basetypes.ObjectAsOptions{})...)
				return &countrySettingPayload{Code: intPtr(obj.Code)}
			},
			fromRaw: func(ctx context.Context, raw json.RawMessage, d *diag.Diagnostics) types.Object {
				var p countrySettingPayload
				if err := json.Unmarshal(raw, &p); err != nil {
					d.AddError("Failed to decode country setting", err.Error())
					return types.ObjectNull(countrySettingsAttrTypes)
				}
				return objectFrom(ctx, countrySettingsAttrTypes, countrySettingsObject{
					Code: intToState(p.Code),
				}, d)
			},
		},
		{
			key:       "locale",
			attrTypes: localeSettingsAttrTypes,
			block:     func(m *networkSettingsModel) *types.Object { return &m.Locale },
			toPayload: func(ctx context.Context, o types.Object, d *diag.Diagnostics) any {
				if o.IsNull() || o.IsUnknown() {
					return nil
				}
				var obj localeSettingsObject
				d.Append(o.As(ctx, &obj, basetypes.ObjectAsOptions{})...)
				return &localeSettingPayload{Timezone: strPtrOrNil(obj.Timezone)}
			},
			fromRaw: func(ctx context.Context, raw json.RawMessage, d *diag.Diagnostics) types.Object {
				var p localeSettingPayload
				if err := json.Unmarshal(raw, &p); err != nil {
					d.AddError("Failed to decode locale setting", err.Error())
					return types.ObjectNull(localeSettingsAttrTypes)
				}
				return objectFrom(ctx, localeSettingsAttrTypes, localeSettingsObject{
					Timezone: strToStateOptional(deref(p.Timezone)),
				}, d)
			},
		},
	}
}

// objectFrom is a thin wrapper over ObjectValueFrom that funnels diagnostics and
// keeps the handler closures terse.
func objectFrom(ctx context.Context, attrTypes map[string]attr.Type, v any, d *diag.Diagnostics) types.Object {
	o, diags := types.ObjectValueFrom(ctx, attrTypes, v)
	d.Append(diags...)
	return o
}

// settingsPresentKeys lists the keys whose block is set (non-null, non-unknown)
// in m — exactly the sections Read refreshes and apply writes. A null block is
// excluded, which is the no-drift guarantee: managing one key never reads or
// writes another.
func settingsPresentKeys(m *networkSettingsModel) []string {
	var keys []string
	for _, h := range settingKeyHandlers() {
		if b := h.block(m); !b.IsNull() && !b.IsUnknown() {
			keys = append(keys, h.key)
		}
	}
	return keys
}

func (r *networkSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_settings"
}

func (r *networkSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Site-wide UniFi Network settings — a **singleton** whose sections are managed à la carte. " +
			"Each section is an **optional nested block**; declare only the sections you want Terraform to manage. " +
			"Omitting a block leaves that section unmanaged (Terraform never reads or writes it), and removing a " +
			"previously-declared block stops managing that section without reverting it. Writes shallow-merge the " +
			"configured fields over the controller's stored section, so unmodeled fields are preserved. " +
			"**Writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed identifier of the site-settings singleton (`settings`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ntp": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Time-synchronization settings (`ntp` section). Omit to leave NTP unmanaged.",
				Attributes: map[string]schema.Attribute{
					"setting_preference": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "`auto` (UniFi-provided servers) or `manual` (use the `ntp_server_*` fields).",
						Validators:          []validator.String{stringvalidator.OneOf("auto", "manual")},
					},
					"ntp_server_1": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Custom NTP server 1 (applies when `setting_preference = manual`)."},
					"ntp_server_2": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Custom NTP server 2."},
					"ntp_server_3": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Custom NTP server 3."},
					"ntp_server_4": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Custom NTP server 4."},
				},
			},
			"country": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Site country (`country` section). Omit to leave the country unmanaged.",
				Attributes: map[string]schema.Attribute{
					"code": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "ISO-3166 **numeric** country code (e.g. `840` = US, `826` = GB, `276` = DE).",
					},
				},
			},
			"locale": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Site locale (`locale` section). Omit to leave the locale unmanaged.",
				Attributes: map[string]schema.Attribute{
					"timezone": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "IANA timezone name (e.g. `America/New_York`, `Europe/London`).",
					},
				},
			},
		},
	}
}

func (r *networkSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *networkSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(networkSettingsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes only the sections present in state, leaving unmanaged (null)
// blocks null — managing one key never pulls another into state.
func (r *networkSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, h := range settingKeyHandlers() {
		blk := h.block(&state)
		if blk.IsNull() || blk.IsUnknown() {
			continue // unmanaged — do not populate
		}
		raw, err := r.net.Settings().GetKey(ctx, h.key)
		if err != nil {
			if isNotFoundErr(err) {
				*blk = types.ObjectNull(h.attrTypes)
				continue
			}
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to read %q setting", h.key), err.Error())
			return
		}
		*blk = h.fromRaw(ctx, raw, &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(networkSettingsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond state removal: site settings always exist; removing
// the resource stops managing the declared sections without reverting them.
func (r *networkSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState seeds the singleton id; the practitioner then declares the section
// blocks to manage and the next apply adopts-and-merges them.
func (r *networkSettingsResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), networkSettingsID)...)
}

// apply writes every present (managed) section via set/setting/<key> and folds
// the controller's stored copy back into the plan so Computed fields settle.
func (r *networkSettingsResource) apply(ctx context.Context, plan *networkSettingsModel, diags *diag.Diagnostics) {
	for _, h := range settingKeyHandlers() {
		blk := h.block(plan)
		payload := h.toPayload(ctx, *blk, diags)
		if diags.HasError() {
			return
		}
		if payload == nil {
			continue // unmanaged section
		}
		raw, err := r.net.Settings().SetKey(ctx, h.key, payload)
		if err != nil {
			diags.AddError(fmt.Sprintf("Failed to apply %q setting", h.key), err.Error())
			return
		}
		*blk = h.fromRaw(ctx, raw, diags)
		if diags.HasError() {
			return
		}
	}
}
