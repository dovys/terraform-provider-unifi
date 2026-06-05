# TODO

Tracked improvements that aren't blocking a phase. Phase-scoped work lives in
[`docs/roadmap.md`](docs/roadmap.md); cross-cutting polish lives here.

## Verify against hardware

The additive **write-verification** tier ([`docs/testing.md`](docs/testing.md)
§"Write verification") has been run against the live console — W0 (control-plane
data), W1 (gateway-inert), W2 (AP/switch) — so every Network resource reachable
by a safe create-and-drop is now **HIL-confirmed** end-to-end (create → update →
import → second-empty-plan → destroy, clean): firewall groups, user groups,
RADIUS profiles/accounts (the `x_password` write-only round-trip included), DNS
records, DHCP reservations, static routes, port forwards, traffic rules/routes,
firewall rules, port profiles, WLAN/AP groups, and a Protect liveview.

What's left genuinely needs hardware this network doesn't have, so it is **not
tracked field-by-field here** — the resource schemas carry an `HIL-unverified`
note and the deferred phases live in [`docs/roadmap.md`](docs/roadmap.md). Each
gap unlocks only with the matching hardware:

- **a disposable gateway/switch rig** — device adopt/forget + config PUT, the
  `device.state` enum, `port_overrides` `poe_mode`/`portconf_id` on a bound port,
  `unifi_network_settings` write semantics, and dynamic DNS;
- **a zone-firewall controller** — `firewall_zone` / `firewall_policy` (the recon
  console runs the mutually-exclusive classic firewall);
- **a VPN-configured gateway** — `vpn_server` / `vpn_client` and BGP;
- **Protect write access / a live Access console** — the Protect camera/NVR +
  light/sensor/chime/viewer PATCH paths (and wiring the per-camera capability
  gate), and the RE'd `access_*` write surface (door groups, policies, holidays,
  doors, guards).

## Settings — grow `unifi_network_settings` key coverage

`unifi_network_settings` ships: one resource whose sections are managed à la
carte, an **optional nested block per `setting/<key>`** (a null block is
unmanaged — Read/apply touch only declared blocks, so managing one key never
pulls another into state). It rides the dedicated settings verbs (`GET
get/setting/<key>`, `POST set/setting/<key>`) — *not* `/rest` — and writes
shallow-merge the configured fields over the stored section so unmodeled fields
survive. The generic JSON-blob `unifi_network_setting` was **rejected** as a
smell (it abandons typed attributes + controller-mirroring validators); the
typed-block model keeps both.

Shipped keys: `ntp`, `country`, `locale`. Remaining (high-value first): `mgmt`,
`guest_access`, `global_switch`, `global_ap`, `mdns`, `ips`, `radius`. Long-tail
/ niche keys (`snmp`, `netflow`, `radio_ai`, `rsyslogd`, `dpi`, `teleport`,
`baresip`, `lcm`, `usg`, `doh`, …) are added on demand; the `super_*` (console-
internal) and `porta` (hardware-only) keys are out of scope. Each new key is one
`settingKeyHandler` table entry + its schema block; secrets (`x_*`) get
`Sensitive`, and confirm the key's live GET field set + shallow-merge semantics
as it's added.
