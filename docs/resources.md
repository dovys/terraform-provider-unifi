# Resource & data-source catalog

Goal: comprehensive coverage of the UniFi Network feature surface — everything in the live network, plus per-port config.
Resources are app-namespaced `unifi_<app>_<resource>`; **this doc is the Network app** (`unifi_network_*`).
Protect is in [protect.md](protect.md). Singular resources; plural for collection data sources.

**Backend:** `L`=legacy `/rest`, `V2`=`/v2/api/site`, `I`=Integration. **Live:** count in
[my-network.md](my-network.md).

## Resources

### Networking
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_network` | L (I read) | 6 | LAN/VLAN/guest/vlan-only; exposes the full **`purpose`** set + DHCP/DNS/IPv6/IGMP/mDNS. Rejects `purpose=wan`; never hardcodes `purpose=corporate`. |
| `unifi_network_wan` | L | 2 | WAN uplink (pppoe/dhcp/static), load-balance, bandwidth. Same `networkconf` object, `purpose=wan` only. |
| `unifi_network_static_route` | L | 0 | Static route. |
| `unifi_network_port_forward` | L | 0 | DNAT. |
| `unifi_network_dns_record` | V2 | 1 | Local DNS (`/static-dns`); **not** legacy `/rest/dnsrecord`. |
| `unifi_network_dynamic_dns` | L | 0 | DDNS client. |

### Wireless
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_wlan` | L | 3 | SSID: WPA2/WPA3, PMF, network + AP-group binding, MAC filter, fast-roaming, guest. |
| `unifi_network_wlan_group` | L | 2 | Mostly system-managed; import/update only. |
| `unifi_network_ap_group` | V2 | 1 | AP grouping by device MAC. |

### Devices & ports (see [devices-and-ports.md](devices-and-ports.md))
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_device` | L (GET-merge-PUT) + I adopt | 5 | One resource per device, keyed by MAC: adopts if pending, manages config (`name`, `led_override`, `mgmt_network_id`, `port_overrides` — **Map keyed by port number**; owned-field allowlist), exposes computed telemetry. Destroy reverts config; `forget_on_destroy` opts into forget. |
| `unifi_network_port_profile` | L | 0 | Reusable `portconf`, referenced by `port_overrides[*].port_profile_id`. |

There's also a read-only `data.unifi_network_device` for referencing a device you don't manage. No
standalone per-port resource (the device-doc write would race); ports are managed inline as a map.

### Firewall & traffic
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_firewall_rule` | L | 8 | Classic rule. `rule_index` full range 0–99999 + plan-time uniqueness guard. Don't normalize controller-authored names. |
| `unifi_network_firewall_group` | L | 0 | Address/port groups. |
| `unifi_network_firewall_zone` / `unifi_network_firewall_policy` | V2 | 0 | Zone-based model (with ordering). |
| `unifi_network_traffic_rule` / `unifi_network_traffic_route` | V2 | 0 | App/category rules; policy routing. |

The controller runs **either** classic **or** zone-based firewall; the provider detects which and
refuses to mix. The live network is classic.

### Identity, RADIUS, QoS
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_user_group` | L | 2 | Bandwidth/QoS group. Live has 2 → modeled. |
| `unifi_network_radius_profile` / `unifi_network_radius_account` | L | 1 / 0 | RADIUS profile; account (`/rest/account`). |
| `unifi_network_dhcp_reservation` | L | 8 | Fixed-IP reservation, keyed by MAC (`/rest/user`, `use_fixedip`). **Find-or-create by MAC** (controller auto-creates a user per seen MAC); destroy clears the reservation, doesn't delete the user. Only client-object touchpoint. |

### Site & settings
| Resource | Backend | Live | Notes |
|----------|:------:|:----:|-------|
| `unifi_network_site` | L | 1 | Site CRUD (`/cmd/sitemgr`). |
| `unifi_network_setting_*` (`mgmt`, `usg`, `guest_access`, `ntp`, `ips`, `country`/`locale`, …) | L | — | Typed singletons for common keys. |
| `unifi_network_setting` (generic) | L | — | Experimental escape hatch: `key` + normalized-JSON `value` for untyped keys. |

Settings are **one doc per key, always present** → singleton lifecycle: Create = read-then-overwrite;
Delete = revert-to-default or no-op; Read never 404s. Single-ownership enforced (generic
`unifi_network_setting` forbidden for a key a typed resource owns).

### Parity tail (later phases)
`unifi_network_vpn_server`, `unifi_network_vpn_client`, `unifi_network_bgp` — round out the feature surface; live count 0.

## Data sources

`unifi_network_site`, `unifi_network_network` + `unifi_network_networks`, `unifi_network_wlan`,
`unifi_network_ap_group`, `unifi_network_device` + `unifi_network_devices`, `unifi_network_port_profile`,
`unifi_network_firewall_group`, `unifi_network_user_group`, `unifi_network_radius_profile`,
`unifi_network_radius_account`, `unifi_network_dns_record`, `unifi_network_application_info`. (Also ships
`device(s)`, `wlan`, `firewall_group`, and `user_group` data sources.)

## Not modeled

- **Client/end-user devices** (inventory, presence, blocking, per-client QoS by presence) — ephemeral
  runtime state, not config. **Exception:** fixed-IP reservations → `unifi_network_dhcp_reservation`.
- Per-client config (presence, blocking, per-client QoS) → not modeled; fixed-IP needs use
  `unifi_network_dhcp_reservation`, and per-client rate can become a reservation attribute later.
- Schedule tasks → via `unifi_network_setting_mgmt`.
- Cloud **Site Manager API** (read-only); other UniFi OS apps (Access, Talk, …) — later. **UniFi
  Protect is in scope** under `unifi_protect_*` — see [protect.md](protect.md).

## Coverage & deliberate design choices

Built for the full feature surface, with intentional shapes: `device` → one adopt-and-configure
resource + read-only data source (inline port map); per-client config → `unifi_network_dhcp_reservation`; `dns_record` → v2;
`setting` → typed + generic; `rule_index` allows the controller's full range. Also modeled:
`user_group`, `ap_group`, `wlan_group`, `firewall_zone`, `firewall_policy`, `traffic_rule`, and richer
data sources. `bgp`/`vpn_*` are later-phase.
