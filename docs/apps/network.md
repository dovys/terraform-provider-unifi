# UniFi Network app (`unifi_network_*`)

This page lists the Network app's resources and data sources, the API surface each one calls, and the contracts and footguns to know before changing one. The provider is tested against Network application 10.4.57. The aim is to cover the Network app's full configurable surface, with per-switch-port config modeled inline on `unifi_network_device`.

Related pages:

- [../architecture.md](../architecture.md): client, auth, capability gating, and the [per-resource surface map](../architecture.md#per-resource-surface-network).
- [../devices-and-ports.md](../devices-and-ports.md): device lifecycle and `port_overrides`.
- [../flows.md](../flows.md): operator runbooks.
- [protect.md](protect.md), [access.md](access.md), [console.md](console.md): the other apps.
- Generated schema reference: [../resources/](../resources/) and [../data-sources/](../data-sources/). Use-case examples: [../../examples/network/](../../examples/network/).

## Naming

Resources are singular and namespaced by app. Newer resources add a group: `unifi_network_wan_sla` is app `network`, group `wan`, resource `sla`. Only collection data sources are plural (`unifi_network_networks`).

## Surfaces

Both surfaces share one cookie+CSRF session.

| Code | Surface | Path prefix |
|------|---------|-------------|
| L | Legacy | `/proxy/network/api/s/<site>/`, mostly `rest/<type>`, plus `stat/device`, `cmd/devmgr` and `get\|set/setting/<key>` |
| V2 | v2 | `/proxy/network/v2/api/site/<site>/` |

Their write contracts differ: a legacy `rest` PUT shallow-merges at the top level, and a v2 PUT carries `_id` in both URL and body. Both are in [architecture.md §4](../architecture.md#4-write-contract-per-surface).

## Resources

Import takes the controller `_id` unless noted. Objects the controller flags `attr_no_delete` (the default WLAN group, AP group, user group and RADIUS profile) are only removed from state on destroy.

### Networking

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_network` | L `rest/networkconf` | LAN networks: `purpose` is `corporate`, `guest` or `vlan-only`, and changing it forces replacement. `purpose = "wan"` is rejected; use `unifi_network_wan`. Omit it and the controller defaults to `corporate`. Covers DHCP, DNS, IPv6, IGMP and mDNS. The DHCP guard maps `dhcp_guard_servers` to `dhcpd_ip_1..3`; guard on with no server fails at plan time instead of returning `api.err.MissingIPAddress`. Import also accepts `name=<name>`. |
| `unifi_network_wan` | L `rest/networkconf` | Same object with `purpose=wan`. PPPoE, DHCP or static, plus load balancing and bandwidth. `pppoe_password` is sensitive and not read back. Import also accepts `name=<name>`. |
| `unifi_network_wan_sla` | V2 `wan-slas` | WAN failover health monitor: probe targets, interval, down threshold. Import also accepts `name=<name>`. |
| `unifi_network_static_route` | L `rest/routing` | |
| `unifi_network_port_forward` | L `rest/portforward` | DNAT. |
| `unifi_network_dns_record` | V2 `static-dns` | Local DNS records. |
| `unifi_network_dynamic_dns` | L `rest/dynamicdns` | DDNS client. Password is sensitive. |

### Wireless

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_wlan` | L `rest/wlanconf` | SSIDs: WPA2/WPA3, PMF, bands, MLO, minimum rates, network and AP-group binding, MAC filter, RADIUS MAC auth, Hotspot 2.0, fast roaming, guest, user group. `passphrase` is sensitive. |
| `unifi_network_wlan_group` | L `rest/wlangroup` | |
| `unifi_network_ap_group` | V2 `apgroups` | Groups APs by device MAC. |

### Devices and ports

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_device` | L: read `stat/device`, write `rest/device/<id>`, adopt/forget `cmd/devmgr` | One resource per device, keyed by MAC. Adopts a pending device, then manages `name`, `led_override` (+ color and brightness), `mgmt_network_id` and `port_overrides`. Exposes `model`, `type`, `version`, `ip`, `state`, `adopted`. No other device field is written. Import by MAC or `_id`. |
| `unifi_network_port_profile` | L `rest/portconf` | Reusable port config, bound through `port_overrides[*].port_profile_id`. |

The device lifecycle, the `port_overrides` merge, and the per-port VLAN rules are in [../devices-and-ports.md](../devices-and-ports.md).

### Firewall and traffic

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_firewall_rule` | L `rest/firewallrule` | Classic rules. `rule_index` accepts any value `>= 0`; apply checks it is unique within the ruleset. Match on address, port, network or firewall group, and connection state. |
| `unifi_network_firewall_group` | L `rest/firewallgroup` | `address-group`, `port-group` or `ipv6-address-group`. `type` forces replacement. `members` is always sent, as an explicit empty list when unset. |
| `unifi_network_traffic_rule` | V2 `trafficrules` | App and category rules. |
| `unifi_network_traffic_route` | V2 `trafficroutes` | Policy-based routing. |

The provider doesn't detect whether a console runs zone-based firewall; classic rules only.

### Identity, RADIUS and clients

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_user_group` | L `rest/usergroup` | Bandwidth group. |
| `unifi_network_radius_profile` | L `rest/radiusprofile` | Secrets are sensitive. |
| `unifi_network_radius_account` | L `rest/account` | Password is sensitive. |
| `unifi_network_dhcp_reservation` | L `rest/user` | Fixed IP (`use_fixedip`) for a client MAC. |
| `unifi_network_client_ap_lock` | L `rest/user` | Pins a client to one AP (`fixed_ap_enabled`, `fixed_ap_mac`). |

The two client resources are the only ones that touch client records. Both find or create the record by MAC, since the controller creates one for every MAC it sees. Destroy clears only the resource's own fields and never deletes the client. Because both send sparse PUTs, a reservation and an AP lock can share a record. Import by MAC or `_id`.

### Site settings

| Resource | Surface | Notes |
|----------|:------:|-------|
| `unifi_network_settings` | L `get\|set/setting/<key>` | One optional nested block per setting key. Shipped keys: `ntp`, `country`, `locale`. |
| `unifi_network_mdns` | V2 `global/config/network` | mDNS reflector scope: `mode` (`all`, `none`, `some`) and `network_ids` for `some`. |
| `unifi_network_threat_management` | L `get\|set/setting/ips` | IDS/IPS: `mode` (`disabled`, `ids`, `ips`, `ipsInline`), `enabled_categories`, `enabled_networks`, `advanced_filtering_preference`, `restrict_torrents`, `honeypot_enabled`, `memory_optimized`, `content_filtering_blocking_page_enabled`. Per-network honeypot entries stay with the controller. Enabling it fails at plan time on a console whose feature list lacks `IPS`. |

Each setting key is one document that always exists. All three resources are singletons with a fixed state `id`, and destroy only removes them from state.

- `set/setting/<key>` shallow-merges like `rest`, so `unifi_network_settings` and `unifi_network_threat_management` send only changed fields.
- In `unifi_network_settings`, a null block is unmanaged. Removing a block stops managing that key and leaves its values in place. Settings are typed blocks, not a generic JSON map.
- The v2 `global/config/network` PUT replaces the whole document. `unifi_network_mdns` reads it, changes the two mDNS fields and writes it back, so the required IGMP fields survive.

To add a settings key, see [Settings keys in TODO.md](../../TODO.md#settings-keys).

## Data sources

| Data source | Looks up |
|-------------|----------|
| `unifi_network_site` | A site by slug. Defaults to the provider's `site`. |
| `unifi_network_network` | One network by name or id. |
| `unifi_network_networks` | All networks on the site, WANs included. |
| `unifi_network_device` | A device you don't manage, read-only. |
| `unifi_network_wan_network_group` | A WAN network group (`WAN`, `WAN2`) via v2 `wan/networkgroups`. |
| `unifi_network_application_info` | The Network application version detected at configure time. |

## Verification status

Three tiers, described in [../testing.md](../testing.md): Tier 2 runs create, update, import and destroy against a Docker controller; Tier 3 (HIL) imports from a live console and runs a subset of writes with temporary objects.

Gaps:

| Resource | Gap |
|----------|-----|
| `unifi_network_device` | No Tier-2 test. Adopt, forget, config PUT and `port_overrides` writes are unverified on hardware. |
| `unifi_network_client_ap_lock` | No Tier-2 test (the Docker controller has no AP). Live write pending. |
| `unifi_network_wlan_group` | No Tier-2 test. |
| `unifi_network_traffic_route` | Tier-2 test skipped: needs a VPN or WAN egress network the Docker controller lacks. |
| `unifi_network_wan_sla` | Tier-2 test skipped: the monitor `type` is sent lowercase (`icmp`), but the API accepts only `ICMP`, `DNS` and `SPEED_LIMIT`. |
| `unifi_network_wan` | Tier-2 update step skipped: update PUTs return `api.err.InvalidPayload`. |
| `unifi_network_threat_management` | Only `mode = "disabled"` verified; other modes need a CyberSecure license. |
| `unifi_network_mdns` | Live write pending. |
| `unifi_network_network`, `unifi_network_wlan`, `unifi_network_settings`, `unifi_network_dynamic_dns` | Writes covered by Tier 2 but not on hardware. |

Open question: it's unconfirmed whether `unifi_network_wan` `wan_type` and `network_group` can change in place or need `RequiresReplace`.

[TODO.md](../../TODO.md#verify-against-hardware) has the checklist for each item.

## Not modeled

- Client devices: inventory, presence, blocking and per-client QoS are runtime state. The exceptions are DHCP reservations and AP locks.
- Zone-based firewall (`firewall_zone`, `firewall_policy`), VPN server and client, BGP, and site create/delete. See [../roadmap.md](../roadmap.md).
- DPI apps and groups, QoS rules, and the remaining setting keys, including `mgmt` (schedule tasks).
