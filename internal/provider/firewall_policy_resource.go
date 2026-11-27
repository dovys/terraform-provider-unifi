package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*firewallPolicyResource)(nil)
)

func NewFirewallPolicyResource() resource.Resource { return &firewallPolicyResource{} }

type firewallPolicyResource struct {
	net *network.Client
}

type firewallPolicyModel struct {
	ID                    types.String         `tfsdk:"id"`
	Name                  types.String         `tfsdk:"name"`
	Description           types.String         `tfsdk:"description"`
	Action                types.String         `tfsdk:"action"`
	Enabled               types.Bool           `tfsdk:"enabled"`
	Protocol              types.String         `tfsdk:"protocol"`
	IPVersion             types.String         `tfsdk:"ip_version"`
	ConnectionStateType   types.String         `tfsdk:"connection_state_type"`
	ConnectionStates      types.Set            `tfsdk:"connection_states"`
	CreateAllowRespond    types.Bool           `tfsdk:"create_allow_respond"`
	Logging               types.Bool           `tfsdk:"logging"`
	MatchIPSec            types.Bool           `tfsdk:"match_ip_sec"`
	MatchOppositeProtocol types.Bool           `tfsdk:"match_opposite_protocol"`
	Index                 types.Int64          `tfsdk:"index"`
	Predefined            types.Bool           `tfsdk:"predefined"`
	Source                *policySourceModel   `tfsdk:"source"` // nil in freshly imported state
	Destination           *policyEndpointModel `tfsdk:"destination"`
}

// policyEndpointModel is the destination; the source adds match_mac.
type policyEndpointModel struct {
	ZoneID                types.String `tfsdk:"zone_id"`
	MatchingTarget        types.String `tfsdk:"matching_target"`
	MatchingTargetType    types.String `tfsdk:"matching_target_type"`
	IPs                   types.Set    `tfsdk:"ips"`
	NetworkIDs            types.Set    `tfsdk:"network_ids"`
	IPGroupID             types.String `tfsdk:"ip_group_id"`
	Port                  types.String `tfsdk:"port"`
	PortGroupID           types.String `tfsdk:"port_group_id"`
	PortMatchingType      types.String `tfsdk:"port_matching_type"`
	MatchOppositeIPs      types.Bool   `tfsdk:"match_opposite_ips"`
	MatchOppositeNetworks types.Bool   `tfsdk:"match_opposite_networks"`
	MatchOppositePorts    types.Bool   `tfsdk:"match_opposite_ports"`
}

type policySourceModel struct {
	policyEndpointModel
	MatchMAC types.Bool `tfsdk:"match_mac"`
}

func (r *firewallPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_firewall_policy"
}

// UseStateForUnknown keeps an unconfigured knob at its read value, so an
// imported policy plans clean against a config that states only its intent.
func ocString(desc string, v ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true, Computed: true, MarkdownDescription: desc, Validators: v,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func ocBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional: true, Computed: true, MarkdownDescription: desc,
		PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
	}
}

func ocStringSet(desc string, v ...validator.String) schema.SetAttribute {
	return schema.SetAttribute{
		Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: desc,
		Validators:    []validator.Set{setvalidator.SizeAtLeast(1), setvalidator.ValueStringsAre(v...)},
		PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
	}
}

func policyEndpointAttrs(source bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"zone_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Firewall zone `_id`; look it up with `data.unifi_network_firewall_zone`.",
		},
		"matching_target": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "What to match inside the zone, e.g. `ANY`, `NETWORK`, `IP`, `CLIENT`, `REGION`.",
		},
		"matching_target_type": ocString("`SPECIFIC` (inline `ips`/`network_ids`) or `OBJECT` (`ip_group_id`). Inferred on create when unset.",
			stringvalidator.OneOf("SPECIFIC", "OBJECT")),
		"ips":           ocStringSet("Addresses, CIDRs or ranges, for `matching_target = \"IP\"` with `SPECIFIC`."),
		"network_ids":   ocStringSet("Network `_id`s, for `matching_target = \"NETWORK\"`."),
		"ip_group_id":   ocString("Firewall group `_id`, for `matching_target = \"IP\"` with `OBJECT`."),
		"port":          ocString("Port, list or range, e.g. `443` or `22,53,80`.", stringvalidator.LengthAtLeast(1)),
		"port_group_id": ocString("Port group `_id`, for `port_matching_type = \"OBJECT\"`."),
		"port_matching_type": ocString("`ANY`, `SPECIFIC` (`port`) or `OBJECT` (`port_group_id`). Inferred on create when unset.",
			stringvalidator.OneOf("ANY", "SPECIFIC", "OBJECT")),
		"match_opposite_ips":      ocBool("Invert the address match."),
		"match_opposite_networks": ocBool("Invert the network match."),
		"match_opposite_ports":    ocBool("Invert the port match."),
	}
	if source {
		attrs["match_mac"] = ocBool("Match the client by MAC address.")
	}
	return attrs
}

func (r *firewallPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A zone-based firewall policy (v2 `firewall-policies`). Needs a site on the zone-based firewall.\n\n" +
			"Ordering is not managed: the controller places a new policy, and `index` reflects its position. " +
			"Updates never change `index`; reorder in the UniFi UI. The schedule and every unmodeled field keep their live value " +
			"(updates GET the policy, overlay the configured fields, and PUT it back); a new policy is scheduled `ALWAYS`.\n\n" +
			"Predefined (system) policies can't be managed. An Optional+Computed attribute left out of the config keeps its " +
			"last-read value, so removing it doesn't reset it.\n\n" +
			"Import by `_id`, or by `name=<policy name>` when the name is unique.\n\n" +
			"**HIL-unverified:** writes have not been confirmed on a live controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":        schema.StringAttribute{Required: true, MarkdownDescription: "Policy name."},
			"description": ocString("Free-text description.", stringvalidator.LengthBetween(1, 500)),
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`ALLOW`, `BLOCK` or `REJECT`.",
				Validators:          []validator.String{stringvalidator.OneOf("ALLOW", "BLOCK", "REJECT")},
			},
			"enabled":  ocBool("Whether the policy is active. Defaults to `true` on create."),
			"protocol": ocString("`all`, `tcp`, `udp`, `tcp_udp`, `icmp` or `icmpv6`. Defaults to `all` on create."),
			"ip_version": ocString("`IPV4`, `IPV6` or `BOTH`. Defaults to `IPV4` on create.",
				stringvalidator.OneOf("IPV4", "IPV6", "BOTH")),
			"connection_state_type": ocString("`ALL`, `RESPOND_ONLY` or `CUSTOM` (with `connection_states`). Defaults to `ALL` on create.",
				stringvalidator.OneOf("ALL", "RESPOND_ONLY", "CUSTOM")),
			"connection_states": ocStringSet("States matched when `connection_state_type = \"CUSTOM\"`: `NEW`, `ESTABLISHED`, `RELATED`, `INVALID`.",
				stringvalidator.OneOf("NEW", "ESTABLISHED", "RELATED", "INVALID")),
			"create_allow_respond":    ocBool("Also create the return-traffic allow for an `ALLOW` policy."),
			"logging":                 ocBool("Log matched traffic."),
			"match_ip_sec":            ocBool("Match on IPsec."),
			"match_opposite_protocol": ocBool("Invert the protocol match."),
			"index": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Evaluation position the controller assigned. Read-only; never written.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"predefined": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this is a system policy (always `false` for a managed one).",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"source": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Traffic origin.",
				Attributes:          policyEndpointAttrs(true),
			},
			"destination": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Traffic target.",
				Attributes:          policyEndpointAttrs(false),
			},
		},
	}
}

func (r *firewallPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *firewallPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := plan.toPolicy(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	applyPolicyCreateDefaults(body)
	created, err := r.net.Firewall().Policies().Create(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall policy", err.Error())
		return
	}
	plan.fromPolicy(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.net.Firewall().Policies().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall policy", err.Error())
		return
	}
	if p.Predefined != nil && *p.Predefined {
		resp.Diagnostics.AddError("Predefined firewall policy",
			fmt.Sprintf("Policy %q (%s) is a predefined system policy, which unifi_network_firewall_policy can't manage. "+
				"Remove it from the configuration and state.", p.Name, state.ID.ValueString()))
		return
	}
	state.fromPolicy(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *firewallPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state firewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.Predefined.ValueBool() {
		resp.Diagnostics.AddError("Predefined firewall policy", "Predefined system policies can't be managed.")
		return
	}
	body := plan.toPolicy(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Firewall().Policies().Update(ctx, state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall policy", err.Error())
		return
	}
	plan.fromPolicy(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.Predefined.ValueBool() {
		resp.Diagnostics.AddError("Predefined firewall policy", "Predefined system policies can't be deleted; remove it from state instead.")
		return
	}
	if err := r.net.Firewall().Policies().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall policy", err.Error())
	}
}

func (r *firewallPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByNameOrID(ctx, req, resp, "firewall policy", func(ctx context.Context, name string) (string, error) {
		p, err := r.net.Firewall().Policies().GetByName(ctx, name)
		if err != nil {
			return "", err
		}
		return deref(p.UnderscoreId), nil
	})
}

// toPolicy leaves unknown and null values empty, which Update's merge treats
// as "keep the live value".
func (m *firewallPolicyModel) toPolicy(ctx context.Context, diags *diag.Diagnostics) *network.FirewallPolicy {
	p := &network.FirewallPolicy{}
	p.UnderscoreId = strPtrIfSet(m.ID)
	p.Name = m.Name.ValueString()
	p.Description = strPtrIfSet(m.Description)
	p.Action = wire.FirewallPoliciesAction(m.Action.ValueString())
	p.Enabled = boolPtr(m.Enabled)
	p.Protocol = strPtrIfSet(m.Protocol)
	p.IpVersion = wire.FirewallPoliciesIpVersion(strOrEmpty(m.IPVersion))
	p.ConnectionStateType = (*wire.FirewallPoliciesConnectionStateType)(strPtrIfSet(m.ConnectionStateType))
	if states := stringSetToSlice(ctx, m.ConnectionStates, diags); len(states) > 0 {
		cs := make([]wire.FirewallPoliciesConnectionStates, len(states))
		for i, s := range states {
			cs[i] = wire.FirewallPoliciesConnectionStates(s)
		}
		p.ConnectionStates = &cs
	}
	p.CreateAllowRespond = boolPtr(m.CreateAllowRespond)
	p.Logging = boolPtr(m.Logging)
	p.MatchIpSec = boolPtr(m.MatchIPSec)
	p.MatchOppositeProtocol = boolPtr(m.MatchOppositeProtocol)
	if m.Source != nil {
		p.Source = m.Source.toEndpoint(ctx, diags)
		p.Source.MatchMAC = boolPtr(m.Source.MatchMAC)
	}
	if m.Destination != nil {
		p.Destination = m.Destination.toEndpoint(ctx, diags)
	}
	return p
}

func (e *policyEndpointModel) toEndpoint(ctx context.Context, diags *diag.Diagnostics) network.PolicyEndpoint {
	return network.PolicyEndpoint{
		ZoneID:                strOrEmpty(e.ZoneID),
		MatchingTarget:        strOrEmpty(e.MatchingTarget),
		MatchingTargetType:    strOrEmpty(e.MatchingTargetType),
		IPs:                   stringSetToSlice(ctx, e.IPs, diags),
		NetworkIDs:            stringSetToSlice(ctx, e.NetworkIDs, diags),
		IPGroupID:             strOrEmpty(e.IPGroupID),
		Port:                  strOrEmpty(e.Port),
		PortGroupID:           strOrEmpty(e.PortGroupID),
		PortMatchingType:      strOrEmpty(e.PortMatchingType),
		MatchOppositeIPs:      boolPtr(e.MatchOppositeIPs),
		MatchOppositeNetworks: boolPtr(e.MatchOppositeNetworks),
		MatchOppositePorts:    boolPtr(e.MatchOppositePorts),
	}
}

// applyPolicyCreateDefaults fills what the v2 POST requires (it takes the full
// DTO) where the config left it unset.
func applyPolicyCreateDefaults(p *network.FirewallPolicy) {
	if p.Enabled == nil {
		t := true
		p.Enabled = &t
	}
	if p.IpVersion == "" {
		p.IpVersion = "IPV4"
	}
	if p.Protocol == nil {
		all := "all"
		p.Protocol = &all
	}
	if p.ConnectionStateType == nil {
		all := wire.FirewallPoliciesConnectionStateType("ALL")
		p.ConnectionStateType = &all
	}
	if len(p.Schedule) == 0 {
		p.Schedule = json.RawMessage(`{"mode":"ALWAYS"}`)
	}
	endpointCreateDefaults(&p.Source)
	endpointCreateDefaults(&p.Destination)
}

func endpointCreateDefaults(e *network.PolicyEndpoint) {
	if e.PortMatchingType == "" {
		switch {
		case e.PortGroupID != "":
			e.PortMatchingType = "OBJECT"
		case e.Port != "":
			e.PortMatchingType = "SPECIFIC"
		default:
			e.PortMatchingType = "ANY"
		}
	}
	if e.MatchingTargetType == "" && e.MatchingTarget != "ANY" {
		if e.IPGroupID != "" {
			e.MatchingTargetType = "OBJECT"
		} else {
			e.MatchingTargetType = "SPECIFIC"
		}
	}
}

// fromPolicy reads empty strings and arrays as null, so the controller's
// several spellings of "unset" can't diff against an omitted attribute.
func (m *firewallPolicyModel) fromPolicy(p *network.FirewallPolicy) {
	m.ID = types.StringValue(deref(p.UnderscoreId))
	m.Name = types.StringValue(p.Name)
	m.Description = strToStateOptional(deref(p.Description))
	m.Action = types.StringValue(string(p.Action))
	m.Enabled = boolToState(p.Enabled)
	m.Protocol = strToStateOptional(deref(p.Protocol))
	m.IPVersion = strToStateOptional(string(p.IpVersion))
	m.ConnectionStateType = strToStateOptional(string(derefEnum(p.ConnectionStateType)))
	var states []string
	if p.ConnectionStates != nil {
		for _, s := range *p.ConnectionStates {
			states = append(states, string(s))
		}
	}
	m.ConnectionStates = nonEmptyStringSet(states)
	m.CreateAllowRespond = boolToState(p.CreateAllowRespond)
	m.Logging = boolToState(p.Logging)
	m.MatchIPSec = boolToState(p.MatchIpSec)
	m.MatchOppositeProtocol = boolToState(p.MatchOppositeProtocol)
	m.Index = intToState(p.Index)
	m.Predefined = types.BoolValue(p.Predefined != nil && *p.Predefined)
	m.Source = &policySourceModel{}
	m.Source.fromEndpoint(p.Source)
	m.Source.MatchMAC = boolToState(p.Source.MatchMAC)
	m.Destination = &policyEndpointModel{}
	m.Destination.fromEndpoint(p.Destination)
}

func (e *policyEndpointModel) fromEndpoint(w network.PolicyEndpoint) {
	e.ZoneID = types.StringValue(w.ZoneID)
	e.MatchingTarget = types.StringValue(w.MatchingTarget)
	e.MatchingTargetType = strToStateOptional(w.MatchingTargetType)
	e.IPs = nonEmptyStringSet(w.IPs)
	e.NetworkIDs = nonEmptyStringSet(w.NetworkIDs)
	e.IPGroupID = strToStateOptional(w.IPGroupID)
	e.Port = strToStateOptional(w.Port)
	e.PortGroupID = strToStateOptional(w.PortGroupID)
	e.PortMatchingType = strToStateOptional(w.PortMatchingType)
	e.MatchOppositeIPs = boolToState(w.MatchOppositeIPs)
	e.MatchOppositeNetworks = boolToState(w.MatchOppositeNetworks)
	e.MatchOppositePorts = boolToState(w.MatchOppositePorts)
}

// nonEmptyStringSet is sliceToStringSet with an empty slice read as null too;
// the sets that use it reject `[]` in config, so null is the only "unset".
func nonEmptyStringSet(items []string) types.Set {
	if len(items) == 0 {
		return types.SetNull(types.StringType)
	}
	return sliceToStringSet(items)
}
