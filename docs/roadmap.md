# Roadmap

What the provider ships today, what is deferred and why, and the design questions still open. Known bugs, and writes that ship but aren't yet confirmed on hardware, are tracked in [TODO.md](../TODO.md#verify-against-hardware).

A resource ships when it meets the [Definition of Done](development.md#definition-of-done): its unit tests pass in CI, its acceptance lifecycle passes against the Docker controller (`make testacc`), and, where the hardware exists, it imports real objects on a live console to a clean plan. Each app section below lists its exceptions.

## Compatibility

Network, Protect and Access wire types are generated against these app versions ([Wire types](development.md#wire-types)):

| App | Version |
| --- | --- |
| Network | 10.4.57 |
| Protect | 7.1.69 |
| Access | 4.2.27 |

Console wire types are hand-written and not version-pinned; they are tested against UniFi OS 5.1.x.

Other versions may work. A missing capability fails at plan time, not as a raw API error.

## Shipped

Every resource and data source runs over one cookie and CSRF session as a local admin account. See [architecture.md](architecture.md).

These tables are the coverage checklist: add a new resource or data source to its app's table.

### Network

Design: [apps/network.md](apps/network.md).

| Area | Resources | Data sources |
| --- | --- | --- |
| Networks and WAN | `unifi_network_network`, `unifi_network_wan`, `unifi_network_wan_sla` | `unifi_network_network`, `unifi_network_networks`, `unifi_network_wan_network_group` |
| Site | | `unifi_network_site`, `unifi_network_application_info` |
| Devices and ports | `unifi_network_device`, `unifi_network_port_profile` | `unifi_network_device` |
| Wi-Fi | `unifi_network_wlan`, `unifi_network_wlan_group`, `unifi_network_ap_group` | |
| Firewall | `unifi_network_firewall_rule`, `unifi_network_firewall_group`, `unifi_network_port_forward` | |
| Routing and traffic | `unifi_network_static_route`, `unifi_network_traffic_rule`, `unifi_network_traffic_route`, `unifi_network_user_group` | |
| DNS and DHCP | `unifi_network_dns_record`, `unifi_network_dynamic_dns`, `unifi_network_dhcp_reservation`, `unifi_network_mdns` | |
| Clients | `unifi_network_client_ap_lock` | |
| RADIUS | `unifi_network_radius_profile`, `unifi_network_radius_account` | |
| Settings | `unifi_network_settings`, `unifi_network_threat_management` | |

- `unifi_network_device` reads are confirmed live. Adopt, forget, the config write and the full `device.state` enum wait on the [W3 rig](../TODO.md#w3-rig).
- `unifi_network_client_ap_lock` write is not yet confirmed on hardware.
- `unifi_network_threat_management` modes other than `disabled` need a CyberSecure-licensed gateway.
- `unifi_network_wan` updates and `unifi_network_wan_sla` monitor types are [known bugs](../TODO.md#known-bugs).

### Protect

Design: [apps/protect.md](apps/protect.md). Uses Protect's private REST API under `/proxy/protect/api`. Wire types are generated from the OpenAPI document Protect ships.

| Resources | Data sources |
| --- | --- |
| `unifi_protect_camera`, `unifi_protect_light`, `unifi_protect_sensor`, `unifi_protect_chime`, `unifi_protect_viewer`, `unifi_protect_liveview`, `unifi_protect_nvr_settings`, `unifi_protect_automation` | `unifi_protect_camera` |

- Cameras, liveviews and NVR settings import clean on a live console. The Docker controller has no Protect, and most config writes (device PATCHes, unadopt, liveview slots, automation) are not yet confirmed on hardware; see [TODO.md](../TODO.md#protect).

### Access

Design: [apps/access.md](apps/access.md). Uses Access's private API under `/proxy/access/api`. Access objects are console-wide, not per site.

| Resources | Data sources |
| --- | --- |
| `unifi_access_door_group`, `unifi_access_policy`, `unifi_access_holiday_group`, `unifi_access_door`, `unifi_access_door_guard` | One per resource, same names |

- Every Access resource is built from the API spec and has not yet run against a live Access console; see [TODO.md](../TODO.md#access).

### Console

Design: [apps/console.md](apps/console.md). Uses the UniFi OS API under `/api` on the console itself.

| Resources | Data sources |
| --- | --- |
| `unifi_console_certificate` | |

- Reads and issuance are confirmed end to end on a live console running UniFi OS 5.1.x. Its wire types are hand-written, since UniFi OS ships no OpenAPI for this surface.

## Deferred

| Item | Reason |
| --- | --- |
| `unifi_network_site` resource | The request and response shapes of the `/cmd/sitemgr` create, update and delete calls need a multi-site controller to confirm. The data source and client methods exist. |
| Zone firewall (`firewall_zone`, `firewall_policy`) | Needs a controller running the zone-based firewall to verify the classic-vs-zone capability gate. |
| VPN server, VPN client, BGP | Needs a test rig with a configured VPN and a BGP-capable gateway. |
| Chime per-doorbell `ringSettings` | Refers to ringtone objects that aren't modeled yet. |
| More `unifi_network_settings` keys | Added by value and on request; [TODO.md](../TODO.md#settings-keys) has the order. |

Not planned:

- A standalone `unifi_network_device_port` resource. It would race the device write, so ports stay inline on `unifi_network_device`.
- End-user client devices. The provider models infrastructure.

To request a resource, open an issue with the feature request template.

## Open questions

Each needs a live controller to settle.

1. CSRF across firmware: the transport reads the CSRF token from the response header, falls back to the JWT claim, and re-authenticates from the token's `exp`. Both need checking on more firmware versions.
2. Protect PATCH fields: the writable field set per device type, whether any type needs the legacy `/bootstrap` fallback, whether to model detection zones, and whether the feature flags the camera capability gate reads (including the doorbell LCD flag) reject unsupported features on real cameras.
3. Clearing a field: removing an optional field such as `domain_name` or `dhcp_gateway` from config keeps the controller's value instead of clearing it, because the sparse PUT omits zero values. DNS server lists already clear correctly. Some fields, such as `wan_ip`, reject an empty string, so each new clear needs a live check first.
4. WAN mutability: whether `wan_type` (DHCP, PPPoE, static) and `network_group` (`WAN`, `WAN2`) change in place or need `RequiresReplace`. Network `purpose` is immutable and already uses `RequiresReplace`.
