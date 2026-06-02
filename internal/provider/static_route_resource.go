package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = (*staticRouteResource)(nil)
	_ resource.ResourceWithConfigure   = (*staticRouteResource)(nil)
	_ resource.ResourceWithImportState = (*staticRouteResource)(nil)
)

func NewStaticRouteResource() resource.Resource { return &staticRouteResource{} }

type staticRouteResource struct {
	net *network.Client
}

type staticRouteResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	Network   types.String `tfsdk:"network"`    // destination CIDR
	RouteType types.String `tfsdk:"route_type"` // nexthop-route | interface-route | blackhole
	NextHop   types.String `tfsdk:"next_hop"`   // gateway IP (nexthop-route)
	Distance  types.Int64  `tfsdk:"distance"`   // administrative distance
	Interface types.String `tfsdk:"interface"`  // egress interface (interface-route)
}

func (r *staticRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_static_route"
}

func (r *staticRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi static route (`routing`, legacy `/rest`) — a user-defined route on the gateway: " +
			"send traffic for a destination prefix via a next-hop gateway, out an interface, or to a blackhole.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Route name.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the route is active. Defaults to `true`.",
			},
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Destination network in CIDR notation (e.g. `192.168.50.0/24`).",
			},
			"route_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Route kind: `nexthop-route` (via `next_hop`), `interface-route` (out `interface`), " +
					"or `blackhole` (drop). Defaults to `nexthop-route`.",
				Validators: []validator.String{stringvalidator.OneOf("nexthop-route", "interface-route", "blackhole")},
			},
			"next_hop": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Next-hop gateway IP. Required when `route_type = nexthop-route`.",
			},
			"distance": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Administrative distance (route priority; lower wins). Defaults to `1`.",
			},
			"interface": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Egress interface. Used when `route_type = interface-route`.",
			},
		},
	}
}

func (r *staticRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *staticRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan staticRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.StaticRoutes().Create(ctx, plan.toRouting())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create static route", err.Error())
		return
	}
	plan.fromRouting(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *staticRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state staticRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rt, err := r.net.StaticRoutes().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read static route", err.Error())
		return
	}
	state.fromRouting(rt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *staticRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan staticRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.StaticRoutes().Update(ctx, plan.toRouting())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update static route", err.Error())
		return
	}
	plan.fromRouting(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *staticRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state staticRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.StaticRoutes().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete static route", err.Error())
	}
}

func (r *staticRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *staticRouteResourceModel) toRouting() *wire.Routing {
	staticType := "static-route" // the controller's discriminator for this doc
	return &wire.Routing{
		UnderscoreId:         strPtrOrNil(m.ID), // nil on create, set on update
		Name:                 strPtrOrNil(m.Name),
		Enabled:              boolPtr(m.Enabled),
		Type:                 &staticType,
		StaticRouteNetwork:   strPtrOrNil(m.Network),
		StaticRouteNexthop:   strPtrOrNil(m.NextHop),
		StaticRouteDistance:  intPtr(m.Distance),
		StaticRouteType:      (*wire.RoutingStaticRouteType)(strPtrOrNil(m.RouteType)),
		StaticRouteInterface: strPtrOrNil(m.Interface),
	}
}

func (m *staticRouteResourceModel) fromRouting(rt *wire.Routing) {
	m.ID = types.StringValue(deref(rt.UnderscoreId))
	m.Name = types.StringValue(deref(rt.Name))
	m.Enabled = boolToState(rt.Enabled)
	m.Network = strToStateOptional(deref(rt.StaticRouteNetwork))
	m.RouteType = strToStateOptional(string(derefEnum(rt.StaticRouteType)))
	m.NextHop = strToStateOptional(deref(rt.StaticRouteNexthop))
	m.Distance = intToState(rt.StaticRouteDistance)
	m.Interface = strToStateOptional(deref(rt.StaticRouteInterface))
}
