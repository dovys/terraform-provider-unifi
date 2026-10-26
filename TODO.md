# TODO

Open work that isn't a roadmap phase. Phases, deferred resources and design questions live in [`docs/roadmap.md`](docs/roadmap.md). Test tiers are described in [`docs/testing.md`](docs/testing.md). Below, "live write run" means the W0–W2 rungs of `./test-hil.sh write` against a live console, and "W3 rig" means a disposable controller and devices.

## Known bugs

- `unifi_network_wan_sla`: the provider sends the monitor `type` in lowercase (`icmp`), but the API accepts only `ICMP`, `DNS` and `SPEED_LIMIT`. Fix the validator and wire value, then un-skip `test/acc/wan_sla_acc_test.go`.
- `unifi_network_wan`: an update PUT returns `api.err.InvalidPayload` for any field. The Tier-2 test skips the update step until this is fixed.

## Verify against hardware

<!-- docs/testing.md and test/hil/README.md link to this heading by name. -->

These writes ship but have not been confirmed on real hardware. Their schemas should carry the `HIL-unverified` marker; *Schema text* below tracks the gaps.

### Network

- `unifi_network_client_ap_lock` (#82). Needs: live write run with a real AP. No Tier-2 test exists, since the Docker controller has no AP. Confirm:
  - the controller accepts an unadopted `ap_mac` (check before writing one);
  - the sparse `fixed_ap_*` PUT pins the client and leaves a coexisting reservation's `fixed_ip` intact;
  - re-pinning `ap_mac` takes effect;
  - destroy (`fixed_ap_enabled = false`) clears the lock without forgetting the client.
- `unifi_network_mdns` (#91). Needs: live write run. The v2 `global/config/network` PUT replaces the whole document, so the resource GET-merges before writing. Tier 2 covers the lifecycle. Confirm:
  - the required IGMP fields survive an mDNS-only change;
  - `mode = "some"` with `network_ids` scopes the reflector;
  - the live GET matches `wire.Global`.
- `unifi_network_threat_management` (#90). Needs: CyberSecure-licensed gateway. Without a subscription the controller keeps `ips_mode = "disabled"` and empties `enabled_categories`, so only the disabled path is verified. Confirm:
  - `ips_mode` (`ids`, `ips`, `ipsInline`) and `enabled_categories` round-trip;
  - whether categories are accepted while `ips_mode = "disabled"`;
  - the per-network `honeypot` array survives the shallow merge;
  - the unmodeled `utm_token`, `ad_blocking_enabled`, `dns_filtering` and `endpoint_scanning` fields are server-managed.

### Protect

- Camera capability gate. Needs: live console with cameras. Confirm `ModifyPlan` rejects a feature on a camera without the HDR, mic, status-LED, smart-detect or doorbell-LCD flag. Only the positive case has been seen.
- `unifi_protect_liveview`. Needs: live write run. W0 confirmed a liveview with no slots; confirm the slot shape (cameras and cycle settings).
- `unifi_protect_automation` (#117). Needs: live write run. Confirm the batch PATCH of the sole `HTTP_REQUEST` action's header and the list-then-filter Read.

### Access

`unifi_access_door_group`, `unifi_access_policy`, `unifi_access_holiday_group`, `unifi_access_door` and `unifi_access_door_guard` are built from the API spec and have never run against a live console. Needs: live Access console. Confirm:

- the CRUD paths, the success codes in the `{code, msg, data}` envelope, and the `/door_ids` response shape;
- `unifi_access_door` `floor_name` and `unifi_access_policy` `schedule_id` and `resources`, which the read view doesn't echo, so refresh can't detect drift on them;
- an Access capability probe, so a console without Access gets a plan-time diagnostic (invariant 3) instead of a raw error at apply or a 404 that drops resources from state.

### Console

- `unifi_console_certificate`. Needs: live write run. The Docker controller has no unifi-core surface, so no automated test covers the create, update, import and destroy lifecycle; Tier 3 only lists and decodes certificates.

### W3 rig

These mutate shared state or have an outward effect, so they wait for a W3 rig (see the W3 row in [`test/hil/README.md`](test/hil/README.md#rungs)).

- `unifi_network_device`: adopt and forget via `/cmd/devmgr`, the config PUT, the `device.state` enum, and `port_overrides` writes, including the accepted `poe_mode` values.
- `unifi_network_network`: create and update a real, non-inert network.
- `unifi_network_wlan`: create and update an enabled SSID.
- `unifi_network_settings`: write semantics.
- `unifi_network_dynamic_dns`: covered by Tier 2, never written on hardware.
- Protect devices (camera, light, sensor, chime, viewer): the config PATCH paths, and unadopt through `forget_on_destroy`.
- `unifi_protect_nvr_settings`: writes.

### Open questions

These need a live controller; the [roadmap](docs/roadmap.md) open questions have detail.

- `unifi_network_wan`: whether `wan_type` and `network_group` change in place or need `RequiresReplace`.
- Sparse PUT: how to clear an owned `omitempty` field.

### Schema text

- `unifi_network_client_ap_lock`, `unifi_network_mdns` and `unifi_network_wan_sla` lack the `HIL-unverified` marker, and `unifi_network_threat_management` words it differently. The `wan_sla` text should also flag the `type` casing bug.
- `unifi_protect_liveview` marks the whole resource `HIL-unverified`; narrow it to slots.

## Settings keys

`unifi_network_settings` ships `ntp`, `country` and `locale`. Next, highest value first: `mgmt`, `guest_access`, `global_switch`, `global_ap`, `radius`.

Long tail, added on request: `snmp`, `netflow`, `radio_ai`, `rsyslogd`, `dpi`, `teleport`, `baresip`, `lcm`, `usg`, `doh` and others. Out of scope: `super_*` (console-internal) and `porta` (hardware-only).

To add a key: write one `settingKeyHandler` entry and its schema block in `internal/provider/network_settings_resource.go`, mark `x_*` fields `Sensitive`, and confirm the key's live GET field set and shallow-merge behavior.
