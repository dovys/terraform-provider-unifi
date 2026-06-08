package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

// portOverrideAttrTypes is the object type for one entry of the device
// `port_overrides` map. Shared by the resource schema and the data source so the
// two stay in lock-step. The field set mirrors unifi_network_port_profile (the
// per-port config has the same shape) plus the per-port keys that only make
// sense on a device: port_profile_id (bind to a profile) and poe_mode.
var portOverrideAttrTypes = map[string]attr.Type{
	"port_profile_id":             types.StringType,
	"name":                        types.StringType,
	"poe_mode":                    types.StringType,
	"forward":                     types.StringType,
	"native_network_id":           types.StringType,
	"tagged_vlan_mgmt":            types.StringType,
	"excluded_network_ids":        types.SetType{ElemType: types.StringType},
	"voice_network_id":            types.StringType,
	"autoneg":                     types.BoolType,
	"speed":                       types.Int64Type,
	"full_duplex":                 types.BoolType,
	"isolation":                   types.BoolType,
	"port_security_enabled":       types.BoolType,
	"port_security_mac_addresses": types.SetType{ElemType: types.StringType},
	"dot1x_ctrl":                  types.StringType,
	"dot1x_idle_timeout":          types.Int64Type,
	"egress_rate_limit_enabled":   types.BoolType,
	"egress_rate_limit_kbps":      types.Int64Type,
	"stp_port_mode":               types.BoolType,
	"stp_bpdu_guard":              types.BoolType,
	"stp_edge_state":              types.StringType,
	"lldpmed_enabled":             types.BoolType,
	"port_keepalive_enabled":      types.BoolType,
	"multicast_router_mode":       types.StringType,
}

// portOverrideSchemaAttrs returns the nested schema attributes for one port
// override. computed controls whether the leaves are Optional+Computed (resource)
// or Computed-only (data source).
func portOverrideSchemaAttrs(computed bool) map[string]schema.Attribute {
	str := func(desc string) schema.Attribute {
		return schema.StringAttribute{Optional: !computed, Computed: true, MarkdownDescription: desc}
	}
	b := func(desc string) schema.Attribute {
		return schema.BoolAttribute{Optional: !computed, Computed: true, MarkdownDescription: desc}
	}
	i := func(desc string) schema.Attribute {
		return schema.Int64Attribute{Optional: !computed, Computed: true, MarkdownDescription: desc}
	}
	set := func(desc string) schema.Attribute {
		return schema.SetAttribute{Optional: !computed, Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	return map[string]schema.Attribute{
		"port_profile_id":             str("Bind the port to a `unifi_network_port_profile` (its `id`). The profile then governs the port's VLAN behavior, so leave the inline VLAN fields (`forward`, `native_network_id`, `tagged_vlan_mgmt`) unset — the controller nulls them once a profile is bound."),
		"name":                        str("Port label."),
		"poe_mode":                    str("Power over Ethernet: `auto`, `pasv24`, `passthrough`, or `off` (varies by switch). HIL-unverified."),
		"forward":                     str("VLAN behavior: `all`, `native`, `customize`, or `disabled`."),
		"native_network_id":           str("Untagged/native network (`unifi_network_network` id)."),
		"tagged_vlan_mgmt":            str("Tagged-VLAN handling: `auto`, `block_all`, or `custom`."),
		"excluded_network_ids":        set("Tagged networks to exclude when `tagged_vlan_mgmt = custom`."),
		"voice_network_id":            str("Voice VLAN network (`unifi_network_network` id)."),
		"autoneg":                     b("Auto-negotiate link speed/duplex."),
		"speed":                       i("Forced link speed in Mbps (when `autoneg = false`)."),
		"full_duplex":                 b("Forced full duplex (when `autoneg = false`)."),
		"isolation":                   b("Isolate this port from other isolated ports."),
		"port_security_enabled":       b("Restrict the port to specific MAC addresses."),
		"port_security_mac_addresses": set("Allowed MAC addresses when `port_security_enabled = true`."),
		"dot1x_ctrl":                  str("802.1X control mode."),
		"dot1x_idle_timeout":          i("802.1X idle timeout (seconds)."),
		"egress_rate_limit_enabled":   b("Enable an egress rate limit."),
		"egress_rate_limit_kbps":      i("Egress rate limit in kbps (when enabled)."),
		"stp_port_mode":               b("Participate in spanning tree."),
		"stp_bpdu_guard":              b("Enable BPDU guard."),
		"stp_edge_state":              str("STP edge-port state."),
		"lldpmed_enabled":             b("Enable LLDP-MED."),
		"port_keepalive_enabled":      b("Enable port keep-alive."),
		"multicast_router_mode":       str("Multicast router mode."),
	}
}

// portOverrideToNet converts one decoded port-override object (with its port
// number) into the client struct. diags collects set-conversion errors.
func portOverrideToNet(ctx context.Context, portIdx int, o portOverrideObject, diags *diag.Diagnostics) network.PortOverride {
	po := network.PortOverride{
		PortIdx:                portIdx,
		PortconfID:             strOrEmpty(o.PortProfileID),
		Name:                   strOrEmpty(o.Name),
		PoEMode:                strOrEmpty(o.POEMode),
		Forward:                strOrEmpty(o.Forward),
		NativeNetworkConfID:    strOrEmpty(o.NativeNetworkID),
		TaggedVLANMgmt:         strOrEmpty(o.TaggedVLANMgmt),
		VoiceNetworkConfID:     strOrEmpty(o.VoiceNetworkID),
		Autoneg:                boolPtr(o.Autoneg),
		Speed:                  intPtr(o.Speed),
		FullDuplex:             boolPtr(o.FullDuplex),
		Isolation:              boolPtr(o.Isolation),
		PortSecurityEnabled:    boolPtr(o.PortSecurityEnabled),
		Dot1XCtrl:              strOrEmpty(o.Dot1XCtrl),
		Dot1XIdleTimeout:       intPtr(o.Dot1XIdleTimeout),
		EgressRateLimitEnabled: boolPtr(o.EgressRateLimitEnabled),
		EgressRateLimitKbps:    intPtr(o.EgressRateLimitKbps),
		STPPortMode:            boolPtr(o.STPPortMode),
		STPBPDUGuard:           boolPtr(o.STPBPDUGuard),
		STPEdgeState:           strOrEmpty(o.STPEdgeState),
		LLDPMEDEnabled:         boolPtr(o.LLDPMEDEnabled),
		PortKeepaliveOn:        boolPtr(o.PortKeepaliveEnabled),
		MulticastRouterMode:    strOrEmpty(o.MulticastRouterMode),
	}
	// Unlike unifi_network_port_profile (which forces []string{} to clear), these
	// stay nil when unset: a per-port override is applied by merge (the network
	// client's mergePortOverrides overlays omitempty fields onto the existing
	// entry), so a nil slice correctly preserves the controller's value rather
	// than clearing it.
	po.ExcludedNetworkConfIDs = stringSetToSlice(ctx, o.ExcludedNetworkIDs, diags)
	po.PortSecurityMACAddress = stringSetToSlice(ctx, o.PortSecurityMACAddresses, diags)
	return po
}

// portOverrideObject is the Go view of one port-override object, used with
// ElementsAs/ObjectValueFrom to (de)serialize the framework Map element.
type portOverrideObject struct {
	PortProfileID            types.String `tfsdk:"port_profile_id"`
	Name                     types.String `tfsdk:"name"`
	POEMode                  types.String `tfsdk:"poe_mode"`
	Forward                  types.String `tfsdk:"forward"`
	NativeNetworkID          types.String `tfsdk:"native_network_id"`
	TaggedVLANMgmt           types.String `tfsdk:"tagged_vlan_mgmt"`
	ExcludedNetworkIDs       types.Set    `tfsdk:"excluded_network_ids"`
	VoiceNetworkID           types.String `tfsdk:"voice_network_id"`
	Autoneg                  types.Bool   `tfsdk:"autoneg"`
	Speed                    types.Int64  `tfsdk:"speed"`
	FullDuplex               types.Bool   `tfsdk:"full_duplex"`
	Isolation                types.Bool   `tfsdk:"isolation"`
	PortSecurityEnabled      types.Bool   `tfsdk:"port_security_enabled"`
	PortSecurityMACAddresses types.Set    `tfsdk:"port_security_mac_addresses"`
	Dot1XCtrl                types.String `tfsdk:"dot1x_ctrl"`
	Dot1XIdleTimeout         types.Int64  `tfsdk:"dot1x_idle_timeout"`
	EgressRateLimitEnabled   types.Bool   `tfsdk:"egress_rate_limit_enabled"`
	EgressRateLimitKbps      types.Int64  `tfsdk:"egress_rate_limit_kbps"`
	STPPortMode              types.Bool   `tfsdk:"stp_port_mode"`
	STPBPDUGuard             types.Bool   `tfsdk:"stp_bpdu_guard"`
	STPEdgeState             types.String `tfsdk:"stp_edge_state"`
	LLDPMEDEnabled           types.Bool   `tfsdk:"lldpmed_enabled"`
	PortKeepaliveEnabled     types.Bool   `tfsdk:"port_keepalive_enabled"`
	MulticastRouterMode      types.String `tfsdk:"multicast_router_mode"`
}

// portOverrideFromNet builds the framework object for one controller override.
func portOverrideFromNet(po network.PortOverride) portOverrideObject {
	return portOverrideObject{
		PortProfileID:            strToStateOptional(po.PortconfID),
		Name:                     strToStateOptional(po.Name),
		POEMode:                  strToStateOptional(po.PoEMode),
		Forward:                  strToStateOptional(po.Forward),
		NativeNetworkID:          strToStateOptional(po.NativeNetworkConfID),
		TaggedVLANMgmt:           strToStateOptional(po.TaggedVLANMgmt),
		ExcludedNetworkIDs:       sliceToStringSet(po.ExcludedNetworkConfIDs),
		VoiceNetworkID:           strToStateOptional(po.VoiceNetworkConfID),
		Autoneg:                  boolToState(po.Autoneg),
		Speed:                    intToState(po.Speed),
		FullDuplex:               boolToState(po.FullDuplex),
		Isolation:                boolToState(po.Isolation),
		PortSecurityEnabled:      boolToState(po.PortSecurityEnabled),
		PortSecurityMACAddresses: sliceToStringSet(po.PortSecurityMACAddress),
		Dot1XCtrl:                strToStateOptional(po.Dot1XCtrl),
		Dot1XIdleTimeout:         intToState(po.Dot1XIdleTimeout),
		EgressRateLimitEnabled:   boolToState(po.EgressRateLimitEnabled),
		EgressRateLimitKbps:      intToState(po.EgressRateLimitKbps),
		STPPortMode:              boolToState(po.STPPortMode),
		STPBPDUGuard:             boolToState(po.STPBPDUGuard),
		STPEdgeState:             strToStateOptional(po.STPEdgeState),
		LLDPMEDEnabled:           boolToState(po.LLDPMEDEnabled),
		PortKeepaliveEnabled:     boolToState(po.PortKeepaliveOn),
		MulticastRouterMode:      strToStateOptional(po.MulticastRouterMode),
	}
}
