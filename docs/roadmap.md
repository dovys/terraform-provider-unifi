# Roadmap, checklist & open questions

Phasing is **vertical**: each phase ships finished resources (schema + backends + docs + unit +
acceptance) rather than half-built breadth. A phase is done when its resources import the live
network to a clean plan (where instances exist) and pass CI.

## Phases

- **0 — Bootstrap:** scaffold (`terraform-provider-scaffolding-framework`, protocol v6); `internal/unifi`
  skeleton (`Client`, `classic` + `integration` transports, `router`, `capabilities`); provider
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
- **4 — Publish:** `vpn_server`/`vpn_client`/`bgp`; optional standalone `unifi_network_device_port` (per-device
  mutex) if demanded; examples + `templates/`; Registry publish; flip public.
- **5 — Protect app (`unifi_protect_*`):** reuse the existing client + console key under
  `/proxy/protect/...`. `unifi_protect_camera` (adopt-if-needed + configure; cameras + doorbells,
  doorbell fields feature-flag-gated; read-only `data.unifi_protect_camera`), `unifi_protect_light`/`_sensor`/`_chime`/`_viewer`,
  `unifi_protect_liveview`, `unifi_protect_nvr_settings`. Device config via official PATCH (Protect 5.3+);
  zones/NVR/users via legacy `/proxy/protect/api/`. **HIL-only** (not in the Docker controller). See
  [apps/protect.md](apps/protect.md).

## Coverage checklist

A resource is checkable only when it meets the Definition of Done ([development.md](development.md)).

- [x] base: provider scaffold, dual-auth client (legacy/v2/Integration transports behind `Client`),
      capability-aware `router`, `Configure` capability probe, CI + release tooling — **P0**
- [x] network, wan, dns_record (v2); data sources network/networks, site, application_info — **P1**
      (`site` *resource* deferred: `/cmd/sitemgr` write shapes need a multi-site controller to verify)
- [ ] device (single adopt+configure resource w/ port map), port_profile, wlan, ap_group,
      firewall_rule, firewall_group, dhcp_reservation — **P2**
- [ ] user_group, radius_profile, radius_account, wlan_group, static_route, port_forward,
      dynamic_dns, firewall_zone, firewall_policy, traffic_rule, traffic_route, setting_* — **P3**
- [ ] vpn_server, vpn_client, bgp, device_port (optional) — **P4**
- [ ] protect_camera (single adopt+configure resource), protect_light, protect_sensor, protect_chime,
      protect_viewer, protect_liveview, protect_nvr_settings — **P5**

## Open questions (don't block design)

1. **OpenAPI for Integration v1?** If the console serves a spec, clean-room codegen that surface.
2. **Exact adoption/forget verbs** (`/cmd/devmgr` vs `/devices/adopt`) + device **state enum**.
3. **Integration write availability per firmware** — which version flips each write on.
4. **CSRF source across firmwares** (header vs JWT claim); validate **`exp`-driven** re-auth (read the
   token's `exp`, don't hard-code 2h).
5. **Self-hosted/HIL runner setup** + documenting what the Docker tier can't cover.
6. **Which settings keys to type first** (likely `mgmt`, `usg`, `guest_access`, `ntp`, `country`, `ips`).
7. **Protect (P5):** exact PATCH field set per device type + which need legacy `/bootstrap` fallback;
   whether detection **zones** are in initial scope; confirm doorbell `feature_flags` names; confirm
   which device-config writes (if any) the official Integration v1 covers vs legacy PATCH.
8. **DHCP reservation wire shape** — confirm the raw `user` field is `fixedip` (one word) and that a
   fixed-IP write requires an accompanying `network_id`, before freezing the model.
9. **Clearing owned `omitempty` fields on GET-merge-PUT.** `structToRawMap` drops zero-valued owned
   fields, so the overlay can *set* but not *clear* a plain `string`/`*bool` value by emitting its zero
   (affects clearing `domain_name`, `dhcp_gateway`, etc.). The DNS-slot lists (`dhcp_dns`/`dns_servers`/
   `ipv6_dns_servers`) are already handled — those slots are `*string`, so a shrunk list writes a non-nil
   `&""` to overwrite the freed slot. Optional+Computed hides the remaining cases for the common path
   (removing from config keeps the controller value); a full clear of the other fields needs confirming
   on a live controller that empty-string writes are accepted per field before emitting them.
10. **WAN field mutability.** Whether `wan_type` (dhcp↔pppoe↔static) and `network_group` (WAN↔WAN2) are
    editable in place or need `RequiresReplace` — confirm against a live controller before adding plan
    modifiers (network `purpose` is known-immutable and already `RequiresReplace`).
