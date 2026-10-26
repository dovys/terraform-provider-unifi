# Devices and switch ports

Terraform can't create a UniFi switch or access point. It can only adopt hardware the controller already sees, then configure it. The provider models this with one resource, `unifi_network_device`, keyed by MAC, and a read-only data source of the same name.

| Object | Create | Destroy |
|--------|--------|---------|
| `unifi_network_device` resource | Adopts the device if it's pending, then applies config | Reverts the managed port overrides; forgets the device only with `forget_on_destroy = true` |
| `unifi_network_device` data source | Read-only lookup by `mac` or `id` | n/a |

The full attribute reference is generated from the schema: [resource](resources/network_device.md), [data source](data-sources/network_device.md). The bring-up runbook is in [flows.md](flows.md#configure-a-new-device). Worked examples: [`examples/network/device.tf`](../examples/network/device.tf) and [`examples/network/port-profile.tf`](../examples/network/port-profile.tf).

```hcl
resource "unifi_network_device" "lab_switch" {
  mac             = "00:00:5e:00:53:02" # RFC 7042 documentation MAC
  name            = "Lab Switch"
  mgmt_network_id = unifi_network_network.mgmt.id

  port_overrides = {
    "3" = { name = "Desk phone", native_network_id = unifi_network_network.lab.id, voice_network_id = unifi_network_network.voip.id }
    "4" = { name = "Printer", native_network_id = unifi_network_network.lab.id, tagged_vlan_mgmt = "block_all", poe_mode = "off" }
    "8" = { port_profile_id = unifi_network_port_profile.ap_uplink.id }
  }
}
```

The resource also exposes read-only telemetry: `id`, `model`, `type`, `version`, `ip`, `state` (`1` = connected), and `adopted`.

## Lifecycle

### Create

The provider looks the device up by MAC. The device must already be visible to the controller, either adopted or pending adoption; an unknown MAC fails the apply. A pending device is adopted through `/cmd/devmgr`, and an adopted one is taken over as is. The provider then applies the config. It does not wait for adoption to finish.

### Import

Import with `terraform import` or an `import {}` block; the id is the MAC or the controller `_id`. With an `import {}` block, `terraform plan -generate-config-out` writes every port that has a live override, so the generated config mirrors the switch ([flows.md](flows.md)). Trim it to the ports you want to manage. A port you remove from config is reverted on the next apply.

### Read

Refreshes telemetry and the managed config. Telemetry is Computed-only, so refreshing it never produces a diff. Managed config is compared after normalization, and only ports already in state are reconciled. If the device is gone, it is removed from state.

### Update

Sends a sparse write of the owned fields (see [How writes work](#how-writes-work)). `mac` is immutable; changing it replaces the resource.

### Destroy

By default, destroy drops the managed port overrides and removes the device from state. The device stays adopted, and `name`, the LED settings, and `mgmt_network_id` keep their last applied values. With `forget_on_destroy = true`, destroy forgets the device instead; guard that with `lifecycle { prevent_destroy = true }`.

### Data source

Looks up by `mac` or `id` and returns the config, every port override (managed or not), and telemetry. Use it to reference a device that Terraform should not adopt or configure.

Adopt, forget, the `state` codes, and the `poe_mode` values are not yet verified on real hardware. See [TODO.md](../TODO.md).

## Owned fields

The resource owns these top-level fields: `name`, `led_override`, `led_override_color`, `led_override_color_brightness`, `mgmt_network_id`, and `port_overrides`. The controller keeps everything else on the device document, such as `radio_table` and `config_network`, as is.

## Port overrides

`port_overrides` is a map keyed by physical port number. A list would track ports by index, so a controller reorder would show up as a diff.

Ownership rules:

- A declared port is merged onto its existing override. Per-port keys the provider doesn't model are preserved.
- A port you never declared is left exactly as the controller has it.
- A port you remove from config is reverted: its override is dropped from the array.
- Destroy reverts every managed port.

There is no per-port resource. All ports live in one device document, so per-port resources would race each other on the same write.

### Stored and derived fields

The controller stores a port's name, profile binding, and VLAN settings as written but recomputes `forward` on every write, so the provider models it as read-only.

| Field | Behavior |
|-------|----------|
| `name`, `port_profile_id`, `native_network_id`, `voice_network_id`, `tagged_vlan_mgmt` | Stored as written (verified on a live console) |
| `excluded_network_ids` | Written through the array merge. Storage not yet verified. |
| PHY, PoE, and security knobs (`autoneg`, `speed`, `full_duplex`, `poe_mode`, `isolation`, `port_security_*`, `dot1x_*`, `egress_rate_limit_*`, `stp_*`, `lldpmed_enabled`, `port_keepalive_enabled`, `multicast_router_mode`) | Written through the array merge. Storage not yet verified on hardware. |
| `forward` | Derived from `native_network_id`, `tagged_vlan_mgmt`, and `excluded_network_ids`. Read-only. |
| `disabled` | Not a document field. The provider writes the VLAN shape that derives `forward = "disabled"`, and reads `disabled` back from `forward`. |

`tagged_vlan_mgmt` sets the tagged behavior, and the controller derives `forward` from it:

| `tagged_vlan_mgmt` | `forward` | Port carries |
|--------------------|-----------|--------------|
| `auto` | `all` | Native untagged, every other network tagged (trunk) |
| `block_all` | `native` | Native untagged only (access port) |
| `custom` | `customize` | Native untagged, every other network tagged except `excluded_network_ids` |

`excluded_network_ids` only applies with `custom`.

### The native VLAN is never tagged

A port's native network is its untagged network and is never in its tagged set, whatever the `forward` mode. This rule is behind the [management VLAN footgun](#footgun-the-native-vlan-must-not-be-the-management-vlan).

### Disabling a port

`disabled = true` shuts the port down. The provider writes it as `tagged_vlan_mgmt = "block_all"` with an explicitly empty native network. If the native network were omitted, the controller would fill in the default network and derive `forward = "native"` instead of `disabled`.

`disabled = false` re-enables the port as the default trunk (`tagged_vlan_mgmt = "auto"`) when no `port_profile_id`, `native_network_id`, or `tagged_vlan_mgmt` is set. If any of those is set, the provider writes them instead, so the port comes back on that VLAN shape.

`disabled` can't be combined with `port_profile_id`, `native_network_id`, `voice_network_id`, `excluded_network_ids`, or a `tagged_vlan_mgmt` other than `block_all`; combining them is a plan error. To disable a port bound to a profile, remove `port_profile_id` and set `disabled = true`.

### Port profiles

Setting `port_profile_id` binds the port to a [`unifi_network_port_profile`](resources/network_port_profile.md). The profile then governs VLAN behavior, and the controller nulls the port's inline `native_network_id`, `tagged_vlan_mgmt`, and `forward`. Leave those unset on a bound port.

Use inline fields for a one-off port and a profile when several ports share a policy. Mirror and aggregate modes (`op_mode`) exist only on the profile.

A standalone port profile stores `forward` as written. The controller derives it only when the profile is bound to a device port.

## `mgmt_network_id`: the device's own management VLAN

`mgmt_network_id` is the network a switch or AP uses for its own control-plane traffic: inform, adoption, and telemetry. The device 802.1Q-tags that traffic with the network's VLAN, so it reaches the controller and gets its management IP on that subnet. The provider writes it to the device document's `mgmt_network_id` key as a network `_id`. It is separate from `config_network`, the device's IP mode (DHCP or static), which the provider doesn't model.

A port's `native_network_id` only governs untagged frames. Because the device tags its management traffic, it lands on the management subnet even when its upstream port has a different native network. An AP on a port whose native network is `lab` still shows up on `mgmt`.

### Footgun: the native VLAN must not be the management VLAN

The native VLAN is never tagged, so a port whose native network is the device's management network drops the device's tagged management frames. The device loses contact with the controller and can't get a management address. The provider doesn't catch this at plan time.

With networks `default` (untagged) and `mgmt` (tagged), on the switch port that feeds an AP with `mgmt_network_id = unifi_network_network.mgmt.id`:

```hcl
"8" = {
  native_network_id = unifi_network_network.default.id # not mgmt
  tagged_vlan_mgmt  = "auto"                           # trunk: mgmt rides tagged
}
```

On a `custom` trunk, set the native network to one the device never tags, such as an unused default network, and don't exclude the management network.

The same applies when you change `mgmt_network_id`: every port between the device and the controller must carry the new network tagged, or the device strands itself. To recover, fix the upstream port's VLANs in the UniFi UI or with a corrected apply.

## How writes work

The device write is a sparse `PUT /rest/device/<id>`. The legacy `/rest` PUT shallow-merges on the server, so the provider sends only the owned top-level keys it has values for, and the controller keeps the rest.

`port_overrides` is the exception. It is a JSON array, and the controller replaces arrays wholesale. When ports are managed, the provider reads the current array from `/stat/device`, overlays the declared ports, keeps unmodeled per-port keys as raw JSON, and sends the merged array.

There is no locking. Two `unifi_network_device` resources for the same MAC, or a UI edit during an apply, are last-write-wins on the port array. Keep one resource per device.

`/stat/device` can briefly return pre-write values after a PUT. On the read that ends an apply, the provider restores the configured values of the fields the controller stores verbatim, so Terraform doesn't report "Provider produced inconsistent result after apply". Those fields are the name, profile binding, native and voice networks, `tagged_vlan_mgmt`, and `disabled`. `forward` is read-only, so it isn't restored. The next refresh shows the settled values.
