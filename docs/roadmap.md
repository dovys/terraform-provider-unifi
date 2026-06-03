# Roadmap, checklist & open questions

Phasing is **vertical**: each phase ships finished resources (schema + backends + docs + unit +
acceptance) rather than half-built breadth. A phase is done when its resources import the live
network to a clean plan (where instances exist) and pass CI.

## Phases

- **0 — Bootstrap:** scaffold (`terraform-provider-scaffolding-framework`, protocol v6); `internal/unifi`
  skeleton (`Client`, `classic` cookie+CSRF transport over legacy + v2, `capabilities`); provider
  config + auth + `Configure` probe; CI (unit + Docker-controller harness + stub HIL job);
  `tfplugindocs`/`goreleaser`/manifest/`Makefile`; **initial commit** (also unblocks web `/ultraplan`).
- **1 — Core networking (v0.1):** `unifi_network_network`, `unifi_network_wan`, `unifi_network_dns_record` (v2);
  data sources `network` + `networks`, `site`, `application_info`. Import live VLANs + WANs + DNS clean.
  The `unifi_network_site` **resource** is deferred past P1: its `/cmd/sitemgr` write shapes can't be verified
  against the single-site fixture (recipe step 1) — the read-only `site` data source ships now; the client
  carries the sitemgr write methods for when a multi-site controller is available.
- **2 — Devices, ports, firewall, reservations:** `unifi_network_device` (adopt-if-needed + configure,
  port map; read-only `data.unifi_network_device`), `unifi_network_port_profile`; `unifi_network_wlan`, `unifi_network_ap_group`;
  `unifi_network_firewall_rule` (+ uniqueness guard), `unifi_network_firewall_group`; `unifi_network_dhcp_reservation`.
  Import the live devices, WLANs, firewall rules, and reservations clean.
- **3 — Parity & long tail:** `user_group`, `radius_profile`/`radius_account`, `wlan_group`;
  `static_route`, `port_forward`, `dynamic_dns`; zone firewall (`firewall_zone`/`firewall_policy`),
  `traffic_rule`/`traffic_route` (tested on HIL); `setting_*` + generic `unifi_network_setting`.
- **4 — Long tail & release prep:** `vpn_server`/`vpn_client`/`bgp` are **deferred** — live recon found
  no configured VPN (no `networkconf` VPN `purpose`; the `openvpn`/`ipsec`/`teleport`/
  `magic_site_to_site_vpn` settings are near-empty global toggles) and **no** BGP REST/v2 surface or
  `usg` field, so neither has a verifiable instance and no gateway hardware can enable them; modeling
  them blind would break clean-room (invariant 7). Build once a configured VPN / a BGP-capable gateway
  is on a HIL rig (see `TODO.md`). The standalone `unifi_network_device_port` resource is **not** built
  — ports are managed inline via `unifi_network_device.port_overrides` (a per-port resource would race
  the device write). **Registry publish + flip-public are out of scope** — the repo stays private; the
  `0.1.0` network release ships as a private git tag + GoReleaser artifacts only, never the Registry.
- **5 — Protect app (`unifi_protect_*`, ships as 0.2.0):** the private REST API the web UI uses
  (`/proxy/protect/api/...`) over the **same cookie session** as Network — one session, **no `api_key`**;
  the official Integration API is too limited for completeness, and per the either-or-per-resource rule
  Protect commits to private REST (Integration stays a permitted-but-unused option, so invariant 2 holds).
  Field schemas come from the **reverse-engineering** workstream ([reverse-engineering.md](reverse-engineering.md)),
  not yet modeled. Resources: `unifi_protect_camera` (configure-existing + adopt-if-needed [HIL]; doorbells
  are cameras; read-only `data.unifi_protect_camera`), `_light`/`_sensor`/`_chime`/`_viewer`, `_liveview`
  (CRUD), `_nvr_settings` (singleton). **HIL-only writes** (no Protect in the Docker controller). See
  [apps/protect.md](apps/protect.md).

## Coverage checklist

A resource is checkable only when it meets the Definition of Done ([development.md](development.md)).

- [x] base: provider scaffold, `classic` cookie+CSRF client (legacy + v2 behind `Client`),
      `Configure` capability probe, CI + release tooling — **P0**
- [x] network, wan, dns_record (v2); data sources network/networks, site, application_info — **P1**
      (`site` *resource* deferred: `/cmd/sitemgr` write shapes need a multi-site controller to verify)
- [x] device (single adopt+configure resource w/ port map) + data source, port_profile, wlan,
      ap_group, firewall_rule, firewall_group, dhcp_reservation — **P2**
      (device adopt/forget + config write are HIL-unverified; the read shape is recon-confirmed — see `TODO.md`)
- [x] **P3** parity & long tail:
  - [x] user_group, radius_profile, radius_account, static_route, port_forward, dynamic_dns,
        wlan_group, traffic_rule, traffic_route
  - **deferred — settings:** `setting_*` is its own sizable typed surface (≈39 singleton
        `setting/<key>` docs). A generic JSON-blob `unifi_network_setting` was rejected as a smell
        (abandons the provider's typed/validated philosophy); the meaningful settings should be
        modeled as **typed** import/update-only resources in a dedicated effort (see `TODO.md`).
  - **deferred — zone firewall:** firewall_zone, firewall_policy — the live controller runs the
        **classic** firewall, so the zone-based v2 endpoints are empty and the classic-vs-zone
        capability gate can't be verified; build once a zone-firewall controller is available
        (see `TODO.md`)
- [x] **P4** long tail & release prep:
  - **deferred — vpn_server, vpn_client, bgp:** no verifiable live instance (no configured VPN, no BGP
        surface, no gateway hardware to enable either); build on a HIL rig once available (see `TODO.md`)
  - **not built — device_port:** ports are managed inline via `unifi_network_device.port_overrides`;
        a standalone per-port resource would race the device write
  - **out of scope — Registry publish / flip public:** repo stays private; `0.1.0` is a private tag +
        GoReleaser artifacts only
- [x] **P5/P6** Protect app (`unifi_protect_*`, ships as **0.2.0**): design ([apps/protect.md](apps/protect.md))
      + the **reverse-engineering** workstream ([reverse-engineering.md](reverse-engineering.md)).
  - [x] **P5a — foundation + first resources:** `Protect()` substruct client over the cookie session
        (`internal/unifi/protect`, private REST `/proxy/protect/api`, no `api_key`); generated `wire`
        types projected from the shipped Protect OpenAPI (`../unifi-specs/protect/v7.1.69` →
        `protect_codegen_spec.py`, 3.1→3.0 + closure-trim + enum/object-hoist). Resources:
        `unifi_protect_camera` (configure-existing + owned-field PATCH merge) + `data.unifi_protect_camera`,
        `unifi_protect_nvr_settings` (singleton). Live-read import-clean (camera + NVR exist); **config
        writes HIL-unverified** (recon console is read-only — see `TODO.md`).
  - [x] **P5b — remaining resources:** `unifi_protect_liveview` (pure CRUD; `slots` modeled as a nested
        block — `cameras` + cycle settings — from the well-known shape, HIL-unverified), plus the
        configure-existing device resources `unifi_protect_light`, `unifi_protect_sensor`,
        `unifi_protect_chime`, `unifi_protect_viewer` (over the existing client groups; full settings
        surface; `forget_on_destroy` unadopts). **All HIL-unverified** (no live device on the recon
        console — see `TODO.md`); chime per-doorbell `ringSettings` deferred (references unmodeled
        ringtone objects).
- [x] **P7** Access app (`unifi_access_*`): design ([apps/access.md](apps/access.md)). Access ships only an
      Integration-API openapi (wrong surface); the private web-UI API is reverse-engineered from the Access
      Go binary (`../unifi-specs/access/v4.2.27`), so fidelity is lower than Protect's vendor OpenAPI and the
      surface **cannot be live-verified** (no Access console; X-API-KEY is Network-only) — every Access write
      surface is therefore **HIL-unverified**, tracked in `TODO.md`.
  - [x] **P7a — foundation + read-only data source:** `Access()` substruct client over the cookie session
        (`internal/unifi/access`, private web-UI API `/proxy/access/api`, `{code,msg,data}` envelope, no
        `api_key`, console-global); generated `wire` types projected from the RE'd spec via the shared
        `access_codegen_spec.py` (reuses the Protect closure-trim/hoist/widen transforms). Ships
        `data.unifi_access_door_group`.
  - [x] **P7b — first managed resource:** `unifi_access_door_group` ships (registered). It is the one Access
        object the RE'd spec provides a write DTO for (`door_group_controller_DoorGroupParams`); its CRUD
        paths, the envelope success-code set, and the `/door_ids` response shape remain **HIL-unverified**
        (see `TODO.md`).
  - [x] **P7c — policies, holidays, doors, door guards:** an RE capture added the missing write DTOs to
        `../unifi-specs/access` (`access_policy_bo_*`, `holiday_bo_*`, `location_bo_UpdateDoorParams`,
        `door_guard_bo_UpdateDoorGuard`); the `wire` regen picked them up. Ships `unifi_access_policy`,
        `unifi_access_holiday_group`, `unifi_access_door` (update/import-only on discovered hardware), and
        `unifi_access_door_guard` (update/import-only), each with a read-only data source. Doors and door
        guards configure discovered hardware, so destroy is a no-op state removal (never-forget). The write
        surface stays **HIL-unverified** (no live Access console — see `TODO.md`).

## Open questions (don't block design)

1. **Exact adoption/forget verbs** (`/cmd/devmgr` `adopt`/`delete-device`) + device **state enum**.
2. **CSRF source across firmwares** (header vs JWT claim); validate **`exp`-driven** re-auth (read the
   token's `exp`, don't hard-code 2h).
3. **Self-hosted/HIL runner setup** + documenting what the Docker tier can't cover.
4. **Which settings keys to type first** (likely `mgmt`, `usg`, `guest_access`, `ntp`, `country`, `ips`).
5. **Protect (P5):** exact PATCH field set per device type + which need a legacy `/bootstrap` fallback;
   whether detection **zones** are in initial scope; confirm doorbell `feature_flags` names.
6. **Clearing owned `omitempty` fields on GET-merge-PUT.** `structToRawMap` drops zero-valued owned
   fields, so the overlay can *set* but not *clear* a plain `string`/`*bool` value by emitting its zero
   (affects clearing `domain_name`, `dhcp_gateway`, etc.). The DNS-slot lists (`dhcp_dns`/`dns_servers`/
   `ipv6_dns_servers`) are already handled — those slots are `*string`, so a shrunk list writes a non-nil
   `&""` to overwrite the freed slot. Optional+Computed hides the remaining cases for the common path
   (removing from config keeps the controller value); a full clear of the other fields needs confirming
   on a live controller that empty-string writes are accepted per field before emitting them.
7. **WAN field mutability.** Whether `wan_type` (dhcp↔pppoe↔static) and `network_group` (WAN↔WAN2) are
    editable in place or need `RequiresReplace` — confirm against a live controller before adding plan
    modifiers (network `purpose` is known-immutable and already `RequiresReplace`).
