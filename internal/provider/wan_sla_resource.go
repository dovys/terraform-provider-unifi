package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// wanSLAResource is the first resource built on the rebuilt substruct client
// (c.Network().WAN().SLAs()) and the unifi_<app>_<group>_<resource> naming
// standard: app=network, group=wan, resource=sla. A WAN SLA is a failover
// health-monitor profile (which targets to probe, how often, the down
// threshold) referenced by WAN load-balancing.
type wanSLAResource struct {
	net *network.Client
}

var (
	_ resource.Resource                = (*wanSLAResource)(nil)
	_ resource.ResourceWithConfigure   = (*wanSLAResource)(nil)
	_ resource.ResourceWithImportState = (*wanSLAResource)(nil)
)

// NewWANSLAResource is the resource factory registered with the provider.
func NewWANSLAResource() resource.Resource { return &wanSLAResource{} }

type wanSLAResourceModel struct {
	ID                   types.String         `tfsdk:"id"`
	Name                 types.String         `tfsdk:"name"`
	MonitorHealthType    types.String         `tfsdk:"monitor_health_type"`
	MonitorDownThreshold types.Int64          `tfsdk:"monitor_down_threshold"`
	Monitors             []wanSLAMonitorModel `tfsdk:"monitors"`
}

type wanSLAMonitorModel struct {
	Target     types.String `tfsdk:"target"`
	Type       types.String `tfsdk:"type"`
	Interval   types.Int64  `tfsdk:"interval"`
	Domain     types.String `tfsdk:"domain"`
	TimePeriod types.Int64  `tfsdk:"time_period"`
}

func (r *wanSLAResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wan_sla"
}

func (r *wanSLAResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi WAN failover SLA (health-monitor) profile (`wan-slas`, v2) — the probes " +
			"and thresholds that decide whether a WAN is healthy, referenced by WAN load balancing.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key — `terraform import` also accepts a friendly `name=<SLA name>` key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SLA profile name.",
			},
			"monitor_health_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How monitor results combine into health (controller-defined).",
			},
			"monitor_down_threshold": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Consecutive failed probes before the WAN is considered down.",
			},
			"monitors": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Probes that make up this SLA.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"target": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Probe target (IP/host).",
						},
						"type": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Probe type: `icmp`, `dns`, or `speedLimit`.",
							Validators:          []validator.String{stringvalidator.OneOf("icmp", "dns", "speedLimit")},
						},
						"interval": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Seconds between probes.",
						},
						"domain": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Domain to resolve (for `dns` probes).",
						},
						"time_period": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Evaluation window in seconds.",
						},
					},
				},
			},
		},
	}
}

func (r *wanSLAResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics)
	if !ok {
		return
	}
	r.net = c.Network()
}

// modelToWire builds the wire SLA from the plan. omitempty + pointer helpers
// give three-state semantics (unset → omit, set → write).
func (m wanSLAResourceModel) toWire() *wire.WanSlas {
	sla := &wire.WanSlas{
		Name:                 strPtrOrNil(m.Name),
		MonitorHealthType:    (*wire.WanSlasMonitorHealthType)(strPtrOrNil(m.MonitorHealthType)),
		MonitorDownThreshold: intPtr(m.MonitorDownThreshold),
	}
	if m.Monitors != nil {
		mons := make([]wire.WanSlasMonitor, 0, len(m.Monitors))
		for _, mon := range m.Monitors {
			mons = append(mons, wire.WanSlasMonitor{
				Target:     strPtrOrNil(mon.Target),
				Type:       (*wire.WanSlasMonitorType)(strPtrOrNil(mon.Type)),
				Interval:   intPtr(mon.Interval),
				Domain:     strPtrOrNil(mon.Domain),
				TimePeriod: intPtr(mon.TimePeriod),
			})
		}
		sla.Monitors = &mons
	}
	return sla
}

// wireToModel maps a wire SLA into resource state.
func wireToWANSLAModel(sla *wire.WanSlas) wanSLAResourceModel {
	m := wanSLAResourceModel{
		ID:                   strToStateOptional(deref(sla.UnderscoreId)),
		Name:                 strToStateOptional(deref(sla.Name)),
		MonitorHealthType:    strToStateOptional(string(derefEnum(sla.MonitorHealthType))),
		MonitorDownThreshold: intToState(sla.MonitorDownThreshold),
	}
	if sla.Monitors != nil {
		for _, mon := range *sla.Monitors {
			m.Monitors = append(m.Monitors, wanSLAMonitorModel{
				Target:     strToStateOptional(deref(mon.Target)),
				Type:       strToStateOptional(string(derefEnum(mon.Type))),
				Interval:   intToState(mon.Interval),
				Domain:     strToStateOptional(deref(mon.Domain)),
				TimePeriod: intToState(mon.TimePeriod),
			})
		}
	}
	return m
}

func (r *wanSLAResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wanSLAResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.WAN().SLAs().Create(ctx, plan.toWire())
	if err != nil {
		resp.Diagnostics.AddError("Creating WAN SLA failed", err.Error())
		return
	}
	state := wireToWANSLAModel(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wanSLAResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wanSLAResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	slas, err := r.net.WAN().SLAs().List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading WAN SLA failed", err.Error())
		return
	}
	for i := range slas {
		if slas[i].UnderscoreId != nil && *slas[i].UnderscoreId == id {
			fresh := wireToWANSLAModel(&slas[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &fresh)...)
			return
		}
	}
	resp.State.RemoveResource(ctx) // remove-on-404 (invariant 5)
}

func (r *wanSLAResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state wanSLAResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sla := plan.toWire()
	id := state.ID.ValueString()
	sla.UnderscoreId = &id
	updated, err := r.net.WAN().SLAs().Update(ctx, sla)
	if err != nil {
		resp.Diagnostics.AddError("Updating WAN SLA failed", err.Error())
		return
	}
	newState := wireToWANSLAModel(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *wanSLAResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wanSLAResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.WAN().SLAs().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Deleting WAN SLA failed", err.Error())
	}
}

// ImportState accepts the controller `_id` or a friendly `name=<SLA name>` key,
// resolved to the `_id`.
func (r *wanSLAResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByNameOrID(ctx, req, resp, "WAN SLA", func(ctx context.Context, name string) (string, error) {
		sla, err := r.net.WAN().SLAs().GetByName(ctx, name)
		if err != nil {
			return "", err
		}
		return deref(sla.UnderscoreId), nil
	})
}

// small helpers shared with the rebuilt resources.

func strPtrOrNil(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func boolPtrOrNil(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// intToFloatPtr bridges an Int64 attribute to a wire *float64 (Protect models
// integer-valued settings as JSON numbers); nil for null/unknown.
func intToFloatPtr(v types.Int64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := float64(v.ValueInt64())
	return &f
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefEnum[T ~string](p *T) T {
	if p == nil {
		var z T
		return z
	}
	return *p
}
