package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

var (
	_ resource.Resource                   = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithConfigure      = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithImportState    = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithValidateConfig = (*firewallPolicyResource)(nil)
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
		"matching_target_type": ocString("`SPECIFIC` (inline `ips`/`network_ids`) or `OBJECT` (`ip_group_id`). "+
			"When unset, inferred from the operands whenever they change.",
			stringvalidator.OneOf("SPECIFIC", "OBJECT")),
		"ips":           optStringSet("Addresses, CIDRs or ranges, for `matching_target = \"IP\"`."),
		"network_ids":   optStringSet("Network `_id`s, for `matching_target = \"NETWORK\"`."),
		"ip_group_id":   optString("Firewall group `_id`, for `matching_target = \"IP\"` with `OBJECT`."),
		"port":          optString("Port, list or range, e.g. `443` or `22,53,80`."),
		"port_group_id": optString("Port group `_id`, for `port_matching_type = \"OBJECT\"`."),
		"port_matching_type": ocString("`ANY`, `SPECIFIC` (`port`) or `OBJECT` (`port_group_id`). "+
			"When unset, inferred from `port`/`port_group_id` whenever they change.",
			stringvalidator.OneOf("ANY", "SPECIFIC", "OBJECT")),
		"match_opposite_ips":      falseBool("Invert the address match."),
		"match_opposite_networks": falseBool("Invert the network match."),
		"match_opposite_ports":    falseBool("Invert the port match."),
	}
	if source {
		attrs["match_mac"] = falseBool("Match the client by MAC address.")
	}
	return attrs
}

func (r *firewallPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A zone-based firewall policy. Needs a site on the zone-based firewall.\n\n" +
			"Ordering isn't managed: the controller places a new policy, `index` shows where, and updates never move it. " +
			"Updates also leave the schedule and settings this resource doesn't model as they are; a new policy runs `ALWAYS`. " +
			"An endpoint's match operands (`ips`, `network_ids`, `ip_group_id`, `port`, `port_group_id`) are exactly what the " +
			"config says, so removing one clears it. Other optional settings keep their current value when removed from the config.\n\n" +
			"Predefined (system) policies can't be managed. Import by `_id`, or by `name=<policy name>` when the name is unique.\n\n" +
			"**Writes are HIL-unverified — see the provider TODO.**",
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
			"create_allow_respond": ocBool("Also create the return-traffic allow for an `ALLOW` policy. " +
				"The controller rejects `true` on a non-`ALLOW` action, a `gateway` or `external` destination zone, " +
				"`connection_state_type = \"RESPOND_ONLY\"`, or `connection_states` of exactly `ESTABLISHED` and `RELATED` (whatever the state type). " +
				"Unset, it's sent as `false` in those cases on create and update; otherwise create keeps the controller's default, `true`, and update the current value. " +
				"Set to `true`, plan fails in them."),
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
	if err := r.defaultAllowRespond(ctx, body); err != nil {
		resp.Diagnostics.AddError("Failed to read firewall zones", err.Error())
		return
	}
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

// defaultAllowRespond sends create_allow_respond = false for an unset create
// that conflicts: the controller reads the omitted key as true and 400s it
// ("create respond traffic not allowed", #8). Otherwise the key stays omitted
// so the controller's default applies.
func (r *firewallPolicyResource) defaultAllowRespond(ctx context.Context, p *network.FirewallPolicy) error {
	if p.CreateAllowRespond != nil {
		return nil
	}
	var states []string
	if p.ConnectionStates != nil {
		for _, s := range *p.ConnectionStates {
			states = append(states, string(s))
		}
	}
	reason := respondConflict(string(p.Action), string(derefEnum(p.ConnectionStateType)), states)
	if reason == "" {
		key, err := r.zoneKey(ctx, p.Destination.ZoneID)
		if err != nil {
			return err
		}
		reason = respondZoneConflict(key)
	}
	if reason != "" {
		f := false
		p.CreateAllowRespond = &f
	}
	return nil
}

func (r *firewallPolicyResource) zoneKey(ctx context.Context, id string) (string, error) {
	zones, err := r.net.Firewall().Zones().List(ctx)
	if err != nil {
		return "", err
	}
	for _, z := range zones {
		if deref(z.UnderscoreId) == id {
			return deref(z.ZoneKey), nil
		}
	}
	return "", nil
}

// respondConflict reports why the controller rejects create_allow_respond =
// true on a create or update with these settings, or "" if it doesn't. Empty values
// never conflict, so callers pass "" for anything unknown.
func respondConflict(action, stateType string, states []string) string {
	switch {
	case action != "" && action != "ALLOW":
		return fmt.Sprintf("`action` is `%s`; only an `ALLOW` policy can create return traffic", action)
	case stateType == "RESPOND_ONLY":
		return "`connection_state_type` is `RESPOND_ONLY`"
	case len(states) == 2 && slices.Contains(states, "ESTABLISHED") && slices.Contains(states, "RELATED"):
		return "`connection_states` is exactly `ESTABLISHED` and `RELATED`"
	}
	return ""
}

// respondZoneConflict is respondConflict's destination-zone case. The
// controller checks a Gateway or External zone attribute the API doesn't
// expose; the built-in zones carrying it are keyed `gateway` and `external`.
func respondZoneConflict(zoneKey string) string {
	if zoneKey == "gateway" || zoneKey == "external" {
		return fmt.Sprintf("the destination is the `%s` zone", zoneKey)
	}
	return ""
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

// endpointCreateDefaults covers operands that were still unknown at plan time.
func endpointCreateDefaults(e *network.PolicyEndpoint) {
	if e.PortMatchingType == "" {
		e.PortMatchingType = inferPortMatchingType(e.Port, e.PortGroupID)
	}
	if e.MatchingTargetType == "" {
		e.MatchingTargetType = inferMatchingTargetType(e.MatchingTarget, e.IPGroupID)
	}
}

func inferPortMatchingType(port, portGroupID string) string {
	switch {
	case portGroupID != "":
		return "OBJECT"
	case port != "":
		return "SPECIFIC"
	default:
		return "ANY"
	}
}

// inferMatchingTargetType returns "" for ANY, which takes no type.
func inferMatchingTargetType(target, ipGroupID string) string {
	switch {
	case target == "ANY":
		return ""
	case ipGroupID != "":
		return "OBJECT"
	default:
		return "SPECIFIC"
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
	m.Source.MatchMAC = falseIfAbsent(p.Source.MatchMAC)
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
	e.MatchOppositeIPs = falseIfAbsent(w.MatchOppositeIPs)
	e.MatchOppositeNetworks = falseIfAbsent(w.MatchOppositeNetworks)
	e.MatchOppositePorts = falseIfAbsent(w.MatchOppositePorts)
}

// nonEmptyStringSet is sliceToStringSet with an empty slice read as null too;
// the sets that use it reject `[]` in config, so null is the only "unset".
func nonEmptyStringSet(items []string) types.Set {
	if len(items) == 0 {
		return types.SetNull(types.StringType)
	}
	return sliceToStringSet(items)
}

// falseIfAbsent reads an absent flag as its default, false, so it matches
// an omitted attribute (which defaults to false).
func falseIfAbsent(p *bool) types.Bool {
	return types.BoolValue(p != nil && *p)
}

// ModifyPlan gates creation on the zone-based firewall and on
// create_allow_respond (invariant 3), and infers each endpoint's match types
// from its operands when unconfigured.
func (r *firewallPolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	creating := req.State.Raw.IsNull()
	if creating && r.net != nil {
		requireZoneFirewall(ctx, r.net, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	var plan, cfg firewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	var prior firewallPolicyModel
	if !creating {
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Source != nil && cfg.Source != nil {
		var p *policyEndpointModel
		if prior.Source != nil {
			p = &prior.Source.policyEndpointModel
		}
		inferEndpointModes(&plan.Source.policyEndpointModel, &cfg.Source.policyEndpointModel, p)
	}
	if plan.Destination != nil && cfg.Destination != nil {
		inferEndpointModes(plan.Destination, cfg.Destination, prior.Destination)
	}
	if r.net != nil {
		r.planAllowRespond(ctx, creating, &cfg, &plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// planAllowRespond applies the respond-traffic rule the controller runs on
// create and update, reading an omitted create_allow_respond as true. A
// conflicting explicit true fails the plan. Unset, a conflicting update plans
// false, since the PUT would otherwise carry the state's or the live true; an
// unset create is left to Create, where the zone is known. A null plan on
// update means no change, so no PUT. Unknown inputs skip their check.
func (r *firewallPolicyResource) planAllowRespond(ctx context.Context, creating bool, cfg, plan *firewallPolicyModel, diags *diag.Diagnostics) {
	explicit := !cfg.CreateAllowRespond.IsNull()
	switch {
	case cfg.CreateAllowRespond.IsUnknown(), explicit && !cfg.CreateAllowRespond.ValueBool():
		return
	case !explicit && (creating || plan.CreateAllowRespond.IsNull() ||
		(!plan.CreateAllowRespond.IsUnknown() && !plan.CreateAllowRespond.ValueBool())):
		return
	}
	reason := r.respondReason(ctx, plan, diags)
	switch {
	case reason == "":
	case explicit:
		diags.AddAttributeError(path.Root("create_allow_respond"), "Return traffic not allowed",
			"The controller rejects `create_allow_respond = true` here: "+reason+". Set it to `false` or leave it unset.")
	default:
		plan.CreateAllowRespond = types.BoolValue(false)
	}
}

// respondReason is respondConflict over a planned policy, zone included.
func (r *firewallPolicyResource) respondReason(ctx context.Context, m *firewallPolicyModel, diags *diag.Diagnostics) string {
	var states []string
	if !m.ConnectionStates.IsUnknown() {
		states = stringSetToSlice(ctx, m.ConnectionStates, diags)
	}
	reason := respondConflict(m.Action.ValueString(), m.ConnectionStateType.ValueString(), states)
	if reason != "" || m.Destination == nil || m.Destination.ZoneID.IsUnknown() {
		return reason
	}
	key, err := r.zoneKey(ctx, m.Destination.ZoneID.ValueString())
	if err != nil {
		diags.AddWarning("Couldn't read firewall zones", err.Error())
		return ""
	}
	return respondZoneConflict(key)
}

// inferEndpointModes sets an unconfigured match type from the operands, but
// keeps the prior value while the operands are unchanged: an imported policy
// must plan clean even where the live type isn't what inference would pick.
func inferEndpointModes(plan, cfg, prior *policyEndpointModel) {
	if cfg.PortMatchingType.IsNull() {
		switch {
		case prior != nil && plan.Port.Equal(prior.Port) && plan.PortGroupID.Equal(prior.PortGroupID):
			plan.PortMatchingType = prior.PortMatchingType
		case plan.Port.IsUnknown() || plan.PortGroupID.IsUnknown():
			plan.PortMatchingType = types.StringUnknown()
		default:
			plan.PortMatchingType = types.StringValue(inferPortMatchingType(plan.Port.ValueString(), plan.PortGroupID.ValueString()))
		}
	}
	if cfg.MatchingTargetType.IsNull() {
		unchanged := prior != nil && plan.MatchingTarget.Equal(prior.MatchingTarget) && plan.IPGroupID.Equal(prior.IPGroupID) &&
			sameStringSet(plan.IPs, prior.IPs) && sameStringSet(plan.NetworkIDs, prior.NetworkIDs)
		switch {
		case unchanged:
			plan.MatchingTargetType = prior.MatchingTargetType
		case plan.MatchingTarget.IsUnknown() || plan.IPGroupID.IsUnknown():
			plan.MatchingTargetType = types.StringUnknown()
		default:
			if t := inferMatchingTargetType(plan.MatchingTarget.ValueString(), plan.IPGroupID.ValueString()); t != "" {
				plan.MatchingTargetType = types.StringValue(t)
			} else {
				plan.MatchingTargetType = types.StringUnknown() // ANY: the controller's value
			}
		}
	}
}

// ValidateConfig rejects operand combinations that contradict each other.
func (r *firewallPolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg firewallPolicyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Source != nil {
		validateEndpoint(path.Root("source"), &cfg.Source.policyEndpointModel, &resp.Diagnostics)
	}
	if cfg.Destination != nil {
		validateEndpoint(path.Root("destination"), cfg.Destination, &resp.Diagnostics)
	}
}

func validateEndpoint(at path.Path, e *policyEndpointModel, diags *diag.Diagnostics) {
	set := func(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }
	bad := func(attrName, msg string) {
		diags.AddAttributeError(at.AtName(attrName), "Contradictory firewall policy endpoint", msg)
	}

	if set(e.Port) && set(e.PortGroupID) {
		bad("port_group_id", "Set `port` or `port_group_id`, not both.")
	}
	if set(e.IPs) && set(e.IPGroupID) {
		bad("ip_group_id", "Set `ips` or `ip_group_id`, not both.")
	}
	if set(e.MatchingTarget) && e.MatchingTarget.ValueString() == "ANY" && (set(e.IPs) || set(e.NetworkIDs) || set(e.IPGroupID)) {
		bad("matching_target", "`matching_target = \"ANY\"` takes no `ips`, `network_ids` or `ip_group_id`.")
	}
	if set(e.PortMatchingType) {
		switch e.PortMatchingType.ValueString() {
		case "ANY":
			if set(e.Port) || set(e.PortGroupID) {
				bad("port_matching_type", "`port_matching_type = \"ANY\"` takes no `port` or `port_group_id`.")
			}
		case "SPECIFIC":
			if set(e.PortGroupID) || e.Port.IsNull() {
				bad("port_matching_type", "`port_matching_type = \"SPECIFIC\"` needs `port` and no `port_group_id`.")
			}
		case "OBJECT":
			if set(e.Port) || e.PortGroupID.IsNull() {
				bad("port_matching_type", "`port_matching_type = \"OBJECT\"` needs `port_group_id` and no `port`.")
			}
		}
	}
	if set(e.MatchingTargetType) {
		switch e.MatchingTargetType.ValueString() {
		case "OBJECT":
			if set(e.IPs) {
				bad("matching_target_type", "`matching_target_type = \"OBJECT\"` takes `ip_group_id`, not `ips`.")
			}
		case "SPECIFIC":
			if set(e.IPGroupID) {
				bad("matching_target_type", "`matching_target_type = \"SPECIFIC\"` takes `ips`, not `ip_group_id`.")
			}
		}
	}
}

// sameStringSet compares sets by membership; the framework's Set.Equal is
// order-sensitive, and plan and state can list the same set differently.
func sameStringSet(a, b types.Set) bool {
	if a.IsNull() || a.IsUnknown() || b.IsNull() || b.IsUnknown() {
		return a.Equal(b)
	}
	if len(a.Elements()) != len(b.Elements()) {
		return false
	}
	have := map[string]bool{}
	for _, v := range a.Elements() {
		have[v.String()] = true
	}
	for _, v := range b.Elements() {
		if !have[v.String()] {
			return false
		}
	}
	return true
}

// requireZoneFirewall stops a create on a classic site at plan time. A probe
// error only warns, so an unexpected controller reply can't block planning.
func requireZoneFirewall(ctx context.Context, net *network.Client, diags *diag.Diagnostics) {
	zbf, err := net.ZoneBasedFirewall(ctx)
	if err != nil {
		diags.AddWarning("Couldn't confirm the site's firewall mode", err.Error())
		return
	}
	if !zbf {
		diags.AddError("Site uses the classic firewall",
			"This site has no firewall zones, so it can't hold a zone-based firewall policy. "+
				"Migrate the site to the zone-based firewall, or use unifi_network_firewall_rule.")
	}
}
