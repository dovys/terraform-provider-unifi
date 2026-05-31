# Roadmap, checklist & open questions

Phasing is **vertical**: each phase ships finished resources (schema + backends + docs + unit +
acceptance) rather than half-built breadth. A phase is done when its resources import the live
network to a clean plan (where instances exist) and pass CI.

## Phases

- **0 — Bootstrap:** scaffold (`terraform-provider-scaffolding-framework`, protocol v6); `internal/unifi`
  skeleton (`Client`, `classic` + `integration` transports, `router`, `capabilities`); provider
  config + auth + `Configure` probe; CI (unit + Docker-controller harness + stub HIL job);
  `tfplugindocs`/`goreleaser`/manifest/`GNUmakefile`; **initial commit** (also unblocks web `/ultraplan`).
- **1 — Core networking (v0.1):** `unifi_network_network`, `unifi_network_wan`, `unifi_network_dns_record` (v2), `site`;
  data sources `network` + `networks`, `site`, `application_info`. Import live VLANs + WANs + DNS clean.
- **2 — Devices, ports, firewall, reservations:** `data.unifi_network_device` + `unifi_network_device_adoption` +
  `unifi_network_device_configuration` (port map), `unifi_network_port_profile`; `unifi_network_wlan`, `unifi_network_ap_group`;
  `unifi_network_firewall_rule` (+ uniqueness guard), `unifi_network_firewall_group`; `unifi_network_dhcp_reservation`.
  Import all 5 devices, 3 WLANs, 8 rules, 8 reservations.
- **3 — Parity & long tail:** `user_group`, `radius_profile`/`radius_account`, `wlan_group`;
  `static_route`, `port_forward`, `dynamic_dns`; zone firewall (`firewall_zone`/`firewall_policy`),
  `traffic_rule`/`traffic_route` (tested on HIL); `setting_*` + generic `unifi_network_setting`.
- **4 — Publish:** `vpn_server`/`vpn_client`/`bgp`; optional standalone `unifi_network_device_port` (per-device
  mutex) if demanded; examples + `templates/`; Registry publish; flip public.
- **5 — Protect app (`unifi_protect_*`):** reuse the existing client + console key under
  `/proxy/protect/...`. `data.unifi_protect_camera` + `unifi_protect_camera_adoption` + `unifi_protect_camera`
  (cameras + doorbells; doorbell fields feature-flag-gated), `unifi_protect_light`/`_sensor`/`_chime`/`_viewer`,
  `unifi_protect_liveview`, `unifi_protect_nvr_settings`. Device config via official PATCH (Protect 5.3+);
  zones/NVR/users via legacy `/proxy/protect/api/`. **HIL-only** (not in the Docker controller). See
  [protect.md](protect.md).

## Coverage checklist

A resource is checkable only when it meets the Definition of Done ([development.md](development.md)).

- [ ] network, wan, dns_record (v2), site — **P1**
- [ ] device (data + adoption + configuration w/ port map), port_profile, wlan, ap_group,
      firewall_rule, firewall_group, dhcp_reservation — **P2**
- [ ] user_group, radius_profile, radius_account, wlan_group, static_route, port_forward,
      dynamic_dns, firewall_zone, firewall_policy, traffic_rule, traffic_route, setting_* — **P3**
- [ ] vpn_server, vpn_client, bgp, device_port (optional) — **P4**
- [ ] protect_camera (data + adoption + configuration), protect_light, protect_sensor, protect_chime,
      protect_viewer, protect_liveview, protect_nvr_settings — **P5**

## Open questions (don't block design)

1. **OpenAPI for Integration v1?** If the console serves a spec, clean-room codegen that surface.
2. **Exact adoption/forget verbs** (`/cmd/devmgr` vs `/devices/adopt`) + device **state enum**.
3. **Integration write availability per firmware** — which version flips each write on.
4. **CSRF source across firmwares** (header vs JWT claim); validate the ~2h re-auth path.
5. **Self-hosted/HIL runner setup** + documenting what the Docker tier can't cover.
6. **Which settings keys to type first** (likely `mgmt`, `usg`, `guest_access`, `ntp`, `country`, `ips`).
7. **Protect (P5):** exact PATCH field set per device type + which need legacy `/bootstrap` fallback;
   whether detection **zones** are in initial scope; confirm doorbell `feature_flags` names.
