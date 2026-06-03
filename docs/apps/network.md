# UniFi Network app (`unifi_network_*`)

The Network application's resources and data sources. Goal: comprehensive coverage of the UniFi
Network feature surface, plus first-class per-port config. Resources are app-namespaced
`unifi_<app>_<resource>`; the parallel Protect app is in [protect.md](protect.md). Singular resources;
plural for collection data sources.

Client, auth, and the per-resource surface map live in [../architecture.md](../architecture.md);
device/port internals in [../devices-and-ports.md](../devices-and-ports.md); operator runbooks in
[../flows.md](../flows.md).

**Surface:** `L`=legacy `/rest`, `V2`=`/v2/api/site` — both over the same cookie+CSRF session.

## Resources

### Networking
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_network` | L | LAN/VLAN/guest/vlan-only; exposes the full **`purpose`** set + DHCP/DNS/IPv6/IGMP/mDNS. Rejects `purpose=wan`; omitting `purpose` lets the controller default it (to `corporate`) rather than the provider hardcoding it. |
| `unifi_network_wan` | L | WAN uplink (pppoe/dhcp/static), load-balance, bandwidth. Same `networkconf` object, `purpose=wan` only. |
| `unifi_network_static_route` | L | Static route. |
| `unifi_network_port_forward` | L | DNAT. |
| `unifi_network_dns_record` | V2 | Local DNS via v2 `/static-dns`. |
| `unifi_network_dynamic_dns` | L | DDNS client. |

### Wireless
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_wlan` | L | SSID: WPA2/WPA3, PMF, network + AP-group binding, MAC filter, fast-roaming, guest. |
| `unifi_network_wlan_group` | L | Mostly system-managed; import/update only. |
| `unifi_network_ap_group` | V2 | AP grouping by device MAC. |

### Devices & ports (see [../devices-and-ports.md](../devices-and-ports.md))
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_device` | L (sparse PUT; `port_overrides` array merged) + I adopt | One resource per device, keyed by MAC: adopts if pending, manages config (`name`, `led_override`, `mgmt_network_id`, `port_overrides` — **Map keyed by port number**; owned-field allowlist), exposes computed telemetry. Import captures the switch's live `port_overrides` (so `plan -generate-config-out` mirrors it). Destroy reverts config; `forget_on_destroy` opts into forget. |
| `unifi_network_port_profile` | L | Reusable `portconf`, referenced by `port_overrides[*].port_profile_id`. |

There's also a read-only `data.unifi_network_device` for referencing a device you don't manage. No
standalone per-port resource (the device-doc write would race); ports are managed inline as a map.

### Firewall & traffic
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_firewall_rule` | L | Classic rule. `rule_index` validated `>= 0` (permissive) + apply-time uniqueness guard within a ruleset. Source/dest by address, port, network, or firewall group; connection-state match. Don't normalize controller-authored names. |
| `unifi_network_firewall_group` | L | Address/port groups (`address-group`/`port-group`/`ipv6-address-group`); immutable `type`; sends explicit empty `group_members`. |
| `unifi_network_firewall_zone` / `unifi_network_firewall_policy` | V2 | Zone-based model (with ordering). |
| `unifi_network_traffic_rule` / `unifi_network_traffic_route` | V2 | App/category rules; policy routing. |

The controller runs **either** classic **or** zone-based firewall; the provider detects which and
refuses to mix.

### Identity, RADIUS, QoS
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_user_group` | L | Bandwidth/QoS group. |
| `unifi_network_radius_profile` / `unifi_network_radius_account` | L | RADIUS profile; account (`/rest/account`). |
| `unifi_network_dhcp_reservation` | L | Fixed-IP reservation, keyed by MAC (`/rest/user`, `use_fixedip`). **Find-or-create by MAC** (controller auto-creates a user per seen MAC); destroy clears the reservation, doesn't delete the user. Only client-object touchpoint. |

### Site & settings
| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_network_site` | L | Site CRUD (`/cmd/sitemgr`). *Deferred past P1 — the sitemgr write shapes need a multi-site controller to verify; the read-only `data.unifi_network_site` ships in P1.* |
| `unifi_network_settings` | L (`get`/`set/setting/<key>`) | One resource, one **optional nested block per `setting/<key>`** — sections managed à la carte. Shipped keys: `ntp`, `country`, `locale`; grown over time (see TODO). A generic JSON-blob `unifi_network_setting` was rejected as a smell. |

Settings are **one doc per key, always present**, read via `get/setting/<key>` and written via
`set/setting/<key>` (a shallow merge — send only changed fields; *not* `/rest`). The single resource keeps
**typed, per-key blocks** rather than a JSON blob: a **null block is unmanaged** (Read/apply touch only
declared blocks), removing a block stops managing that section without reverting it, and Delete is state
removal only (the singleton always exists).

### Parity tail (later phases)
`unifi_network_vpn_server`, `unifi_network_vpn_client`, `unifi_network_bgp` — round out the feature surface.

## Data sources

`unifi_network_site`, `unifi_network_network` + `unifi_network_networks`, `unifi_network_wlan`,
`unifi_network_ap_group`, `unifi_network_device`, `unifi_network_port_profile`,
`unifi_network_firewall_group`, `unifi_network_user_group`, `unifi_network_radius_profile`,
`unifi_network_radius_account`, `unifi_network_dns_record`, `unifi_network_application_info`. (Also ships
`device`, `wlan`, `firewall_group`, and `user_group` data sources.)

## Not modeled

- **Client/end-user devices** (inventory, presence, blocking, per-client QoS) — ephemeral runtime
  state, not config. **Exception:** fixed-IP reservations → `unifi_network_dhcp_reservation`
  (per-client rate could become a reservation attribute later).
- Schedule tasks → via the `mgmt` block of `unifi_network_settings` (when that key lands).
- Cloud **Site Manager API** (read-only); other UniFi OS apps (Access, Talk, …) — later. **UniFi
  Protect is in scope** under `unifi_protect_*` — see [protect.md](protect.md).

## Coverage & deliberate design choices

Built for the full feature surface, with intentional shapes: `device` → one adopt-and-configure
resource + read-only data source (inline port map); per-client config → `unifi_network_dhcp_reservation`; `dns_record` → v2;
`setting` → typed + generic; `rule_index` allows the controller's full range. Also modeled:
`user_group`, `ap_group`, `wlan_group`, `firewall_zone`, `firewall_policy`, `traffic_rule`, and richer
data sources. `bgp`/`vpn_*` are later-phase.
