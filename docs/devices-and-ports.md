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

Rack/power it → it appears **pending** → add one `unifi_network_device` (MAC + `wait_for_adoption` +
name + ports) → `apply` adopts, waits, then merges config. Day-2 = ordinary edits. Destroying it
reverts the config and leaves the device adopted; `forget_on_destroy = true` makes destroy forget it.
Full runbook in [flows.md](flows.md).
