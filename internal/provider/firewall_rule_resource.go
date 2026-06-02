package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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

// firewallRulesets is the full set of classic firewall chains the controller
// accepts (IPv4 and IPv6 variants). Validators mirror the controller rather than
// over-constraining (invariant: no over-tight validators).
var firewallRulesets = []string{
	"WAN_IN", "WAN_OUT", "WAN_LOCAL",
	"LAN_IN", "LAN_OUT", "LAN_LOCAL",
	"GUEST_IN", "GUEST_OUT", "GUEST_LOCAL",
	"WANv6_IN", "WANv6_OUT", "WANv6_LOCAL",
	"LANv6_IN", "LANv6_OUT", "LANv6_LOCAL",
	"GUESTv6_IN", "GUESTv6_OUT", "GUESTv6_LOCAL",
}

var (
	_ resource.Resource                = (*firewallRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*firewallRuleResource)(nil)
	_ resource.ResourceWithImportState = (*firewallRuleResource)(nil)
)

func NewFirewallRuleResource() resource.Resource { return &firewallRuleResource{} }

type firewallRuleResource struct {
	net *network.Client
}

// firewallRuleResourceModel covers the classic firewallrule surface. Match
// fields are Optional+Computed: unset leaves the controller's value and reads
// back what it computes, so plans stay clean.
type firewallRuleResourceModel struct {
	ID        types.String `tfsdk:"id"`         // controller _id; import key (computed)
	Name      types.String `tfsdk:"name"`       // rule name (required)
	Enabled   types.Bool   `tfsdk:"enabled"`    // whether the rule is active
	Ruleset   types.String `tfsdk:"ruleset"`    // chain the rule lives in (required)
	RuleIndex types.Int64  `tfsdk:"rule_index"` // evaluation order within the ruleset (required)
	Action    types.String `tfsdk:"action"`     // accept | drop | reject (required)

	Protocol         types.String `tfsdk:"protocol"`                // matched protocol
	ProtocolV6       types.String `tfsdk:"protocol_v6"`             // matched IPv6 protocol (v6 rulesets)
	ProtocolExcepted types.Bool   `tfsdk:"protocol_match_excepted"` // match every protocol except the one above
	ICMPTypename     types.String `tfsdk:"icmp_typename"`           // ICMP type (when protocol = icmp)
	Logging          types.Bool   `tfsdk:"logging"`                 // log matching packets
	IPSec            types.String `tfsdk:"ipsec"`                   // match-ipsec | match-none

	SettingPreference types.String `tfsdk:"setting_preference"` // controller auto/manual toggle (read-only)

	// Connection-state matching.
	StateNew         types.Bool `tfsdk:"state_new"`
	StateEstablished types.Bool `tfsdk:"state_established"`
	StateRelated     types.Bool `tfsdk:"state_related"`
	StateInvalid     types.Bool `tfsdk:"state_invalid"`

	// Source match.
	SrcAddress          types.String `tfsdk:"src_address"`
	SrcPort             types.String `tfsdk:"src_port"`
	SrcNetworkID        types.String `tfsdk:"src_network_id"`
	SrcNetworkType      types.String `tfsdk:"src_network_type"`
	SrcFirewallGroupIDs types.Set    `tfsdk:"src_firewall_group_ids"`

	// Destination match.
	DstAddress          types.String `tfsdk:"dst_address"`
	DstPort             types.String `tfsdk:"dst_port"`
	DstNetworkID        types.String `tfsdk:"dst_network_id"`
	DstNetworkType      types.String `tfsdk:"dst_network_type"`
	DstFirewallGroupIDs types.Set    `tfsdk:"dst_firewall_group_ids"`
}

func (r *firewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_firewall_rule"
}

func (r *firewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	optStrSet := func(desc string) schema.SetAttribute {
		return schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UniFi classic firewall rule (`firewallrule`, legacy `/rest`). Matches on protocol, " +
			"addresses/ports, networks, firewall groups, and connection state within a ruleset (chain). The newer " +
			"zone-based firewall is a separate surface (a later phase).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller object ID (`_id`). Import key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Rule name."},
			"enabled": optBool("Whether the rule is active."),
			"ruleset": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The chain this rule belongs to, e.g. `LAN_IN`, `WAN_OUT`, `GUEST_LOCAL` (and `*v6_*` variants).",
				Validators:          []validator.String{stringvalidator.OneOf(firewallRulesets...)},
			},
			"rule_index": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Evaluation order within the ruleset (lower runs first). Must be unique within the " +
					"ruleset — the provider rejects a duplicate at apply time.",
				Validators: []validator.Int64{int64validator.AtLeast(0)},
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What to do with matching traffic: `accept`, `drop`, or `reject`.",
				Validators:          []validator.String{stringvalidator.OneOf("accept", "drop", "reject")},
			},

			"protocol":                optStr("Protocol to match: `all`, `tcp`, `udp`, `tcp_udp`, `icmp`, or a protocol number."),
			"protocol_v6":             optStr("IPv6 protocol to match (for `*v6_*` rulesets)."),
			"protocol_match_excepted": optBool("Match every protocol *except* the one in `protocol`."),
			"icmp_typename":           optStr("ICMP type name to match (when `protocol = icmp`)."),
			"logging":                 optBool("Log packets that match this rule."),
			"ipsec": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "IPsec matching: `match-ipsec` (only IPsec traffic) or `match-none` (only non-IPsec).",
				Validators:          []validator.String{stringvalidator.OneOf("match-ipsec", "match-none")},
			},

			"setting_preference": schema.StringAttribute{Computed: true, MarkdownDescription: "Controller auto/manual toggle (read-only)."},

			"state_new":         optBool("Match new connections."),
			"state_established": optBool("Match established connections."),
			"state_related":     optBool("Match related connections."),
			"state_invalid":     optBool("Match invalid connections."),

			"src_address":            optStr("Source IP or CIDR to match."),
			"src_port":               optStr("Source port or range to match."),
			"src_network_id":         optStr("Source network (`unifi_network_network` id) to match."),
			"src_network_type":       optStr("Source network match type (e.g. `NETv4`, `ADDRv4`)."),
			"src_firewall_group_ids": optStrSet("Source firewall groups (`unifi_network_firewall_group` ids) to match."),

			"dst_address":            optStr("Destination IP or CIDR to match."),
			"dst_port":               optStr("Destination port or range to match."),
			"dst_network_id":         optStr("Destination network (`unifi_network_network` id) to match."),
			"dst_network_type":       optStr("Destination network match type (e.g. `NETv4`, `ADDRv4`)."),
			"dst_firewall_group_ids": optStrSet("Destination firewall groups (`unifi_network_firewall_group` ids) to match."),
		},
	}
}

func (r *firewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok {
		r.net = c.Network()
	}
}

func (r *firewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.assertRuleIndexUnique(ctx, plan.Ruleset.ValueString(), plan.RuleIndex.ValueInt64(), "", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.net.Firewall().Rules().Create(ctx, plan.toFirewallRule(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall rule", err.Error())
		return
	}
	plan.fromFirewallRule(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.net.Firewall().Rules().Get(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall rule", err.Error())
		return
	}
	state.fromFirewallRule(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *firewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.assertRuleIndexUnique(ctx, plan.Ruleset.ValueString(), plan.RuleIndex.ValueInt64(), plan.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.net.Firewall().Rules().Update(ctx, plan.ID.ValueString(), plan.toFirewallRule(ctx, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall rule", err.Error())
		return
	}
	plan.fromFirewallRule(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.net.Firewall().Rules().Delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall rule", err.Error())
	}
}

func (r *firewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// assertRuleIndexUnique rejects a rule_index already used by another rule in the
// same ruleset (the controller silently accepts duplicates, which makes ordering
// ambiguous). selfID is excluded so an in-place update of the same rule passes.
func (r *firewallRuleResource) assertRuleIndexUnique(ctx context.Context, ruleset string, ruleIndex int64, selfID string, diags *diag.Diagnostics) {
	rules, err := r.net.Firewall().Rules().List(ctx)
	if err != nil {
		diags.AddError("Failed to validate rule_index uniqueness", err.Error())
		return
	}
	if name, conflict := ruleIndexConflict(rules, ruleset, ruleIndex, selfID); conflict {
		diags.AddAttributeError(path.Root("rule_index"),
			"Duplicate firewall rule_index",
			fmt.Sprintf("Rule %q already uses rule_index %d in ruleset %q. Each rule needs a unique index within its ruleset.",
				name, ruleIndex, ruleset))
	}
}

// ruleIndexConflict reports the name of an existing rule (other than selfID)
// that already occupies ruleIndex in ruleset, if any.
func ruleIndexConflict(rules []wire.Firewallrule, ruleset string, ruleIndex int64, selfID string) (string, bool) {
	for i := range rules {
		if deref(rules[i].UnderscoreId) == selfID || string(derefEnum(rules[i].Ruleset)) != ruleset || rules[i].RuleIndex == nil {
			continue
		}
		if int64(*rules[i].RuleIndex) == ruleIndex {
			return deref(rules[i].Name), true
		}
	}
	return "", false
}

func (m *firewallRuleResourceModel) toFirewallRule(ctx context.Context, diags *diag.Diagnostics) *wire.Firewallrule {
	rule := &wire.Firewallrule{
		UnderscoreId:          strPtrOrNil(m.ID),
		Name:                  strPtrOrNil(m.Name),
		Enabled:               boolPtr(m.Enabled),
		Ruleset:               (*wire.FirewallruleRuleset)(strPtrOrNil(m.Ruleset)),
		RuleIndex:             intPtr(m.RuleIndex),
		Action:                (*wire.FirewallruleAction)(strPtrOrNil(m.Action)),
		Protocol:              strPtrOrNil(m.Protocol),
		ProtocolV6:            strPtrOrNil(m.ProtocolV6),
		ProtocolMatchExcepted: boolPtr(m.ProtocolExcepted),
		IcmpTypename:          (*wire.FirewallruleIcmpTypename)(strPtrOrNil(m.ICMPTypename)),
		Logging:               boolPtr(m.Logging),
		Ipsec:                 (*wire.FirewallruleIpsec)(strPtrOrNil(m.IPSec)),
		StateNew:              boolPtr(m.StateNew),
		StateEstablished:      boolPtr(m.StateEstablished),
		StateRelated:          boolPtr(m.StateRelated),
		StateInvalid:          boolPtr(m.StateInvalid),
		SrcAddress:            strPtrOrNil(m.SrcAddress),
		SrcPort:               strPtrOrNil(m.SrcPort),
		SrcNetworkconfId:      strPtrOrNil(m.SrcNetworkID),
		SrcNetworkconfType:    (*wire.FirewallruleSrcNetworkconfType)(strPtrOrNil(m.SrcNetworkType)),
		DstAddress:            strPtrOrNil(m.DstAddress),
		DstPort:               strPtrOrNil(m.DstPort),
		DstNetworkconfId:      strPtrOrNil(m.DstNetworkID),
		DstNetworkconfType:    (*wire.FirewallruleDstNetworkconfType)(strPtrOrNil(m.DstNetworkType)),
	}
	// Leave the group sets nil when unset so GET-merge-PUT preserves the
	// controller's value (the fields are omitempty — a nil pointer is dropped).
	if g := stringSetToSlice(ctx, m.SrcFirewallGroupIDs, diags); g != nil {
		rule.SrcFirewallgroupIds = &g
	}
	if g := stringSetToSlice(ctx, m.DstFirewallGroupIDs, diags); g != nil {
		rule.DstFirewallgroupIds = &g
	}
	return rule
}

func (m *firewallRuleResourceModel) fromFirewallRule(r *wire.Firewallrule) {
	m.ID = types.StringValue(deref(r.UnderscoreId))
	m.Name = types.StringValue(deref(r.Name))
	m.Enabled = boolToState(r.Enabled)
	m.Ruleset = types.StringValue(string(derefEnum(r.Ruleset)))
	m.RuleIndex = intToState(r.RuleIndex)
	m.Action = types.StringValue(string(derefEnum(r.Action)))
	m.Protocol = strToStateOptional(deref(r.Protocol))
	m.ProtocolV6 = strToStateOptional(deref(r.ProtocolV6))
	m.ProtocolExcepted = boolToState(r.ProtocolMatchExcepted)
	m.ICMPTypename = strToStateOptional(string(derefEnum(r.IcmpTypename)))
	m.Logging = boolToState(r.Logging)
	m.IPSec = strToStateOptional(string(derefEnum(r.Ipsec)))
	m.SettingPreference = types.StringValue(string(derefEnum(r.SettingPreference)))
	m.StateNew = boolToState(r.StateNew)
	m.StateEstablished = boolToState(r.StateEstablished)
	m.StateRelated = boolToState(r.StateRelated)
	m.StateInvalid = boolToState(r.StateInvalid)
	m.SrcAddress = strToStateOptional(deref(r.SrcAddress))
	m.SrcPort = strToStateOptional(deref(r.SrcPort))
	m.SrcNetworkID = strToStateOptional(deref(r.SrcNetworkconfId))
	m.SrcNetworkType = strToStateOptional(string(derefEnum(r.SrcNetworkconfType)))
	m.SrcFirewallGroupIDs = sliceToStringSet(derefSlice(r.SrcFirewallgroupIds))
	m.DstAddress = strToStateOptional(deref(r.DstAddress))
	m.DstPort = strToStateOptional(deref(r.DstPort))
	m.DstNetworkID = strToStateOptional(deref(r.DstNetworkconfId))
	m.DstNetworkType = strToStateOptional(string(derefEnum(r.DstNetworkconfType)))
	m.DstFirewallGroupIDs = sliceToStringSet(derefSlice(r.DstFirewallgroupIds))
}
