package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                   = (*mdnsResource)(nil)
	_ resource.ResourceWithConfigure      = (*mdnsResource)(nil)
	_ resource.ResourceWithImportState    = (*mdnsResource)(nil)
	_ resource.ResourceWithValidateConfig = (*mdnsResource)(nil)
)

// mdnsID is the fixed state id of the mDNS-reflector singleton. The scope lives
// in the site's global network-config document (one per site), so the id is a
// constant rather than a controller-assigned _id.
const mdnsID = "mdns"

// mdnsModeSome is the scope mode where mdns_enabled_for_network_ids selects the
// participating networks; "all"/"none" ignore the id list.
const mdnsModeSome = string(wire.GlobalIgmpSnoopingForSome)

func NewMdnsResource() resource.Resource { return &mdnsResource{} }

type mdnsResource struct {
	net *network.Client
}

// mdnsModel is the mDNS-reflector scope: a mode plus the networks it spans when
// mode is "some". The scope is two fields of the site's global network-config
// document (mdns_enabled_for, mdns_enabled_for_network_ids); the resource owns
// only those two and leaves the document's IGMP/multicast fields untouched.
type mdnsModel struct {
	ID         types.String `tfsdk:"id"`
	Mode       types.String `tfsdk:"mode"`
	NetworkIDs types.Set    `tfsdk:"network_ids"`
}

func (r *mdnsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_mdns"
}

func (r *mdnsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Multicast DNS (mDNS) reflector scope for the site — a **singleton** on the site's " +
			"global network-config (`/v2/.../global/config/network`). The reflector bridges Bonjour/mDNS service " +
			"discovery (AirPlay, Chromecast, printers, …) across the selected networks (VLANs).\n\n" +
			"Set `mode` to `all` (every network), `none` (off), or `some` (only the networks in `network_ids`). " +
			"This resource owns only the mDNS scope; the document's other multicast knobs (IGMP snooping, " +
			"multicast flooding) are left as the controller has them.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed identifier of the mDNS-reflector singleton (`mdns`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mode": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Reflector scope: `all` (reflect across every network), `none` (reflector off), " +
					"or `some` (only the networks listed in `network_ids`).",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(wire.GlobalIgmpSnoopingForAll),
						string(wire.GlobalIgmpSnoopingForNone),
						string(wire.GlobalIgmpSnoopingForSome),
					),
				},
			},
			// Optional+Computed but deliberately NOT UseStateForUnknown: the computed
			// value depends on mode (for all/none it settles to null), so pinning the
			// prior value through a mode change would make the plan promise the stale
			// set and trip the planned-vs-applied consistency check. Leaving it unknown
			// when unset lets the apply settle it from the stored document.
			"network_ids": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Network (VLAN) `_id`s the reflector spans. Used only when `mode = \"some\"`; " +
					"ignored (and forced empty) for `all`/`none`.",
			},
		},
	}
}

// ValidateConfig rejects network_ids alongside a non-"some" mode: the controller
// ignores the id list unless mode is "some", so configuring both is a mistake the
// practitioner should see at plan time, not a silently-dropped value.
func (r *mdnsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg mdnsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMdnsConfig(&cfg, &resp.Diagnostics)
}

// validateMdnsConfig flags network_ids set alongside a non-"some" mode. Pulled
// out of ValidateConfig so it is unit-testable without a framework config.
func validateMdnsConfig(cfg *mdnsModel, diags *diag.Diagnostics) {
	if cfg.Mode.IsUnknown() || cfg.NetworkIDs.IsNull() || cfg.NetworkIDs.IsUnknown() {
		return
	}
	if cfg.Mode.ValueString() != mdnsModeSome && len(cfg.NetworkIDs.Elements()) > 0 {
		diags.AddAttributeError(
			path.Root("network_ids"),
			"network_ids requires mode = \"some\"",
			"network_ids selects the networks the reflector spans, which only applies when mode = \"some\". "+
				"Set mode = \"some\", or remove network_ids.",
		)
	}
}

func (r *mdnsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *mdnsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mdnsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the scope from the global network-config document.
func (r *mdnsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mdnsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	doc, err := r.net.GlobalNetwork().Get(ctx)
	if err != nil {
		if isNotFoundErr(err) {
			// The global-config surface is absent (older firmware): the scope this
			// resource manages no longer exists, so drop it from state.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read mDNS scope", err.Error())
		return
	}
	applyMdnsDocToModel(doc, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mdnsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan mdnsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond state removal: the global network-config singleton
// always exists. Removing the resource stops managing the mDNS scope; it is left
// as the controller last had it, not reverted.
func (r *mdnsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState seeds the singleton id; the next Read populates the scope from the
// controller.
func (r *mdnsResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), mdnsID)...)
}

// write reads the current global network-config document, overlays the mDNS
// scope (mode + network_ids), PUTs the whole document back (the v2 surface is
// replace-style, so the required IGMP fields must ride along untouched), then
// folds the controller's stored copy back into plan so Computed network_ids
// settles.
func (r *mdnsResource) write(ctx context.Context, plan *mdnsModel, diags *diag.Diagnostics) {
	doc, err := r.net.GlobalNetwork().Get(ctx)
	if err != nil {
		if isNotFoundErr(err) {
			diags.AddError(
				"mDNS reflector is not available",
				"This resource manages the site's mDNS reflector scope via the global network-config surface "+
					"(/v2/api/site/<site>/global/config/network), which did not respond on the configured console. "+
					"This usually means the UniFi Network firmware is too old. Upgrade UniFi Network, or remove "+
					"unifi_network_mdns from your configuration.",
			)
			return
		}
		diags.AddError("Failed to read mDNS scope", err.Error())
		return
	}

	overlayMdnsScope(ctx, plan, doc, diags)
	if diags.HasError() {
		return
	}

	stored, err := r.net.GlobalNetwork().Set(ctx, doc)
	if err != nil {
		diags.AddError("Failed to apply mDNS scope", err.Error())
		return
	}
	applyMdnsDocToModel(stored, plan)
}

// overlayMdnsScope writes the model's mode + network_ids onto the global
// network-config document, leaving every other field as Get returned it (the v2
// surface is replace-style, so the document must carry its required IGMP fields
// untouched). network_ids only applies to "some"; for all/none it forces an
// explicit empty list so the controller drops any stale selection (the field is
// required, not omitempty — never send a bare null).
func overlayMdnsScope(ctx context.Context, plan *mdnsModel, doc *wire.Global, diags *diag.Diagnostics) {
	mode := wire.GlobalIgmpSnoopingFor(plan.Mode.ValueString())
	doc.MdnsEnabledFor = mode
	ids := stringSetToSlice(ctx, plan.NetworkIDs, diags)
	if diags.HasError() {
		return
	}
	if mode != wire.GlobalIgmpSnoopingForSome {
		ids = nil
	}
	if ids == nil {
		ids = []string{}
	}
	doc.MdnsEnabledForNetworkIds = ids
}

// applyMdnsDocToModel folds the controller's stored scope into model. mode comes
// straight back; network_ids is normalized to null for non-"some" modes (the
// id list is irrelevant there) so the attribute doesn't churn against an omitted
// config.
func applyMdnsDocToModel(doc *wire.Global, model *mdnsModel) {
	model.ID = types.StringValue(mdnsID)
	model.Mode = types.StringValue(string(doc.MdnsEnabledFor))
	if doc.MdnsEnabledFor == wire.GlobalIgmpSnoopingForSome {
		model.NetworkIDs = sliceToStringSet(doc.MdnsEnabledForNetworkIds)
	} else {
		model.NetworkIDs = types.SetNull(types.StringType)
	}
}
