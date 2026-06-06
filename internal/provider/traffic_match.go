package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// Shared destination-matching attributes for the traffic resources
// (traffic_rule + traffic_route). The controller models domains / ip_addresses
// / ip_ranges as arrays of objects, not flat strings: a domain carries optional
// port and port-range refinements, an IP carries its version (v4/v6) plus the
// same port refinements, and an IP range is a typed start/stop pair. wire reuses
// one set of nested types (Trafficroutes*) across both resources, so the model
// structs, attribute types, schema builders, and converters below are shared.

// --- nested model structs (tfsdk-tagged) ---

type trafficPortRangeModel struct {
	PortStart types.Int64 `tfsdk:"port_start"`
	PortStop  types.Int64 `tfsdk:"port_stop"`
}

type trafficDomainModel struct {
	Domain     types.String `tfsdk:"domain"`
	Ports      types.Set    `tfsdk:"ports"`       // set of port numbers
	PortRanges types.List   `tfsdk:"port_ranges"` // ordered start/stop pairs
}

type trafficIPAddressModel struct {
	IPVersion  types.String `tfsdk:"ip_version"`   // v4 | v6
	IPOrSubnet types.String `tfsdk:"ip_or_subnet"` // address or CIDR
	Ports      types.Set    `tfsdk:"ports"`
	PortRanges types.List   `tfsdk:"port_ranges"`
}

type trafficIPRangeModel struct {
	IPVersion types.String `tfsdk:"ip_version"` // v4 | v6
	IPStart   types.String `tfsdk:"ip_start"`
	IPStop    types.String `tfsdk:"ip_stop"`
}

// --- attribute-type maps (for ObjectValueFrom / Set/List element types) ---

var trafficPortRangeAttrTypes = map[string]attr.Type{
	"port_start": types.Int64Type,
	"port_stop":  types.Int64Type,
}

var portRangeListType = types.ListType{ElemType: types.ObjectType{AttrTypes: trafficPortRangeAttrTypes}}
var portsSetType = types.SetType{ElemType: types.Int64Type}

var trafficDomainAttrTypes = map[string]attr.Type{
	"domain":      types.StringType,
	"ports":       portsSetType,
	"port_ranges": portRangeListType,
}

var trafficIPAddressAttrTypes = map[string]attr.Type{
	"ip_version":   types.StringType,
	"ip_or_subnet": types.StringType,
	"ports":        portsSetType,
	"port_ranges":  portRangeListType,
}

var trafficIPRangeAttrTypes = map[string]attr.Type{
	"ip_version": types.StringType,
	"ip_start":   types.StringType,
	"ip_stop":    types.StringType,
}

var (
	domainSetType    = types.SetType{ElemType: types.ObjectType{AttrTypes: trafficDomainAttrTypes}}
	ipAddressSetType = types.SetType{ElemType: types.ObjectType{AttrTypes: trafficIPAddressAttrTypes}}
	ipRangeSetType   = types.SetType{ElemType: types.ObjectType{AttrTypes: trafficIPRangeAttrTypes}}
)

// --- schema builders ---

func portRangesSchema() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Optional destination port ranges to narrow the match.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"port_start": schema.Int64Attribute{Required: true, MarkdownDescription: "First port in the range."},
				"port_stop":  schema.Int64Attribute{Required: true, MarkdownDescription: "Last port in the range."},
			},
		},
	}
}

func portsSchema() schema.SetAttribute {
	return schema.SetAttribute{
		Optional:            true,
		ElementType:         types.Int64Type,
		MarkdownDescription: "Optional destination ports to narrow the match.",
	}
}

func ipVersionAttr() schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "IP version: `v4` or `v6`.",
		Validators:          []validator.String{stringvalidator.OneOf("v4", "v6")},
	}
}

func domainsSchema(desc string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Optional:            true,
		MarkdownDescription: desc,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"domain":      schema.StringAttribute{Required: true, MarkdownDescription: "Domain to match (e.g. `example.com`)."},
				"ports":       portsSchema(),
				"port_ranges": portRangesSchema(),
			},
		},
	}
}

func ipAddressesSchema(desc string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Optional:            true,
		MarkdownDescription: desc,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"ip_version":   ipVersionAttr(),
				"ip_or_subnet": schema.StringAttribute{Required: true, MarkdownDescription: "IP address or CIDR to match."},
				"ports":        portsSchema(),
				"port_ranges":  portRangesSchema(),
			},
		},
	}
}

func ipRangesSchema(desc string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Optional:            true,
		MarkdownDescription: desc,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"ip_version": ipVersionAttr(),
				"ip_start":   schema.StringAttribute{Required: true, MarkdownDescription: "First address of the range."},
				"ip_stop":    schema.StringAttribute{Required: true, MarkdownDescription: "Last address of the range."},
			},
		},
	}
}

// stringSetToSlicePtr / slicePtrToStringSet bridge a framework string Set and a
// wire `*[]string` field (nil pointer when the set is unconfigured).
func stringSetToSlicePtr(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := stringSetToSlice(ctx, s, diags)
	return &v
}

// slicePtrToStringSet maps a wire string slice back to a framework Set. An
// omitted match field round-trips as null: the controller echoes an empty array
// ([]) rather than omitting it, so an empty slice must read back as null (not an
// empty set) to match the planned null — otherwise create reports "inconsistent
// result after apply". The same empty==null rule applies to every reader below.
func slicePtrToStringSet(p *[]string) types.Set {
	if p == nil || len(*p) == 0 {
		return types.SetNull(types.StringType)
	}
	return sliceToStringSet(*p)
}

// --- port / port-range converters ---

func setToPorts(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]int {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	ports := intSetToSlice(ctx, s, diags)
	return &ports
}

func portsToSet(p *[]int) types.Set {
	if p == nil || len(*p) == 0 {
		return types.SetNull(types.Int64Type)
	}
	return sliceToIntSet(*p)
}

func listToPortRanges(ctx context.Context, l types.List, diags *diag.Diagnostics) *[]wire.TrafficroutesDomainPortRange {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var objs []trafficPortRangeModel
	diags.Append(l.ElementsAs(ctx, &objs, false)...)
	out := make([]wire.TrafficroutesDomainPortRange, 0, len(objs))
	for _, o := range objs {
		out = append(out, wire.TrafficroutesDomainPortRange{
			PortStart: intPtr(o.PortStart),
			PortStop:  intPtr(o.PortStop),
		})
	}
	return &out
}

func portRangesToList(ctx context.Context, pr *[]wire.TrafficroutesDomainPortRange, diags *diag.Diagnostics) types.List {
	if pr == nil || len(*pr) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: trafficPortRangeAttrTypes})
	}
	objs := make([]trafficPortRangeModel, 0, len(*pr))
	for _, r := range *pr {
		objs = append(objs, trafficPortRangeModel{
			PortStart: intToState(r.PortStart),
			PortStop:  intToState(r.PortStop),
		})
	}
	l, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: trafficPortRangeAttrTypes}, objs)
	diags.Append(d...)
	return l
}

// --- domains / ip_addresses / ip_ranges converters ---

func setToDomains(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]wire.TrafficroutesDomain {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var objs []trafficDomainModel
	diags.Append(s.ElementsAs(ctx, &objs, false)...)
	out := make([]wire.TrafficroutesDomain, 0, len(objs))
	for _, o := range objs {
		out = append(out, wire.TrafficroutesDomain{
			Domain:     o.Domain.ValueString(),
			Ports:      setToPorts(ctx, o.Ports, diags),
			PortRanges: listToPortRanges(ctx, o.PortRanges, diags),
		})
	}
	return &out
}

func domainsToSet(ctx context.Context, d *[]wire.TrafficroutesDomain, diags *diag.Diagnostics) types.Set {
	if d == nil || len(*d) == 0 {
		return types.SetNull(domainSetType.ElemType)
	}
	objs := make([]trafficDomainModel, 0, len(*d))
	for _, dm := range *d {
		objs = append(objs, trafficDomainModel{
			Domain:     types.StringValue(dm.Domain),
			Ports:      portsToSet(dm.Ports),
			PortRanges: portRangesToList(ctx, dm.PortRanges, diags),
		})
	}
	s, di := types.SetValueFrom(ctx, domainSetType.ElemType, objs)
	diags.Append(di...)
	return s
}

func setToIPAddresses(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]wire.TrafficroutesIpAddress {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var objs []trafficIPAddressModel
	diags.Append(s.ElementsAs(ctx, &objs, false)...)
	out := make([]wire.TrafficroutesIpAddress, 0, len(objs))
	for _, o := range objs {
		out = append(out, wire.TrafficroutesIpAddress{
			IpVersion:  wire.TrafficroutesIpAddressIpVersion(o.IPVersion.ValueString()),
			IpOrSubnet: o.IPOrSubnet.ValueString(),
			Ports:      setToPorts(ctx, o.Ports, diags),
			PortRanges: listToPortRanges(ctx, o.PortRanges, diags),
		})
	}
	return &out
}

func ipAddressesToSet(ctx context.Context, a *[]wire.TrafficroutesIpAddress, diags *diag.Diagnostics) types.Set {
	if a == nil || len(*a) == 0 {
		return types.SetNull(ipAddressSetType.ElemType)
	}
	objs := make([]trafficIPAddressModel, 0, len(*a))
	for _, ip := range *a {
		objs = append(objs, trafficIPAddressModel{
			IPVersion:  types.StringValue(string(ip.IpVersion)),
			IPOrSubnet: types.StringValue(ip.IpOrSubnet),
			Ports:      portsToSet(ip.Ports),
			PortRanges: portRangesToList(ctx, ip.PortRanges, diags),
		})
	}
	s, di := types.SetValueFrom(ctx, ipAddressSetType.ElemType, objs)
	diags.Append(di...)
	return s
}

func setToIPRanges(ctx context.Context, s types.Set, diags *diag.Diagnostics) *[]wire.TrafficroutesIpRange {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var objs []trafficIPRangeModel
	diags.Append(s.ElementsAs(ctx, &objs, false)...)
	out := make([]wire.TrafficroutesIpRange, 0, len(objs))
	for _, o := range objs {
		out = append(out, wire.TrafficroutesIpRange{
			IpVersion: wire.TrafficroutesIpAddressIpVersion(o.IPVersion.ValueString()),
			IpStart:   o.IPStart.ValueString(),
			IpStop:    o.IPStop.ValueString(),
		})
	}
	return &out
}

func ipRangesToSet(ctx context.Context, r *[]wire.TrafficroutesIpRange, diags *diag.Diagnostics) types.Set {
	if r == nil || len(*r) == 0 {
		return types.SetNull(ipRangeSetType.ElemType)
	}
	objs := make([]trafficIPRangeModel, 0, len(*r))
	for _, rg := range *r {
		objs = append(objs, trafficIPRangeModel{
			IPVersion: types.StringValue(string(rg.IpVersion)),
			IPStart:   types.StringValue(rg.IpStart),
			IPStop:    types.StringValue(rg.IpStop),
		})
	}
	s, di := types.SetValueFrom(ctx, ipRangeSetType.ElemType, objs)
	diags.Append(di...)
	return s
}
