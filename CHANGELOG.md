# Changelog

All notable changes to this provider are documented here. The format loosely
follows [Keep a Changelog](https://keepachangelog.com/); releases are tagged
`vX.Y.Z` and published to the Terraform Registry as `dovys/unifi`.

## [Unreleased]

_Network app build-out continues; the next tagged release is **0.1.0** once the
network surface is feature-complete._

### Added

- **Phase 2 — Wi-Fi.** SSIDs and the AP groups they broadcast on:
  - `unifi_network_wlan` — a Wi-Fi network/SSID (`wlanconf`, legacy `/rest`),
    modeling the configurable surface: security suite (`open`/`wpapsk`/`wpaeap`)
    with WPA3 + Protected-Management-Frames refinement, the network/VLAN it
    bridges onto, AP groups and bands, MAC filtering, minimum data rates
    (2.4/5 GHz), DTIM, fast roaming, multicast/broadcast handling, and guest
    policies. Updates are GET-merge-PUT so controller-managed fields (inter-AP
    key, per-client PSKs, schedules) are preserved. The `passphrase` is
    `Sensitive` and write-preserved when the controller echoes it empty. Imports
    by `_id`.
  - `unifi_network_ap_group` — the named set of APs a WLAN broadcasts on (v2
    `/apgroups`), honoring the v2 write contract (`_id` in URL and body) and
    sending an explicit empty `device_macs` array so clearing members empties the
    group. The controller's protected default group (`attr_no_delete`) is
    destroy-as-state-removal, never a real delete. Imports by `_id`.
  - `unifi_network_firewall_group` — a reusable address/port group
    (`firewallgroup`, legacy `/rest`): `address-group`, `port-group`, or
    `ipv6-address-group`. Sends an explicit empty `group_members` array so a
    group can be emptied; `type` is immutable. Imports by `_id`.
  - `unifi_network_firewall_rule` — a classic firewall rule (`firewallrule`,
    legacy `/rest`): ruleset/chain, evaluation index, action, protocol,
    source/destination addresses, ports, networks and firewall groups, and
    connection-state matching. The provider rejects a `rule_index` already used
    within the same ruleset (uniqueness guard). GET-merge-PUT preserves
    controller-managed fields. Imports by `_id`.
  - `unifi_network_dhcp_reservation` — a DHCP fixed-IP reservation (legacy
    `/rest/user`), the one client-level object the provider manages. Find-or-create
    by MAC (attaches to a client the controller already tracks); `terraform
    destroy` clears the reservation rather than deleting the client record. Models
    the address, network, alias, and an optional local-DNS companion record.
    Imports by MAC (or `_id`).
  - Examples (all under `examples/network/`): a smart-home scenario (`wifi.tf`)
    with a WPA3-only home SSID, a WPA2/WPA3 guest SSID, and a 2.4 GHz IoT SSID
    bound to the networks in `guest.tf`; `firewall.tf` blocking the IoT VLAN from
    the trusted LAN via a firewall group + rule; `reservation.tf` pinning a NAS;
    and the per-ISP WAN configs. Passphrases/secrets via `Sensitive` variables.

## [0.0.1]

First development release: the provider base and the core of the network app.

### Added

- **Phase 1 — core networking.** First resources and data sources, modeling the
  **full configurable field surface** of each object type (confirmed against a
  live controller, not just commonly-used fields):
  - `unifi_network_network` — LAN/VLAN/guest networks (`networkconf`, legacy
    `/rest`): `purpose` (immutable), VLAN, subnet, NAT/internet-access, IGMP
    snooping, mDNS, UPnP, DHCP-guard; the full DHCP server (pool, lease time,
    DNS, gateway, NTP, WINS, TFTP/PXE boot, option-43/WPAD, conflict checking,
    relay); and IPv6/DHCPv6 (RA, prefix delegation, pools). Updates are
    GET-merge-PUT so controller-managed fields are preserved. Imports by `_id`.
  - `unifi_network_wan` — WAN uplinks (`networkconf`, `purpose=wan`): DHCP/PPPoE
    (write-only `pppoe_password`)/static, WAN/WAN2 group, IPv4 + IPv6 DNS,
    dual-WAN load-balance role + weight + failover priority, smart-queue QoS,
    advertised ISP bandwidth, MAC clone, VLAN tag, and IPv6 delegation.
  - `unifi_network_dns_record` — local DNS records via the v2 `/static-dns`
    surface (A/AAAA/CNAME/MX/TXT/SRV/NS). Routing flips to the Integration DNS
    surface on supporting firmware with no schema change (invariant #2);
    until that path is implemented the router keeps local DNS on v2.
  - Data sources: `unifi_network_network` (look up by `id` or `name`),
    `unifi_network_networks` (list), `unifi_network_site` (slug → UUID), and
    `unifi_network_application_info` (probed version + capability flags).
  - The `Client` gained typed networkconf, static-DNS, and site/application-info
    methods, with legacy-envelope/v2-bare-JSON/Integration-paging helpers and a
    shared `ErrNotFound` mapped to remove-from-state on Read.
  - Unit tests (client CRUD over `httptest` incl. the GET-merge-PUT and v2
    write-contract assertions, model wire shapes, schema validation, conversion
    round-trips) and Tier-2 acceptance tests (create → update → import → destroy
    with a second empty plan, `CheckDestroy`, and `tfacc-` sweepers).
    Hand-written real-use-case examples under `examples/` (a guest-network
    scenario, per-ISP WAN configs using Terraform `import` blocks, local DNS,
    and data-source lookups), plus generated schema-reference docs.
  - The `unifi_network_site` **resource** is deferred past this phase: its
    `/cmd/sitemgr` write shapes can't be verified against a single-site fixture.
    The client carries the write methods; the read-only data source ships now.

- **Phase 0 — base / bootstrap.** The provider scaffold and client foundation:
  - `terraform-plugin-framework` provider (Terraform protocol v6) with a
    configuration schema (`host`, `api_key`, `username`, `password`, `site`,
    `allow_insecure`, `timeout`, `max_retries`) and environment-variable
    fallbacks for each.
  - Dual authentication: a local-admin cookie+CSRF session (`classic`
    transport, with `exp`-driven re-auth and a clear 2FA/cloud-account error)
    and an Integration `X-API-KEY` session (`integration` transport). Both honor
    HTTP 429 with `Retry-After` backoff.
  - A hybrid `internal/unifi` client: the legacy, v2, and Integration surfaces
    sit behind a single `Client` interface, with a capability-aware `router`
    that picks the backend per operation (so a future Integration write is a
    one-line routing change).
  - A `Configure`-time capability probe: Network application version detection
    (legacy `/stat/sysinfo`, with an Integration `application-info` fallback),
    the site name↔UUID map (Integration `/sites`, matched on
    `internalReference`), and version-derived feature flags.
  - Build & release tooling: `Makefile`, GoReleaser config, Terraform
    Registry manifest, `tfplugindocs` tool dependency, golangci-lint config, and
    GitHub Actions for build/lint/unit plus a gated Docker-controller acceptance
    harness and an HIL stub.

- **Toolchain & repo hardening.** Go floor raised to **1.26.3** (CI runs the
  latest stable Go); dependencies updated to latest. The lint job builds
  golangci-lint from source (`install-mode: goinstall`) so its Go always
  satisfies the module directive. `main` is protected: PRs required, the
  build/lint/unit checks are mandatory before merge, and merged branches
  auto-delete.
