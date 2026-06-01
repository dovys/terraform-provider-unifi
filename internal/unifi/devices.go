package unifi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const deviceType = "device"

// Device is a UniFi network device (AP, switch, gateway). Reads come from the
// legacy `/stat/device` list (rich: telemetry + the full config document);
// config writes go to `/rest/device/<_id>` as a partial PUT — the controller
// merges the top-level fields server-side, so we send only the owned ones and
// never a whole document. (The modern v2 `/v2/api/site/<site>/device` endpoint
// the UI port view loads carries live `port_table` state but deliberately omits
// `port_overrides`, so it's a display/telemetry source, not a config source —
// confirmed against a live USW/UCG; we stay on `/stat/device` + `/rest/device`.) `port_overrides`, though, is a single array field with
// replace semantics, so UpdateDevice rebuilds the full array: it overlays the
// owned per-port fields onto each existing override (preserving unmodeled keys
// like `setting_preference`) and leaves undeclared ports untouched.
//
// NOTE: every device on the recon network was already adopted and the recon
// session is read-only, so the config-write, adoption and forget paths are
// modeled from documented behavior and tracked in TODO.md for HIL verification.
// The field set below is confirmed against a live UCG/USW (`/stat/device`);
// `portconf_id`/`poe_mode` inside `port_overrides` are standard documented keys
// but weren't exercised on the recon network (no bound profiles), so they're
// also flagged for HIL.
type Device struct {
	ID      string `json:"_id,omitempty"`
	SiteID  string `json:"site_id,omitempty"`
	MAC     string `json:"mac,omitempty"` // natural key; omitempty so a config PUT can't blank it
	Adopted *bool  `json:"adopted,omitempty"`

	// Telemetry / read-only (controller-owned; never written back).
	Model   string `json:"model,omitempty"`
	Type    string `json:"type,omitempty"` // uap | usw | ugw | udm | …
	Version string `json:"version,omitempty"`
	IP      string `json:"ip,omitempty"`
	State   *int   `json:"state,omitempty"` // 1 = connected (other values HIL-unconfirmed)

	// Config / owned (partial top-level fields the controller merges on PUT).
	Name                       string `json:"name,omitempty"`
	LEDOverride                string `json:"led_override,omitempty"` // default | on | off (null = follow site)
	LEDOverrideColor           string `json:"led_override_color,omitempty"`
	LEDOverrideColorBrightness *int   `json:"led_override_color_brightness,omitempty"`
	MgmtNetworkID              string `json:"mgmt_network_id,omitempty"`

	// PortOverrides is replace-semantics; UpdateDevice rebuilds it via merge.
	PortOverrides []PortOverride `json:"port_overrides,omitempty"`
}

// PortOverride is one entry of a switch's `port_overrides` array, keyed by
// PortIdx. Its field set matches `portconf` (PortProfile) — confirmed against a
// live USW — plus the per-port keys PortIdx, PortconfID (bind to a profile) and
// PoEMode. Pointers + omitempty give three-state semantics inside the merge.
type PortOverride struct {
	PortIdx    int    `json:"port_idx"`              // physical port number (the map key); always sent
	PortconfID string `json:"portconf_id,omitempty"` // bind to a unifi_network_port_profile
	Name       string `json:"name,omitempty"`
	PoEMode    string `json:"poe_mode,omitempty"` // auto | pasv24 | passthrough | off

	// VLAN: Forward is the port's mode ("all" trunk / "native" access / "customize"
	// selective trunk / "disabled"); NativeNetworkConfID is the untagged VLAN;
	// TaggedVLANMgmt picks the trunked VLANs ("auto" all / "block_all" access /
	// "custom" all-except-ExcludedNetworkConfIDs); VoiceNetworkConfID is the VoIP VLAN.
	Forward                string   `json:"forward,omitempty"`                  // all | native | customize | disabled
	NativeNetworkConfID    string   `json:"native_networkconf_id,omitempty"`    // untagged/native network (networkconf _id)
	TaggedVLANMgmt         string   `json:"tagged_vlan_mgmt,omitempty"`         // auto (allow all) | block_all (access) | custom
	ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids,omitempty"` // VLANs to block on the trunk when TaggedVLANMgmt=custom
	VoiceNetworkConfID     string   `json:"voice_networkconf_id,omitempty"`     // voice VLAN (networkconf _id)

	// Link: force Speed/FullDuplex when Autoneg is off.
	Autoneg    *bool `json:"autoneg,omitempty"`     // auto-negotiate speed/duplex
	Speed      *int  `json:"speed,omitempty"`       // forced link speed (Mbps) when autoneg off
	FullDuplex *bool `json:"full_duplex,omitempty"` // forced full duplex when autoneg off

	// Security/control: Isolation blocks inter-port traffic for untrusted ports;
	// Dot1XCtrl is 802.1X access control (force_authorized default | auto require
	// RADIUS | force_unauthorized | mac_based | multi_host); EgressRateLimit* caps
	// outbound throughput.
	Isolation              *bool    `json:"isolation,omitempty"`                      // isolate from other isolated ports
	PortSecurityEnabled    *bool    `json:"port_security_enabled,omitempty"`          // lock to the MAC allow-list
	PortSecurityMACAddress []string `json:"port_security_mac_address,omitempty"`      // allowed MACs when port security on
	Dot1XCtrl              string   `json:"dot1x_ctrl,omitempty"`                     // auto | force_authorized | force_unauthorized | mac_based | multi_host
	Dot1XIdleTimeout       *int     `json:"dot1x_idle_timeout,omitempty"`             // seconds before an authed 802.1X session is cleared
	EgressRateLimitEnabled *bool    `json:"egress_rate_limit_kbps_enabled,omitempty"` // attr name drops the `_kbps_` infix
	EgressRateLimitKbps    *int     `json:"egress_rate_limit_kbps,omitempty"`         // egress cap in kbps when enabled

	// Spanning tree: STPBPDUGuard shuts the port on a received BPDU (rogue-switch/
	// loop protection); STPEdgeState is PortFast-style edge behavior.
	STPPortMode  *bool  `json:"stp_port_mode,omitempty"`          // participate in STP/RSTP
	STPBPDUGuard *bool  `json:"stp_bpdu_guard_enabled,omitempty"` // shut the port on a received BPDU
	STPEdgeState string `json:"stp_edge_state,omitempty"`         // auto | enabled | disabled (edge/PortFast)

	LLDPMEDEnabled      *bool  `json:"lldpmed_enabled,omitempty"`        // advertise LLDP-MED (VoIP voice-VLAN/QoS auto-config)
	PortKeepaliveOn     *bool  `json:"port_keepalive_enabled,omitempty"` // link keep-alives
	MulticastRouterMode string `json:"multicast_router_mode,omitempty"`  // multicast-router (IGMP/MLD querier) handling
}

// ListDevices returns every device on the site (legacy /stat/device).
func (c *client) ListDevices(ctx context.Context) ([]Device, error) {
	return legacyData[Device](ctx, c, http.MethodGet, legacyPath(c.cfg.Site, "stat/device"), nil)
}

// GetDevice finds a device by its controller _id. The legacy /rest/device GET is
// unreliable on current firmware (it returned nothing on recon), so this filters
// the authoritative /stat/device list.
func (c *client) GetDevice(ctx context.Context, id string) (*Device, error) {
	devices, err := c.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].ID == id {
			return &devices[i], nil
		}
	}
	return nil, ErrNotFound
}

// GetDeviceByMAC finds a device by MAC (case-insensitive) — the resource's
// natural key. Returns ErrNotFound if the controller doesn't know the MAC.
func (c *client) GetDeviceByMAC(ctx context.Context, mac string) (*Device, error) {
	want := strings.ToLower(strings.TrimSpace(mac))
	devices, err := c.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if strings.ToLower(devices[i].MAC) == want {
			return &devices[i], nil
		}
	}
	return nil, ErrNotFound
}

// UpdateDevice applies the owned config to a device. Top-level fields ride along
// as a partial PUT (controller-merged). port_overrides is rebuilt: ownedPortIdx
// is the set of ports Terraform manages this cycle (config ∪ prior state); each
// existing override keeps its unmodeled keys, declared ports are overlaid, and a
// previously-managed port that's no longer declared is dropped (reverted).
// Undeclared, never-managed ports are left exactly as the controller has them.
func (c *client) UpdateDevice(ctx context.Context, d *Device, ownedPortIdx []int) (*Device, error) {
	if d.ID == "" {
		return nil, fmt.Errorf("UpdateDevice: missing id")
	}
	overlay := map[string]any{}
	if d.Name != "" {
		overlay["name"] = d.Name
	}
	if d.LEDOverride != "" {
		overlay["led_override"] = d.LEDOverride
	}
	if d.LEDOverrideColor != "" {
		overlay["led_override_color"] = d.LEDOverrideColor
	}
	if d.LEDOverrideColorBrightness != nil {
		overlay["led_override_color_brightness"] = *d.LEDOverrideColorBrightness
	}
	if d.MgmtNetworkID != "" {
		overlay["mgmt_network_id"] = d.MgmtNetworkID
	}

	// Only rebuild port_overrides when Terraform manages at least one port (or is
	// clearing the last one). Otherwise leave the array entirely untouched.
	if d.PortOverrides != nil || len(ownedPortIdx) > 0 {
		existing, err := c.devicePortOverridesRaw(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		merged, err := mergePortOverrides(existing, d.PortOverrides, ownedPortIdx)
		if err != nil {
			return nil, err
		}
		overlay["port_overrides"] = merged
	}

	if _, err := legacyData[Device](ctx, c, http.MethodPut, legacyRESTPath(c.cfg.Site, deviceType, d.ID), overlay); err != nil {
		return nil, err
	}
	// Re-read from the authoritative /stat/device list rather than trusting the
	// PUT echo (which is firmware-dependent and may omit port_overrides), so the
	// returned device is the same source of truth the merge was built from.
	return c.GetDevice(ctx, d.ID)
}

// devicePortOverridesRaw returns the device's current port_overrides as raw field
// maps (keyed positionally), so the merge can preserve keys we don't model.
func (c *client) devicePortOverridesRaw(ctx context.Context, id string) ([]map[string]json.RawMessage, error) {
	raws, err := legacyData[map[string]json.RawMessage](ctx, c, http.MethodGet, legacyPath(c.cfg.Site, "stat/device"), nil)
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
// port that has no desired entry (it was removed from config → reverted).
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
			// by parking it under a synthetic negative key preserving order.
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

// AdoptDevice asks the controller to adopt a pending device by MAC
// (`/cmd/devmgr` `adopt`). HIL-unverified: the recon network had no adoptable
// device. The router prefers the Integration adopt write on supporting firmware
// (OpDeviceAdopt); until that path is wired this uses the legacy command.
func (c *client) AdoptDevice(ctx context.Context, mac string) error {
	_, err := c.devmgr(ctx, map[string]any{"cmd": "adopt", "mac": strings.ToLower(mac)})
	return err
}

// ForgetDevice removes a device from the site (`/cmd/devmgr` `delete-device`).
// HIL-unverified (see AdoptDevice). Tolerates an already-absent device.
func (c *client) ForgetDevice(ctx context.Context, mac string) error {
	_, err := c.devmgr(ctx, map[string]any{"cmd": "delete-device", "mac": strings.ToLower(mac)})
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

func (c *client) devmgr(ctx context.Context, body map[string]any) ([]json.RawMessage, error) {
	return legacyData[json.RawMessage](ctx, c, http.MethodPost, legacyPath(c.cfg.Site, "cmd/devmgr"), body)
}
