package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

var (
	_ resource.Resource                = (*protectViewerResource)(nil)
	_ resource.ResourceWithConfigure   = (*protectViewerResource)(nil)
	_ resource.ResourceWithImportState = (*protectViewerResource)(nil)
)

func NewProtectViewerResource() resource.Resource { return &protectViewerResource{} }

type protectViewerResource struct {
	protect *protect.Client
}

type protectViewerResourceModel struct {
	ID  types.String `tfsdk:"id"`
	MAC types.String `tfsdk:"mac"`

	Name     types.String `tfsdk:"name"`
	Liveview types.String `tfsdk:"liveview"`

	ForgetOnDestroy types.Bool `tfsdk:"forget_on_destroy"`

	Model       types.String `tfsdk:"model"`
	State       types.String `tfsdk:"state"`
	Host        types.String `tfsdk:"host"`
	IsAdopted   types.Bool   `tfsdk:"is_adopted"`
	StreamLimit types.Int64  `tfsdk:"stream_limit"`
}

func (r *protectViewerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_protect_viewer"
}

func (r *protectViewerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi Protect Viewer (UP-Viewport), identified by MAC. Configures the existing, adopted device over the " +
			"private Protect API (`/proxy/protect/api/viewers/<id>`): name and the live view pushed to its display. " +
			"`terraform destroy` removes the viewer from state and leaves it configured; set `forget_on_destroy = true` to unadopt it. " +
			"**Config writes are HIL-unverified — see the provider TODO.**",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protect object id (24-hex). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Viewer MAC address (the natural key); any separator/case is accepted and normalized. The device must be adopted. Immutable — changing it replaces the resource.",
				PlanModifiers:       []planmodifier.String{normalizeMAC(), stringplanmodifier.RequiresReplace()},
			},
			"name":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Viewer display name."},
			"liveview": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Object id of the `unifi_protect_liveview` pushed to this viewer's display."},
			"forget_on_destroy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When `true`, `terraform destroy` unadopts (forgets) the viewer instead of just removing it from state. Defaults to `false`.",
			},

			"model":        schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model."},
			"state":        schema.StringAttribute{Computed: true, MarkdownDescription: "Connection state."},
			"host":         schema.StringAttribute{Computed: true, MarkdownDescription: "Current management IP/host."},
			"is_adopted":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the viewer is adopted by this console."},
			"stream_limit": schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum number of parallel live streams the viewer supports."},
		},
	}
}

func (r *protectViewerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.protect = c.Protect()
	}
}

func (r *protectViewerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan protectViewerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac := strings.TrimSpace(plan.MAC.ValueString())
	current, err := r.protect.Viewers().GetByMAC(ctx, mac)
	if err != nil {
		if isNotFoundErr(err) {
			resp.Diagnostics.AddError("Viewer not found",
				fmt.Sprintf("No viewer with MAC %q is adopted on this Protect console. Adopt it first.", mac))
			return
		}
		resp.Diagnostics.AddError("Failed to look up viewer", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectViewerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state protectViewerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := r.protect.Viewers().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read viewer", err.Error())
		return
	}
	state.fromViewer(v)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *protectViewerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state protectViewerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.protect.Viewers().Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read viewer before update", err.Error())
		return
	}
	r.applyConfig(ctx, &plan, current, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *protectViewerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state protectViewerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ForgetOnDestroy.ValueBool() {
		if err := r.protect.Viewers().Delete(ctx, state.ID.ValueString()); err != nil && !isNotFoundErr(err) {
			resp.Diagnostics.AddError("Failed to unadopt viewer", err.Error())
		}
	}
}

func (r *protectViewerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if strings.Contains(id, ":") {
		v, err := r.protect.Viewers().GetByMAC(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Import failed", fmt.Sprintf("no viewer with MAC %q: %s", id, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), v.Id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), macCanonical(v.Mac))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *protectViewerResource) applyConfig(ctx context.Context, plan *protectViewerResourceModel, current *wire.Viewer, diags *diag.Diagnostics) {
	cfg := viewerConfig{
		Name:     mergeStr(plan.Name, current.Name),
		Liveview: mergeStr(plan.Liveview, current.Liveview),
	}
	updated, err := r.protect.Viewers().Update(ctx, current.Id, cfg)
	if err != nil {
		diags.AddError("Failed to apply viewer config", err.Error())
		return
	}
	plan.fromViewer(updated)
}

func (m *protectViewerResourceModel) fromViewer(v *wire.Viewer) {
	m.ID = types.StringValue(v.Id)
	if m.MAC.IsNull() || m.MAC.IsUnknown() {
		m.MAC = types.StringValue(macCanonical(v.Mac))
	}
	m.Name = strToStateOptional(deref(v.Name))
	m.Liveview = strToStateOptional(deref(v.Liveview))

	m.Model = strToStateOptional(deref(v.MarketName))
	m.State = types.StringValue(string(derefEnum(v.State)))
	m.Host = strToStateOptional(deref(v.Host))
	m.IsAdopted = boolToState(v.IsAdopted)
	m.StreamLimit = float64ToState(v.StreamLimit)
}

// --- owned-field overlay (omitempty) sent on PATCH ---

type viewerConfig struct {
	Name     *string `json:"name,omitempty"`
	Liveview *string `json:"liveview,omitempty"`
}
