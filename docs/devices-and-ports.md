# Devices & switch ports

UniFi devices are physical hardware — Terraform can't *create* a switch, only **adopt** one that
exists and configure it. So a device is **one resource**, `unifi_network_device`, keyed by MAC: it
adopts the device if it isn't already, manages its config, and exposes the device's read-only
telemetry for other resources to reference. A read-only `data.unifi_network_device` exists too, for
the case where you want to reference a device but *not* manage it.

| Object | Kind | Create | Delete |
|--------|------|--------|--------|
| `unifi_network_device` | resource | adopt-if-pending, then apply config | revert config; **forget only if `forget_on_destroy = true`** |
| `unifi_network_device` | data source | — (read-only lookup) | — |

```hcl
resource "unifi_network_device" "sw16" {
  mac               = "00:00:5e:00:53:02"   # example MAC (RFC 7042) — the one on the label
  wait_for_adoption = "5m"                  # ctx-cancellable; respects operation timeout
  name              = "Switch 16"
  port_overrides = {                        # MAP keyed by port number
    "1" = { name = "Port 1", native_network_id = unifi_network_network.trusted.id }
    "9" = { name = "Port 9", native_network_id = unifi_network_network.servers.id, poe_mode = "auto" }
  }
}

# Reference its live, read-only attributes anywhere:
#   unifi_network_device.sw16.ip   .model   .state   .version   .num_ports
```

## Lifecycle

**Create** — find the device by MAC. If it's **pending** → adopt and wait (up to `wait_for_adoption`;
absent with wait `0` → error, so a clean apply means real hardware). If it's **already adopted** →
take over management, no re-adopt. Then apply the config overlay. `terraform import` does the same
for an already-managed device — import key = **MAC**.

**Read** — refresh the **Computed telemetry** (`ip`, `model`, `type`, `state`, `adopted`, `version`,
`num_ports`) and reconcile only the **owned config** fields, normalized. Telemetry is Computed-only,
so it refreshes without ever producing a phantom diff.

**Update** — **GET device doc → merge owned fields → PUT** under a per-device mutex, re-reading inside
the lock. Owned-field allowlist: `name`, `port_overrides`, `led_override`, `mgmt_network_id`.

**Delete** — **safe by default:** revert owned fields to their controller defaults and drop from
state; the device stays adopted and online. Set `forget_on_destroy = true` to actually forget it
(back to pending) — the dangerous path; guard it with `lifecycle { prevent_destroy = true }` where it
matters. Create adopts but destroy doesn't un-adopt by default; that asymmetry is intentional —
you're bringing existing hardware under management, not provisioning it. `mac` is `RequiresReplace`.

**`data.unifi_network_device`** — by `mac`/`name`; exports the same telemetry (`id`, `mac`, `model`,
`type`, `state`, `adopted`, `ip`, `version`, `num_ports`). Use it to reference a device you do **not**
want Terraform to adopt or configure; otherwise just use the resource.

## `mgmt_network_id` — the device's own management VLAN

`mgmt_network_id` is the **management VLAN** an infrastructure device (switch/AP) uses for its own
control-plane traffic — inform, adoption, telemetry. The device **802.1Q-tags that traffic** onto the
referenced network's VLAN, so it reaches the controller and gets its management IP on **that subnet,
regardless of the upstream switch port's native VLAN — with one exception below**. It serializes to the
device document's `mgmt_network_id` key (a `networkconf` `_id`) and is **distinct from** the device's
`config_network` object, which is only its IP-addressing *mode* (`dhcp`/`static`) and which this
provider does not model.

This is why it interacts with `native_network_id` the way it does: a port's `native_network_id` governs
only the **untagged** frames of an attached endpoint. An infra device with `mgmt_network_id` set *tags*
its management frames, so it lands on the management VLAN even when the port has a different native VLAN
— e.g. an AP on a downlink port whose native VLAN is `iot` still appears on the management subnet,
because its inform traffic is tagged onto mgmt.

**The exception — and it's a footgun: the port's native VLAN must not itself be the management VLAN.**
A native VLAN is by definition the *untagged* one, so it is never part of the port's tagged set. Make
the native VLAN equal to `mgmt_network_id`'s VLAN and the device's own tagged management frames are
dropped — it loses adoption, fails DHCP on the management subnet, and strands itself on a self-assigned
fallback address off every managed VLAN while (for an AP) still serving Wi-Fi from its last-known config,
so nothing looks broken. On a `customize` trunk carrying tagged management, set `native_network_id` to a
VLAN the device never tags — the unused default VLAN is the safe parking slot.

## Why merge `port_overrides`, and ports as a Map

The device write is a **sparse PUT** (`PUT /rest/device/<id>`) — the legacy `/rest` PUT shallow-merges
server-side, so we send only the owned top-level keys (`name`, `led_override`, `mgmt_network_id`) and
the controller keeps everything we don't model (`radio_table`, `config_network`, counters) untouched.
The exception is **`port_overrides`**: it's a JSON *array*, which the controller replaces wholesale
rather than merging element-by-element. So that one field we GET, overlay the owned ports onto the
existing array (raw `json.RawMessage` passthrough for unmodeled per-port keys, not a `map[string]any`
schema), and send back merged. That array read happens inside the per-device lock; a separate apply or
UI edit still means **last-write-wins**.

`port_overrides` is a **`MapNestedAttribute` keyed by port number**, not a list or block — a list
tracks by index, so controller array reordering yields phantom diffs. Per-entry: `name`,
`native_network_id`, `tagged_network_ids`/`excluded_network_ids`, `poe_mode`, `op_mode`,
`port_profile_id`, `port_security_enabled`, `stp_port_mode`, `speed`/`autoneg`,
`aggregate_num_ports`. Omitted ports keep controller defaults; `manage_all_ports` opts into closure.
No standalone per-port resource (N writers racing one device doc).

`forward` (the VLAN mode: `all`/`native`/`customize`) is **read-only** — the controller derives it from
the intent fields (`native_network_id` + `tagged_vlan_mgmt` + `excluded_network_ids`) and recomputes it
on every write, so the practitioner sets the intent and reads `forward` back. (Verified live: every value
written to a device port came back recomputed. A standalone `unifi_network_port_profile` stores `forward`
verbatim because the derivation runs only when a profile is *bound* to a device port.)

## New device, in short

Rack/power it → it appears **pending** → add one `unifi_network_device` (MAC + `wait_for_adoption` +
name + ports) → `apply` adopts, waits, then merges config. Day-2 = ordinary edits. Destroying it
reverts the config and leaves the device adopted; `forget_on_destroy = true` makes destroy forget it.
Full runbook in [flows.md](flows.md).
