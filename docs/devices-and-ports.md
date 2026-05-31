# Devices & switch ports

UniFi devices are physical hardware — Terraform can't *create* a switch, only adopt and configure
one that exists. We split that into three objects, each with honest CRUD.

| Object | Kind | Owns | Create | Delete |
|--------|------|------|--------|--------|
| `unifi_network_device` | data source | nothing (read) | — | — |
| `unifi_network_device_adoption` | resource | adopt↔forget | adopt a discoverable device | **forget** (→ pending) |
| `unifi_network_device_configuration` | resource | config overlay | apply overlay | revert overlay |

```hcl
data "unifi_network_device" "sw16" { mac = "00:00:5e:00:53:02" }   # a 16-port PoE switch (example MAC, RFC 7042)

resource "unifi_network_device_adoption" "sw16" {
  mac               = "00:00:5e:00:53:02"
  wait_for_adoption = "5m"               # ctx-cancellable; respects operation timeout
  lifecycle { prevent_destroy = true }   # destroying this FORGETS the device
}

resource "unifi_network_device_configuration" "sw16" {
  device_id = unifi_network_device_adoption.sw16.device_id   # or data.unifi_network_device.sw16.id
  name      = "Switch 16"
  port_overrides = {                     # MAP keyed by port number
    "1"  = { name = "Port 1",  native_network_id = unifi_network_network.trusted.id }
    "9"  = { name = "Port 9",  native_network_id = unifi_network_network.servers.id, poe_mode = "auto" }
  }
}
```

## Lifecycle

**`data.unifi_network_device`** — by `mac`/`name`; exports `id`, `mac`, `model`, `type`, `state`, `adopted`,
`ip`, `version`, `num_ports`. All telemetry lives here, never as configured attributes.

**`unifi_network_device_adoption`** — Create: find by MAC; if pending → adopt and wait (up to
`wait_for_adoption`); if absent and wait `0` → error. Delete: forget (back to pending); `CheckDestroy`
confirms. `mac` is `RequiresReplace`.

**`unifi_network_device_configuration`** — Create/Update: **GET device doc → merge owned fields → PUT** under
a per-device mutex, re-reading inside the lock. Read: refresh only owned fields, normalized. Delete:
revert owned fields to defaults. Owned-field allowlist: `name`, `port_overrides`, `led_override`,
`mgmt_network_id`.

## Why GET-merge-PUT, and ports as a Map

The controller writes the **whole device document** (`PUT /rest/device/<id>`), which carries fields
we don't model (`radio_table`, `config_network`, counters). So we GET, overwrite only owned keys, and
PUT — keeping unmodeled fields as raw passthrough (a `json.RawMessage` carry, not a `map[string]any`
schema). The doc also changes out-of-band, so the read happens inside the per-device lock; a separate
apply or UI edit still means **last-write-wins**.

`port_overrides` is a **`MapNestedAttribute` keyed by port number**, not a list or block — a list
tracks by index, so controller array reordering yields phantom diffs. Per-entry: `name`,
`native_network_id`, `tagged_network_ids`/`excluded_network_ids`, `poe_mode`, `op_mode`,
`port_profile_id`, `port_security_enabled`, `stp_port_mode`, `speed`/`autoneg`,
`aggregate_num_ports`. Omitted ports keep controller defaults; `manage_all_ports` opts into closure.
No standalone per-port resource (N writers racing one device doc).

## New device, in short

Rack/power it → it appears **pending** → add `unifi_network_device_adoption` (MAC + `wait_for_adoption`) and
`unifi_network_device_configuration` (name + ports) → `apply` adopts, waits, merges. Day-2 = ordinary edits.
Destroying the configuration reverts the overlay; destroying the adoption forgets the device. Full
runbook in [flows.md](flows.md).
