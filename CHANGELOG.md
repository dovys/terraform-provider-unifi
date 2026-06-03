# Changelog

All notable changes are documented here as **dated** entries (newest first); add
yours under today's date in the PR that makes the change. Versioning is decoupled
from this file: fixes merged to `main` auto-release the next patch (`vX.Y.Z`),
and features (staged on `next`) ship as a minor (`vX.Y.0`) via a manual workflow
run — both attach binaries, and each GitHub Release carries the per-version
commit list (see [`docs/releasing.md`](docs/releasing.md)). The repo is private
and the provider is **not** published to the Terraform Registry — releases ship
as GoReleaser artifacts on the private repo only. Sections tagged `[vX.Y.Z]`
below predate the date-based convention.

## 2026-06-03

### Changed

- **Releases now follow a two-branch model.** `main` holds only released code;
  features are staged on a `next` branch that never releases. CI only tags and
  builds the release — it never merges or pushes a branch. The bump is derived
  from how a commit reached `main`: a fix PR merged straight in → **patch**; a
  `next → main` PR merged in → **minor** (or **major** with the `release:major`
  label), with the lower parts reset to zero so features land on `vX.Y.0`. A
  feature ships by merging a `next → main` PR (its CI tests the merged result
  first), stopping multi-PR features from emitting a release per PR. See
  [`docs/releasing.md`](docs/releasing.md).

## 2026-06-02

### Added

- **UniFi Access app — substruct client foundation + read-only data source
  (`c.Access()…`).** The console `unifi.Client` gained an `Access()` accessor
  exposing the Access application over the **same cookie session** as Network and
  Protect (the private web-UI API at `/proxy/access/api/...`, no `api_key`,
  console-global). Responses are unwrapped from Access's `{code,msg,data}`
  envelope inside the client. Object types are generated `wire` structs
  (`internal/unifi/access/wire`, oapi-codegen types-only) projected from the
  reverse-engineered Access spec (`../unifi-specs/access/v4.2.27`) by the shared
  projection (`unifi-specs/tools/access_codegen_spec.py`, reusing the Protect
  closure-trim/hoist/widen transforms). Ships `data.unifi_access_door_group`
  (look up a door group by `id` or `name`) and the managed
  `unifi_access_door_group` resource (create/update/delete a named set of doors;
  membership read back from the dedicated `/door_ids` endpoint). door_group is
  the one Access object the RE'd spec provides a write DTO for; its CRUD paths +
  `{code,msg,data}` envelope contract are **HIL-unverified** (no live Access
  console) — see `TODO.md`. Doors, policies, and holidays have no write DTO in the
  spec and await an RE capture of their controller param types.
- **UniFi Protect app — first resources on a new substruct client
  (`c.Protect()…`).** The console `unifi.Client` gained a `Protect()` accessor
  exposing the Protect application over the **same cookie session** as Network
  (the private REST API at `/proxy/protect/api/...`, no `api_key`). Its object
  types are generated `wire` structs (`internal/unifi/protect/wire`,
  oapi-codegen types-only) projected from the spec Protect itself ships
  (`../unifi-specs/protect/v7.1.69`); a deterministic 3.1→3.0 + closure-trim +
  enum/object-hoist projection (`unifi-specs/tools/protect_codegen_spec.py`)
  feeds codegen. New: `unifi_protect_camera` (configure-existing, doorbells
  included; owned-field PATCH merge) + `data.unifi_protect_camera`, and
  `unifi_protect_nvr_settings` (the NVR singleton, import/update-only).
  **Config writes are HIL-unverified** (the recon console is read-only) — see
  `TODO.md`.
- **UniFi Protect — remaining device + live-view resources (P5b).**
  `unifi_protect_light`, `unifi_protect_sensor`, `unifi_protect_chime`, and
  `unifi_protect_viewer` are configure-existing resources keyed by MAC (full
  settings surface; `forget_on_destroy` unadopts), and `unifi_protect_liveview`
  is a full-CRUD multi-camera grid (its `slots` modeled as a nested block from
  the well-known shape). All over the existing `c.Protect()` groups; **all
  HIL-unverified** — see `TODO.md`. Chime per-doorbell `ringSettings` are
  deferred (they reference unmodeled ringtone objects).

### Changed

- **Network client rebuilt on a substruct + codegen design (no resource-surface
  change).** The flat `internal/unifi` method-per-operation client is replaced by
  a substruct `network.Client` (`c.Network().WAN().SLAs()…`) whose object types
  are generated `wire` structs (`internal/unifi/network/wire`, oapi-codegen
  types-only) regenerated from the version-anchored sibling spec
  (`../unifi-specs`). `internal/unifi` is now just the console session
  (transport, capabilities, site management, and the `Network()` accessor).
  Generic collections (`LegacyCollection`, `V2Collection`) carry the per-surface
  CRUD contract; custom collections handle id-in-body GET-merge-PUT and
  List-filter Read. Every resource and data source moved over; the legacy flat
  client and its types were removed. Schemas, resource names, and behavior are
  unchanged. Updated invariant 1 + the resource recipe in `CLAUDE.md`,
  `docs/architecture.md`, and `docs/development.md` accordingly.
- The `device` object spec gained the read-only stat/identity fields
  (`mac`/`model`/`type`/`version`/`ip`/`state`/`adopted`) so `wire.Device`
  surfaces them; the device config write merges `port_overrides` per-port (the
  ported replace-semantics merge) instead of a whole-array PUT.

## [0.1.0] - 2026-06-02

First tagged release: the complete network app (Phases 0–4), plus the Protect
**design** and the reverse-engineering workstream. Ships as private GoReleaser
artifacts — not published to the Terraform Registry.

### Added

- **Project, docs & tooling.**
  - Protect app **design** (`docs/apps/protect.md`): the `unifi_protect_*`
    resources target the private REST API (`/proxy/protect/api/...`) over the
    same cookie session as Network — one session, no `api_key`.
  - **Reverse-engineering workstream** (`docs/reverse-engineering.md`): a
    clean-room method for modeling the private UniFi APIs from the observed
    contract, plus `scripts/redact-capture.py` to strip secrets/MAC/IP/PII from
    captures while preserving field names, enums, and structure.
  - **Dependabot** weekly updates for Go modules and GitHub Actions
    (`.github/dependabot.yml`).

- **Phase 4 — long tail & release prep.** No new resources; the remaining P4
  candidates are deferred against the live-network bar (invariant 7):
  - `vpn_server`/`vpn_client` **deferred** — the live console has no configured
    VPN (no `networkconf` VPN `purpose`; the `openvpn`/`ipsec`/`teleport`/
    `magic_site_to_site_vpn` settings are near-empty global toggles), so the
    per-VPN field surface isn't discoverable and there's no gateway hardware to
    create one. Build on a HIL rig with a real VPN configured (see `TODO.md`).
  - `bgp` **deferred** — no REST/v2 surface or `usg` field exposes BGP here; it's
    an FRR-config feature on specific gateways. Build on BGP-capable hardware.
  - `device_port` **not built** — switch ports are managed inline via
    `unifi_network_device.port_overrides`; a standalone per-port resource would
    race the device write.
  - The Terraform Registry publish + public flip remain **out of scope** (the repo
    stays private); the `0.1.0` network release is a private git tag + GoReleaser
    artifacts only.

- **Phase 3 — Parity & long tail.** Filling out the remaining configurable
  object types:
  - `unifi_network_user_group` — a bandwidth/QoS group (`usergroup`, legacy
    `/rest`) that caps the up/down rate (kbps) of the clients assigned to it.
    Rate caps are `Optional+Computed` (omit, or set `0`, for unlimited).
    GET-merge-PUT preserves controller-managed fields. The controller's
    protected default group (`attr_no_delete`) is destroy-as-state-removal,
    never a real delete. Imports by `_id`.
  - `unifi_network_radius_profile` — a RADIUS profile (`radiusprofile`, legacy
    `/rest`) for WPA-Enterprise/802.1X: ordered `auth_servers`/`acct_servers`
    lists (`{ip, port, x_secret}`, full-replace so removing a server drops it;
    `x_secret` is `Sensitive`), the gateway built-in toggles
    (`use_usg_auth_server`/`use_usg_acct_server`), accounting interim updates,
    and RADIUS-assigned-VLAN behavior. GET-merge-PUT preserves controller-managed
    fields; the protected default profile is destroy-as-state-removal. Imports
    by `_id`.
  - `unifi_network_radius_account` — a RADIUS user (`account`, legacy `/rest`)
    served by the gateway's built-in server: username + write-only `password`
    (`x_password`) and the dynamic-VLAN tunnel attributes (`tunnel_type`,
    `tunnel_medium_type`, `vlan`). Modeled from documented behavior — the field
    set is **HIL-unverified** (no live RADIUS users to confirm against; see
    `TODO.md`). Imports by `_id`.
  - `unifi_network_static_route` — a gateway static route (`routing`, legacy
    `/rest`): a destination `network` reached via a `next_hop` gateway
    (`nexthop-route`), out an `interface` (`interface-route`), or dropped
    (`blackhole`), with an administrative `distance`. GET-merge-PUT preserves
    controller-managed fields. Field set **HIL-unverified** (no live routes; see
    `TODO.md`). Imports by `_id`.
  - `unifi_network_port_forward` — a destination-NAT rule (`portforward`, legacy
    `/rest`): forward a WAN `destination_port` to an internal `forward_ip`/
    `forward_port`, scoped by `interface` (`wan`/`wan2`/`both`), `protocol`, and
    a `source` filter. GET-merge-PUT preserves controller-managed fields. Field
    set **HIL-unverified** (no live rules; see `TODO.md`). Imports by `_id`.
  - `unifi_network_dynamic_dns` — a dynamic-DNS updater (`dynamicdns`, legacy
    `/rest`): publish a WAN `interface`'s IP to a DDNS `service` for a
    `hostname`, with `login` + write-only `password` (`x_password`, preserved
    from config on read) and an optional custom `server`. Field set
    **HIL-unverified** (no live configs; see `TODO.md`). Imports by `_id`.
  - `unifi_network_wlan_group` — a WLAN group (`wlangroup`, legacy `/rest`), the
    older SSID-grouping unit that predates AP groups. Only `name` is configurable
    (confirmed live); the controller's protected default group
    (`attr_no_delete`) is destroy-as-state-removal. Imports by `_id`.
  - `unifi_network_traffic_rule` — a traffic-management rule (v2 `trafficrules`):
    `BLOCK`/`ALLOW`/`SPEED_LIMIT` traffic matched by destination (`INTERNET`,
    `DOMAIN`, `IP`, `REGION`, `APP`, `APP_CATEGORY`) for a set of `target_devices`
    (all clients / a client MAC / a network), with a nested `bandwidth_limit` for
    speed limits. v2 write contract (`_id` in URL + body). Field set
    **HIL-unverified** (no live rules; see `TODO.md`). Imports by `_id`.
  - `unifi_network_traffic_route` — a policy route (v2 `trafficroutes`): send
    traffic matched by destination (`DOMAIN`/`IP`/`REGION`/`INTERNET`) out a
    specific interface/VPN (`network_id`) for a set of `target_devices`, with an
    optional `kill_switch_enabled`. Field set **HIL-unverified** (no live routes;
    see `TODO.md`). Imports by `_id`.
  - The zone-based firewall resources (`firewall_zone`/`firewall_policy`) are
    **deferred**: the test controller runs the classic firewall, so the v2 zone
    endpoints are empty and the classic-vs-zone capability gate can't be verified
    (see `TODO.md`).

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
  - `unifi_network_port_profile` — a reusable switch-port profile (`portconf`,
    legacy `/rest`): VLAN behavior (`forward`, native/voice network, tagged-VLAN
    management with per-network exclusions), PoE mode, link (autoneg/speed/duplex),
    port security and 802.1X, egress rate limiting, STP (port mode, BPDU guard,
    edge state), LLDP-MED, and multicast routing. GET-merge-PUT preserves
    controller-managed fields. Imports by `_id`. (Enum values for `forward`/
    `poe_mode`/`dot1x_ctrl` are modeled from documented behavior pending
    hardware verification — see `TODO.md`.)
  - `unifi_network_device` — the single per-device resource (AP/switch/gateway),
    keyed by MAC: it adopts the device if it's pending, manages its config (name,
    LED override + color/brightness, management network, and per-port
    `port_overrides` as a map keyed by port number), and exposes read-only
    telemetry (model, type, version, ip, state, adopted). `port_overrides` does a
    per-port GET-merge: Terraform manages only the declared ports — each is merged
    onto the controller's existing override (unmodeled keys preserved), undeclared
    ports are left untouched, and dropping a port reverts it. `terraform destroy`
    reverts the managed port overrides by default; `forget_on_destroy = true`
    unadopts the device instead. The read shape is recon-confirmed; **adoption,
    forget, and the config write are HIL-unverified** (see `TODO.md`). Imports by
    MAC (or `_id`). A read-only `data.unifi_network_device` (look up by `mac` or
    `id`) exposes the full device including every `port_overrides` entry.
  - Examples (all under `examples/network/`): a smart-home scenario (`wifi.tf`)
    with a WPA3-only home SSID, a WPA2/WPA3 guest SSID, and a 2.4 GHz IoT SSID
    bound to the networks in `guest.tf`; `firewall.tf` blocking the IoT VLAN from
    the trusted LAN via a firewall group + rule; `reservation.tf` pinning a NAS;
    `port-profile.tf` with an IoT-access and an AP-trunk port profile; `device.tf`
    managing a switch's name/LED/ports (one port bound to a profile); and the
    per-ISP WAN configs. Passphrases/secrets via `Sensitive` variables.

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
    surface (A/AAAA/CNAME/MX/TXT/SRV/NS).
  - Data sources: `unifi_network_network` (look up by `id` or `name`),
    `unifi_network_networks` (list), `unifi_network_site` (slug → site), and
    `unifi_network_application_info` (probed controller version).
  - The `Client` gained typed networkconf, static-DNS, and site methods, with
    legacy-envelope and v2-bare-JSON helpers and a shared `ErrNotFound` mapped to
    remove-from-state on Read.
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
    configuration schema (`host`, `username`, `password`, `site`,
    `allow_insecure`, `timeout`, `max_retries`) and environment-variable
    fallbacks for each.
  - A local-admin cookie+CSRF session (`classic` transport, with `exp`-driven
    re-auth and a clear 2FA/cloud-account error), honoring HTTP 429 with
    `Retry-After` backoff.
  - The `internal/unifi` client: the legacy `/rest` and v2 surfaces — both over
    the single classic session — sit behind one `Client` interface, each method
    targeting its surface directly via the `legacy*`/`v2*` helpers.
  - A `Configure`-time capability probe: Network application version detection
    from legacy `/stat/sysinfo`.
  - Build & release tooling: `Makefile`, GoReleaser config, Terraform
    Registry manifest, `tfplugindocs` tool dependency, golangci-lint config, and
    GitHub Actions for build/lint/unit plus a gated Docker-controller acceptance
    harness and an HIL stub.

- **Toolchain & repo hardening.** Go floor raised to **1.26.3** (CI runs the
  latest stable Go); dependencies updated to latest. The lint job builds
  golangci-lint from source (`install-mode: goinstall`) so its Go always
  satisfies the module directive. `main` is protected: PRs required, the
  build/lint/unit checks are mandatory before merge.
