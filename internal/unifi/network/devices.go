package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// DevicesService is the devices group: UniFi hardware (APs, switches, gateways).
// Reads come from the authoritative legacy /stat/device list (telemetry + the
// full config document); config writes are a sparse PUT to /rest/device/<id>
// (owned fields only; the controller shallow-merges, and the port_overrides
// array is GET-merged since the controller replaces it wholesale); adopt/forget
// go through /cmd/devmgr.
// The adopt/forget verbs are HIL-unverified (no adoptable hardware on the recon
// network) — see TODO.md.
type DevicesService struct{ core *core }

// Devices returns the devices group.
func (c *Client) Devices() DevicesService { return DevicesService{core: c.core} }

// ForwardDisabled is the value the controller DERIVES into `forward` for an
// administratively disabled switch port. The controller has no direct disable
// field: forward is a function of tagged_vlan_mgmt + the native VLAN, so block_all
// with an empty native derives `disabled`, and auto with the default native derives
// the `all` trunk. See PortOverride.Disabled / applyDerivedDisable.
const (
	ForwardDisabled    = "disabled"
	taggedVLANBlockAll = "block_all"
	taggedVLANAuto     = "auto"
)

// PortOverride is one curated entry of a switch's `port_overrides` array, keyed
// by PortIdx. The controller's full per-port surface is much larger (see
// wire.Device.PortOverrides); this is the modeled subset the provider writes.
// It is applied by per-port merge (replace-semantics array) rather than a typed
// PUT, so it lives here as a facade helper. The json tags match the controller
// document, so it round-trips with wire.Device.PortOverrides (PortOverridesOf).
// Pointers + omitempty give three-state semantics inside the merge overlay.
type PortOverride struct {
	PortIdx    int    `json:"port_idx"`              // physical port number (the map key); always sent
	PortconfID string `json:"portconf_id,omitempty"` // bind to a port profile (portconf _id)
	Name       string `json:"name,omitempty"`
	PoEMode    string `json:"poe_mode,omitempty"` // auto | pasv24 | passthrough | off

	Forward                string   `json:"forward,omitempty"`                  // all | native | customize | disabled — controller-DERIVED on a device port (read-only); see RestoreConfiguredScalars
	NativeNetworkConfID    string   `json:"native_networkconf_id,omitempty"`    // untagged/native network (networkconf _id)
	TaggedVLANMgmt         string   `json:"tagged_vlan_mgmt,omitempty"`         // auto | block_all | custom
	ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids,omitempty"` // VLANs to block on the trunk when TaggedVLANMgmt=custom
	VoiceNetworkConfID     string   `json:"voice_networkconf_id,omitempty"`     // voice VLAN (networkconf _id)

	Autoneg    *bool `json:"autoneg,omitempty"`     // auto-negotiate speed/duplex
	Speed      *int  `json:"speed,omitempty"`       // forced link speed (Mbps) when autoneg off
	FullDuplex *bool `json:"full_duplex,omitempty"` // forced full duplex when autoneg off

	Isolation              *bool    `json:"isolation,omitempty"`
	PortSecurityEnabled    *bool    `json:"port_security_enabled,omitempty"`
	PortSecurityMACAddress []string `json:"port_security_mac_address,omitempty"`
	Dot1XCtrl              string   `json:"dot1x_ctrl,omitempty"` // auto | force_authorized | force_unauthorized | mac_based | multi_host
	Dot1XIdleTimeout       *int     `json:"dot1x_idle_timeout,omitempty"`
	EgressRateLimitEnabled *bool    `json:"egress_rate_limit_kbps_enabled,omitempty"`
	EgressRateLimitKbps    *int     `json:"egress_rate_limit_kbps,omitempty"`

	STPPortMode  *bool  `json:"stp_port_mode,omitempty"`
	STPBPDUGuard *bool  `json:"stp_bpdu_guard_enabled,omitempty"`
	STPEdgeState string `json:"stp_edge_state,omitempty"`

	LLDPMEDEnabled      *bool  `json:"lldpmed_enabled,omitempty"`
	PortKeepaliveOn     *bool  `json:"port_keepalive_enabled,omitempty"`
	MulticastRouterMode string `json:"multicast_router_mode,omitempty"`

	// Disabled administratively shuts the port down (issue #108). Three-state like
	// the pointer fields above: nil = unmanaged, &true = disable, &false = re-enable.
	// Not a document field — the merge (applyDerivedDisable) translates it to the
	// VLAN-derived shape the controller actually stores. Excluded from the JSON.
	Disabled *bool `json:"-"`
}

// RestoreConfiguredScalars repairs a read that lagged a write. The /rest/device PUT
// lands, but the /stat/device list the Read maps from can briefly echo the old
// value. That stale value contradicts the known plan value and trips "inconsistent
// result after apply". This overlays the configured value back so a known field
// round-trips; the next Read picks up the controller's settled value.
//
// Restored fields are the ones the controller stores VERBATIM (verified live): the
// name, the port-profile binding, and the VLAN intent — the native/voice networks
// and the tagged-VLAN mode.
//
// `forward` is NOT restored: the controller DERIVES it from those intent fields
// (native + tagged_vlan_mgmt + excluded), so it is modeled Computed-only. A Computed
// field is never a known plan value, so a lagging read of it can't trip the
// consistency check, and the read stays authoritative. A field the user left unset
// (empty want) also defers to the controller. (excluded_networkconf_ids — a
// custom-mode-only Set — is left to the array merge; restoring it is a possible
// follow-up.)
func (po *PortOverride) RestoreConfiguredScalars(want PortOverride) {
	restore := func(dst *string, src string) {
		if src != "" && *dst != src {
			*dst = src
		}
	}
	restore(&po.Name, want.Name)
	restore(&po.PortconfID, want.PortconfID)
	restore(&po.NativeNetworkConfID, want.NativeNetworkConfID)
	restore(&po.VoiceNetworkConfID, want.VoiceNetworkConfID)
	restore(&po.TaggedVLANMgmt, want.TaggedVLANMgmt)
	// `disabled` can be a KNOWN plan value (the user set true OR false), but it reads
	// back derived from the controller-computed `forward`, which a lagging read can
	// echo stale. Carry the configured intent so the derived state pins to it. #108.
	// (forward/tagged/native themselves needn't be pinned — they're Computed or
	// user-unset, never known plan values, so a stale read can't trip the post-apply
	// consistency check.)
	if want.Disabled != nil {
		po.Disabled = want.Disabled
	}
}

// DeviceConfig is the owned, writable subset of a device document. Scalars ride
// along as a partial PUT (controller-merged); PortOverrides is rebuilt by merge.
type DeviceConfig struct {
	Name                       string
	LEDOverride                string
	LEDOverrideColor           string
	LEDOverrideColorBrightness *int
	MgmtNetworkID              string
	PortOverrides              []PortOverride
}

// PortOverridesOf returns the device's port_overrides as the curated subset.
// wire.Device.PortOverrides is an anonymous, far-richer struct; rather than
// hand-map two dozen fields (and their enum types), we round-trip it through
// JSON into PortOverride — the json tags align, int/string enums serialize to
// their wire values, and unmodeled keys drop out. Returns nil when the device
// has no overrides.
func PortOverridesOf(d *wire.Device) ([]PortOverride, error) {
	if d == nil || d.PortOverrides == nil {
		return nil, nil
	}
	b, err := json.Marshal(*d.PortOverrides)
	if err != nil {
		return nil, err
	}
	var out []PortOverride
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// List returns every device on the site (legacy /stat/device).
func (s DevicesService) List(ctx context.Context) ([]wire.Device, error) {
	return legacyData[wire.Device](ctx, s.core, http.MethodGet, s.core.legacyPath("stat/device"), nil)
}

// Get returns the device with the given controller _id, or ErrNotFound. Read is
// from the authoritative /stat/device list (the /rest/device GET is thin).
func (s DevicesService) Get(ctx context.Context, id string) (*wire.Device, error) {
	devices, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].UnderscoreId != nil && *devices[i].UnderscoreId == id {
			return &devices[i], nil
		}
	}
	return nil, ErrNotFound
}

// GetByMAC returns the device with the given MAC (case-insensitive), or
// ErrNotFound — the lookup adopt-if-needed does before configuring. The match
// is against the typed wire.Device.Mac (a read-only stat field carried by the
// generated type), filtering the authoritative /stat/device list like Get.
func (s DevicesService) GetByMAC(ctx context.Context, mac string) (*wire.Device, error) {
	want := strings.ToLower(strings.TrimSpace(mac))
	devices, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].Mac != nil && strings.ToLower(*devices[i].Mac) == want {
			return &devices[i], nil
		}
	}
	return nil, ErrNotFound
}

// UpdateConfig applies the owned config to a device. Scalar fields ride along as
// a partial PUT to /rest/device/<id> (controller-merged top-level). port_overrides
// is a single replace-semantics array, so it is rebuilt: ownedPortIdx is the set
// of ports Terraform manages this cycle (config ∪ prior state); each existing
// override keeps its unmodeled keys, declared ports are overlaid, and a
// previously-managed port no longer declared is dropped (reverted). Undeclared,
// never-managed ports are left exactly as the controller has them. The existing
// overrides are read from the authoritative /stat/device list (the /rest/device
// GET is thin on current firmware), and the result is re-read from there too.
func (s DevicesService) UpdateConfig(ctx context.Context, id string, cfg DeviceConfig, ownedPortIdx []int) (*wire.Device, error) {
	if id == "" {
		return nil, fmt.Errorf("UpdateConfig: missing id")
	}
	overlay := map[string]any{}
	if cfg.Name != "" {
		overlay["name"] = cfg.Name
	}
	if cfg.LEDOverride != "" {
		overlay["led_override"] = cfg.LEDOverride
	}
	if cfg.LEDOverrideColor != "" {
		overlay["led_override_color"] = cfg.LEDOverrideColor
	}
	if cfg.LEDOverrideColorBrightness != nil {
		overlay["led_override_color_brightness"] = *cfg.LEDOverrideColorBrightness
	}
	if cfg.MgmtNetworkID != "" {
		overlay["mgmt_network_id"] = cfg.MgmtNetworkID
	}

	// Only rebuild port_overrides when Terraform manages at least one port (or is
	// clearing the last one). Otherwise leave the array entirely untouched.
	if cfg.PortOverrides != nil || len(ownedPortIdx) > 0 {
		existing, err := s.portOverridesRaw(ctx, id)
		if err != nil {
			return nil, err
		}
		merged, err := mergePortOverrides(existing, cfg.PortOverrides, ownedPortIdx)
		if err != nil {
			return nil, err
		}
		overlay["port_overrides"] = merged
	}

	if _, err := legacyData[wire.Device](ctx, s.core, http.MethodPut, s.core.legacyPath("rest/device/"+id), overlay); err != nil {
		return nil, err
	}
	// Re-read from the authoritative /stat/device list rather than trusting the
	// PUT echo (firmware-dependent, may omit port_overrides).
	return s.Get(ctx, id)
}

// portOverridesRaw returns the device's current port_overrides as raw field maps
// (keyed positionally) from the authoritative /stat/device list, so the merge can
// preserve keys the provider doesn't model.
func (s DevicesService) portOverridesRaw(ctx context.Context, id string) ([]map[string]json.RawMessage, error) {
	raws, err := legacyData[map[string]json.RawMessage](ctx, s.core, http.MethodGet, s.core.legacyPath("stat/device"), nil)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		if idEq(raw["_id"], id) {
			po, ok := raw["port_overrides"]
			if !ok || len(po) == 0 {
				return nil, nil
			}
			var arr []map[string]json.RawMessage
			if err := json.Unmarshal(po, &arr); err != nil {
				return nil, fmt.Errorf("decode port_overrides for %s: %w", id, err)
			}
			return arr, nil
		}
	}
	return nil, ErrNotFound
}

// mergePortOverrides rebuilds the port_overrides array. existing is the
// controller's current array; desired is the owned per-port config; ownedPortIdx
// is the full set of ports Terraform manages this cycle. The result keeps every
// existing override for unowned ports, overlays the owned fields of each desired
// port onto its existing override (preserving unmodeled keys), and omits an owned
// port that has no desired entry (removed from config → reverted).
func mergePortOverrides(existing []map[string]json.RawMessage, desired []PortOverride, ownedPortIdx []int) ([]map[string]json.RawMessage, error) {
	owned := make(map[int]bool, len(ownedPortIdx))
	for _, idx := range ownedPortIdx {
		owned[idx] = true
	}
	byIdx := make(map[int]map[string]json.RawMessage, len(existing))
	order := make([]int, 0, len(existing))
	for _, e := range existing {
		idx, err := portIdxOf(e)
		if err != nil {
			// An override without a usable port_idx can't be keyed; keep it as-is
			// under a synthetic negative key preserving order.
			idx = -(len(order) + 1)
		}
		if _, seen := byIdx[idx]; !seen {
			order = append(order, idx)
		}
		byIdx[idx] = e
	}

	for i := range desired {
		idx := desired[i].PortIdx
		fields, err := structToRawMap(desired[i])
		if err != nil {
			return nil, err
		}
		base := byIdx[idx]
		if base == nil {
			base = map[string]json.RawMessage{}
			order = append(order, idx)
		}
		for k, v := range fields {
			base[k] = v
		}
		applyDerivedDisable(base, desired[i])
		byIdx[idx] = base
	}

	result := make([]map[string]json.RawMessage, 0, len(order))
	for _, idx := range order {
		// Drop a port we own but no longer have a desired entry for (reverted).
		if owned[idx] && !hasDesired(desired, idx) {
			continue
		}
		result = append(result, byIdx[idx])
	}
	return result, nil
}

// applyDerivedDisable translates the Disabled write-intent onto a port's merged
// document (the existing override with the desired overlay already applied). The
// controller has no direct disable field — forward is DERIVED from tagged_vlan_mgmt
// + the native VLAN — so the intent becomes that pair (verified against the
// controller's normalizer in service/config/g and the switch firmware, where
// forward=disabled maps to switch.port.<idx>.status=disabled). See issue #108.
//
//   - &true → block_all + an EXPLICITLY empty native. Sending native absent
//     (omitempty) would make the controller inject the default network and derive
//     forward=native, so the empty string must hit the wire literally.
//   - &false → re-enable to the default trunk (auto + native omitted, so the
//     controller injects the default and derives forward=all) — but ONLY for a port
//     the controller currently has disabled, and only when the user manages no VLAN
//     inline. Otherwise it's a no-op: an already-enabled port, or one whose VLAN the
//     user owns, is left to preserve-merge semantics.
//   - nil → unmanaged, no-op.
func applyDerivedDisable(doc map[string]json.RawMessage, po PortOverride) {
	if po.Disabled == nil {
		return
	}
	if *po.Disabled {
		doc["tagged_vlan_mgmt"] = json.RawMessage(`"` + taggedVLANBlockAll + `"`)
		doc["native_networkconf_id"] = json.RawMessage(`""`)
		return
	}
	if !docIsDisabled(doc) || po.TaggedVLANMgmt != "" || po.NativeNetworkConfID != "" || po.PortconfID != "" {
		return
	}
	doc["tagged_vlan_mgmt"] = json.RawMessage(`"` + taggedVLANAuto + `"`)
	delete(doc, "native_networkconf_id")
}

// docIsDisabled reports whether a port's merged document is in the controller's
// disabled shape (derived forward=disabled, or its block_all + empty-native source).
func docIsDisabled(doc map[string]json.RawMessage) bool {
	return string(doc["forward"]) == `"`+ForwardDisabled+`"` ||
		(string(doc["tagged_vlan_mgmt"]) == `"`+taggedVLANBlockAll+`"` && string(doc["native_networkconf_id"]) == `""`)
}

func hasDesired(desired []PortOverride, idx int) bool {
	for i := range desired {
		if desired[i].PortIdx == idx {
			return true
		}
	}
	return false
}

func portIdxOf(m map[string]json.RawMessage) (int, error) {
	raw, ok := m["port_idx"]
	if !ok {
		return 0, fmt.Errorf("override missing port_idx")
	}
	var idx int
	if err := json.Unmarshal(raw, &idx); err != nil {
		return 0, err
	}
	return idx, nil
}

func idEq(raw json.RawMessage, id string) bool {
	if len(raw) == 0 {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return s == id
}

// Adopt adopts a pending device into the site (/cmd/devmgr adopt). HIL-unverified.
func (s DevicesService) Adopt(ctx context.Context, mac string) error {
	return s.devmgr(ctx, "adopt", mac)
}

// Forget removes a device from the site (/cmd/devmgr delete-device). HIL-unverified.
func (s DevicesService) Forget(ctx context.Context, mac string) error {
	return s.devmgr(ctx, "delete-device", mac)
}

func (s DevicesService) devmgr(ctx context.Context, cmd, mac string) error {
	body := map[string]any{"cmd": cmd, "mac": strings.ToLower(strings.TrimSpace(mac))}
	_, err := legacyData[map[string]any](ctx, s.core, http.MethodPost, s.core.legacyPath("cmd/devmgr"), body)
	return err
}
