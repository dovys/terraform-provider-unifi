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

- **a real AP MAC + a write-tier run** — `unifi_network_client_ap_lock` (new,
  issue #82) is the one Network write not yet HIL-confirmed: verify the sparse
  `fixed_ap_*` PUT pins the client and leaves a coexisting reservation's
  `fixed_ip` intact, that re-pinning `ap_mac` takes effect, and that destroy
  (`fixed_ap_enabled=false`) clears the lock without forgetting the client;
- **a disposable gateway/switch rig** — device adopt/forget + config PUT, the
  `device.state` enum, and `unifi_network_settings` write semantics, and dynamic
  DNS. (The `port_overrides` write→read-back contract — which fields the controller
  stores verbatim vs derives — is verified; see `RestoreConfiguredScalars`.)
- **a CyberSecure-licensed gateway** for `unifi_network_threat_management` — the
  enabled (non-`disabled`) write path is unverified. Confirmed live (UCG Max, no
  active subscription): the controller lists the `IPS` feature and accepts a
  `set/setting/ips` write with HTTP 200 but silently keeps `ips_mode="disabled"`
  and forces `enabled_categories` to `[]`; even the gateway-less Tier-2 controller
  behaves the same. So the resource gates on the `IPS` capability (plan) + verifies
  the mode took (apply). Still to verify on a licensed console: that `ips_mode`
  (`ids`/`ips`/`ipsInline`) + `enabled_categories` actually round-trip, that
  setting categories with `mode="disabled"` is accepted (it is NOT on an unlicensed
  console), and that the per-network `honeypot` array survives the shallow merge.
  NB the live `ips` doc also carries server-managed fields the spec omits
  (`utm_token`, `ad_blocking_enabled`, `dns_filtering`, `endpoint_scanning`) — left
  unmodeled per the spec's writable set; reconfirm none are practitioner-writable;
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

Shipped keys: `ntp`, `country`, `locale`. The `ips` key ships separately as the
dedicated `unifi_network_threat_management` resource (IDS/IPS is a large enough
surface — mode, category set, the engine flags — to warrant its own typed
resource rather than a `network_settings` block). Remaining (high-value first):
`mgmt`, `guest_access`, `global_switch`, `global_ap`, `mdns`, `radius`. Long-tail
/ niche keys (`snmp`, `netflow`, `radio_ai`, `rsyslogd`, `dpi`, `teleport`,
`baresip`, `lcm`, `usg`, `doh`, …) are added on demand; the `super_*` (console-
internal) and `porta` (hardware-only) keys are out of scope. Each new key is one
`settingKeyHandler` table entry + its schema block; secrets (`x_*`) get
`Sensitive`, and confirm the key's live GET field set + shallow-merge semantics
as it's added.
