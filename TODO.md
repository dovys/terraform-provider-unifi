# TODO

Tracked improvements that aren't blocking a phase. Phase-scoped work lives in
[`docs/roadmap.md`](docs/roadmap.md); cross-cutting polish lives here.

## Spec: enrich `device.json` with read-only `/stat/device` fields

The rebuilt client's generated `wire.Device` is typed from the **writable**
`api/fields` set, so it omits read-only stat/identity fields the client needs —
notably `mac` (the natural key), plus `state`, `model`, `version`, `adopted`,
`ip`, etc. `network.Devices().GetByMAC` currently works around this by scanning
the raw `/stat/device` documents and decoding the matched one. Fix at the source:
add the observed stat fields to `objects/device.json` (as `server_fields`/
read-only) in `../unifi-specs`, regenerate, then drop the raw-scan workaround.
Other stat-heavy legacy objects (clients, etc.) may have the same gap.

## Verify against hardware

The live console allows **read-only** recon, so anything that needs a real write
to hardware — or hardware in a state we don't currently have — is modeled from
read-only GETs + documented behavior and must be confirmed once a hardware test
rig (HIL) exists.

**Clean-room caveat:** for the HIL-unverified resources below, every field
name/shape must be confirmed against the **actual controller API** (a live GET on
a populated collection, or official Ubiquiti documentation) — never assumed from
how a prior third-party provider modeled it. Where the live collection was empty,
the shapes are a best-effort from the documented API contract and are explicitly
provisional pending that clean-room verification.

Open items:

- **Port profile (`portconf`) wire shape.** The live site has **zero** custom
  port profiles, so `/rest/portconf` returned nothing to confirm against. The
  `unifi_network_port_profile` fields are modeled from the confirmed
  `port_overrides` field set (which shares `portconf`'s schema) plus documented
  enums. Verify the exact accepted values for `forward`
  (`all`/`native`/`customize`/`disabled`), `poe_mode` (`auto`/`pasv24`/
  `passthrough`/`off`), and `op`/dot1x once a profile can be created.
- **Device adoption / forget verbs.** All 5 live devices are already adopted
  (`adopted=true`), so the adopt and forget write paths in `unifi_network_device`
  are unverified — confirm the exact command (`/cmd/devmgr` `adopt` vs
  `force-provision`, and `delete-device` for forget) against a pending/adoptable
  device, including how long adoption takes and whether a config PUT can follow
  immediately.
- **Device config write (`PUT /rest/device/<id>`).** The device *read* shape
  (`/stat/device`: `name`, `led_override`(+color/brightness), `mgmt_network_id`,
  `port_overrides`, telemetry) is now recon-confirmed, but the *write* is not: the
  recon session is read-only, so verify that a partial PUT merges top-level fields
  server-side (we never send the whole document) and that `port_overrides` is
  replace-semantics (the resource rebuilds the full array via per-port merge).
- **Device `state` enum.** Only `state=1` (connected) is observable live; the
  other states (provisioning, upgrading, disconnected, heartbeat-missed, …) and
  their integer values need confirming before any state-dependent logic.
- **`port_overrides` `portconf_id` / `poe_mode`.** These two per-port keys are
  standard documented fields but were **not set on any live override** (the recon
  network binds no port profiles and uses default PoE), so their exact key names
  and accepted values are unconfirmed. The rest of the `port_overrides` field set
  and types are recon-confirmed.
- **RADIUS account (`/rest/account`) field set.** The live site has **zero**
  RADIUS users, so `unifi_network_radius_account` is modeled from documented
  controller behavior: `name`, `x_password`, and the dynamic-VLAN tunnel attrs
  (`tunnel_type`, `tunnel_medium_type`, `vlan`). Confirm the exact keys, whether
  `x_password` is echoed back on read (assumed write-only → preserved from
  config), and the accepted tunnel values once an account can be created.
- **RADIUS profile (`radiusprofile`) write-only fields.** The default profile
  confirmed `name`, `auth_servers[]{ip,port,x_secret}`, `acct_servers`,
  `use_usg_auth_server`, `attr_no_edit/attr_no_delete`. The added
  `use_usg_acct_server`, `interim_update_enabled`/`interim_update_interval`,
  `vlan_enabled`, and `vlan_wlan_mode` (`disabled`/`optional`/`required`) are
  documented but were not present on the default profile — confirm their exact
  key names/enums once a profile can be written, and whether `x_secret` is
  echoed (the round-trip currently assumes it is, unlike a write-only secret).

- **Static route (`/rest/routing`) field set.** The live site has **zero**
  static routes, so `unifi_network_static_route` is modeled from documented
  behavior: `type: "static-route"` with the hyphenated `static-route_network`/
  `_nexthop`/`_distance`/`_type`/`_interface` keys. Confirm the exact keys, the
  `static-route_type` enum (`nexthop-route`/`interface-route`/`blackhole`), the
  `interface` values, and the default `distance` once a route can be created.
- **Port forward (`/rest/portforward`) field set.** The live site has **zero**
  port-forward rules, so `unifi_network_port_forward` is modeled from documented
  behavior: `name`, `enabled`, `pfwd_interface` (`wan`/`wan2`/`both`), `proto`
  (`tcp_udp`/`tcp`/`udp`), `src`, `dst_port`, `fwd`, `fwd_port`, `log`. Confirm
  the keys/enums and whether `fwd_port` truly defaults to `dst_port` server-side
  once a rule can be created.

- **Dynamic DNS (`/rest/dynamicdns`) field set.** The live site has **zero**
  DDNS configs, so `unifi_network_dynamic_dns` is modeled from documented
  behavior: `service`, `host_name`, `server`, `login`, `x_password`,
  `interface`. Confirm the accepted `service` provider keys, whether
  `x_password` is echoed on read (assumed write-only → preserved from config),
  and the `interface` values once a config can be created.

- **Traffic rules/routes (v2 `trafficrules`/`trafficroutes`) field set.** The
  live site has **zero** traffic rules and routes (endpoints valid, empty), so
  `unifi_network_traffic_rule`/`unifi_network_traffic_route` are modeled from
  documented v2 behavior. Confirm: the `matching_target`/`action` enums; whether
  `domains` is a list of strings or of `{domain, ports, port_ranges}` objects (we
  use strings); the `target_devices` entry shape (`type`/`client_mac`/
  `network_id`); the `bandwidth_limit` keys; and whether a `schedule` object is
  required (deferred — not modeled yet). Also confirm `traffic_route.network_id`
  is the right field for the egress interface/VPN.
- **Zone-based firewall (v2 `firewall/zone` + `firewall-policies`) — deferred.**
  The live controller runs the **classic** firewall, so the zone endpoints return
  empty and the classic-vs-zone capability gate (the controller runs one model or
  the other, never both) can't be designed/verified. Build `firewall_zone` +
  `firewall_policy` once a zone-firewall-enabled controller is available, with a
  `ModifyPlan`/`ValidateConfig` gate that refuses to manage zone resources on a
  classic controller (and vice-versa for `firewall_rule`).
- **VPN server / VPN client (`vpn_server`/`vpn_client`) — deferred (P4).** Live
  recon found **no** configured VPN: no `networkconf` carries a VPN `purpose`, and
  the `openvpn`/`ipsec`/`teleport`/`magic_site_to_site_vpn` setting docs are
  near-empty global toggles (a DH key, an IKEv2 method, an `enabled` flag, a
  WireGuard keypair) — not the per-VPN field set. The full configurable surface
  (vpn type, listen port, served subnet/pool, RADIUS binding, WireGuard peers,
  OpenVPN options, IPsec proposals, site-to-site peer config) is **not
  discoverable** without a configured instance, and there's no gateway hardware to
  create one. Determine whether modern UniFi models these as `networkconf`
  vpn-server/vpn-client purposes or as the per-key settings above (or both), then
  model the confirmed field set, on a HIL rig with a real VPN configured. Defer
  until then (invariant 7: live network is the bar — nothing to import clean).
- **BGP (`bgp`) — deferred (P4).** No REST/v2 endpoint and no `usg` setting field
  exposes BGP on the live console — UniFi BGP is an FRR-config-file feature on
  specific gateways (UXG/UDM-SE/EFG), enabled per-gateway, not present here. Find
  the actual API (config-blob upload vs a typed doc) on a BGP-capable gateway and
  model it there; deferred until that hardware exists.

- **Protect camera / NVR config writes (`PATCH /proxy/protect/api/{cameras/<id>,nvr}`).**
  The Protect *read* shapes are recon-confirmed (the live console runs Protect
  7.1.69 with an adopted camera + the NVR singleton), and the resources import
  clean read-only. The *writes* are not verified — the recon session is
  read-only — so confirm on a HIL rig: (a) that a partial PATCH merges
  server-side and that sending an owned **sub-object** (`osdSettings`,
  `ledSettings`, `smartDetectSettings`, `recordingSettings`) replaces only that
  object's keys (the resource sends the full merged sub-object to avoid clearing
  siblings — confirm this is necessary/sufficient); (b) the doorbell-gated
  `lcdMessage` accept/reject on a non-doorbell camera (today an unsupported-field
  write surfaces as the controller's 4xx — a `ModifyPlan`-with-read per-camera
  capability gate is the eventual fix, see `docs/apps/protect.md` §capability);
  (c) the camera **unadopt** verb (`POST /cameras/<id>/unadopt`) used by
  `forget_on_destroy`; (d) the exact accepted `hdrType` values per model.
- **Protect capability gating not yet wired (invariant 3).** `docs/apps/protect.md`
  specifies two gates — a Configure-time Protect-version/installed probe, and a
  per-camera `ModifyPlan`-with-read capability gate (HDR/mic/LED/smart-detect/
  doorbell-LCD). Neither is wired in P5a: `protect.NVR().Version()` exists as the
  intended probe source but nothing calls it, and managing Protect on a console
  without it (or writing `lcd_message`/HDR/smart-detect to an incapable camera)
  surfaces as the controller's raw 4xx instead of a plan-time diagnostic. Wire the
  cheap Configure-time probe (set a `caps.Protect` bit, friendly diagnostic when
  absent) and the per-camera `ModifyPlan` gate in P5b.
- **Protect liveview (`/proxy/protect/api/liveviews`) — slots untyped.** The
  shipped Protect OpenAPI types `liveview.slots` as an untyped array (`items:
  {}`), so the slot object shape (cameras, cycle mode/interval) can't be modeled
  clean-room from the spec alone. Capture a real liveview (live GET on the cookie
  session, or a HIL create) to confirm the slot field names before building
  `unifi_protect_liveview` (P5b). The scalar fields (name, layout, isDefault,
  isGlobal) are typed and ready.

## Settings — model as typed resources (not a generic blob)

Site settings are ≈39 singleton `setting/<key>` documents (`ntp`, `mgmt`, `usg`,
`guest_access`, `global_switch`, `mdns`, `ips`, `country`, `locale`, …; the
`super_*` keys are console-internal and out of scope). A generic JSON-blob
`unifi_network_setting` was **rejected** (2026-06-02) as a smell: it abandons the
provider's typed-attributes + controller-mirroring-validators philosophy. Model
the meaningful, user-facing settings as **typed, import/update-only** resources
(they're singletons — no create/delete; destroy = state removal), each backed by
`GET-merge-PUT` on `/rest/setting/<key>/<id>` so unmodeled fields survive. Pick
the user-facing subset; confirm each field set against a live GET. This is a
dedicated effort (its own phase/PR set), not part of the P3 stack.

## Cross-resource wiring follow-ups

- **`unifi_network_wlan` → RADIUS profile.** A `wpaeap` SSID needs a
  `radiusprofile_id` to bind to a `unifi_network_radius_profile`; the WLAN
  resource exposes `radius_mac_auth_enabled`/`radius_das_enabled` but **not** the
  profile reference yet. Add `radiusprofile_id` to `unifi_network_wlan` (and the
  wired 802.1X port profile binding) so the RADIUS profiles are actually
  referenceable from config.

## Import UX: friendlier keys than the controller `_id`

Importing `unifi_network_network` / `unifi_network_wan` (and future
`networkconf`-backed resources) currently requires the controller object id
(`_id`, a 24-character hex string) in the `import {}` block or `terraform import`
command. The UniFi UI does **not** expose this id anywhere — the settings URL
stays at `/settings/internet` (or `/settings/networks`) with no id, and the side
panel doesn't show it — so users must discover it via the `network_ids`
data-source output or the legacy REST API. That is poor UX.

Improve it:
- Accept a human-friendly import key in `ImportState` and resolve it to the
  `_id` — e.g. `terraform import unifi_network_wan.primary WAN1` (by name), or a
  prefixed form like `name=WAN1`, while still accepting the raw `_id`.
- Consider exposing the lookup more directly (e.g. import by `name`) so the
  `import {}` block can reference a data source: `id = data.unifi_network_network.wan.id`.
- Apply the same friendly-key pattern to other resources as they land, and
  document it once in the import guidance.

The raw `_id` must keep working — it remains the resource's state `id`.

## Clear owned `omitempty` fields on GET-merge-PUT (beyond DNS slots)

`structToRawMap` drops zero-valued owned fields, so the overlay can set but not
clear a plain `string`/`*bool` back to its zero (e.g. clearing `domain_name`,
`dhcp_gateway`). The DNS-slot lists are already handled (`*string` slots write a
non-nil `&""`); a related edge remains: setting a list to an explicit empty `[]`
reads back as `null`, which fails the post-apply consistency check. Decide the
general approach (per-field empty-write allow-list, or always-emit owned keys)
once empty-string writes are confirmed acceptable per field on a live controller.
See `docs/roadmap.md` open question #6.

## Access (P7): verify the RE'd surface before registering the managed resource

The Access app is modeled from the **reverse-engineered** private web-UI API
(`../unifi-specs/access/v4.2.27`, derived from the controller Go binary — no
vendor OpenAPI for this surface). It **cannot be verified against a live console
this phase** (no Access console; X-API-KEY is Network-only). The substruct client
and `data.unifi_access_door_group` ship; the managed `unifi_access_door_group`
resource is written + unit-tested but **not registered** in `provider.go` until
the following are confirmed against a live/HIL Access console. Each is a one-line
re-enable once checked:

- **CRUD paths.** Confirm `GET /api/v2/access/door_groups` (list),
  `GET|PUT|DELETE /api/v2/access/door_group/{id}`, `POST /api/v2/access/door_group`
  (create), and `GET /api/v2/access/door_group/{id}/door_ids` exist as modeled
  (`internal/unifi/access/door_groups.go`).
- **`{code,msg,data}` envelope success set.** `envelope.ok()` currently treats
  `0 / 200 / "SUCCESS" / "success" / "" / null` as success (`access/core.go`).
  This is fail-open (an unmodeled/absent code reads as success). Pin it to the
  real success sentinel once a live capture shows it.
- **Not-found signalling.** Whether a missing object is an HTTP 404, a 400 with a
  not-found message, or a non-zero envelope `code` — replace the message-substring
  heuristic (`msgNotFound`) with the real code when known.
- **`/door_ids` response shape.** Modeled as a flat `[]string`; confirm it isn't
  an object wrapper or array of door objects. Also confirm an **empty** group
  returns `[]` (not `null`) so an explicit `door_ids = []` config round-trips
  without a perpetual diff (else normalize nil→empty in `readInto`).
- **`group_type` enum + `floor_ids`.** Confirm the `group_type` values and whether
  it's immutable-once-assigned (if so, `UseStateForUnknown`/`RequiresReplace` may
  be warranted). `floor_ids` is writable in `DoorGroupParams` but has **no read
  path** in the modeled surface, so floor groups are out of scope until a richer
  group-detail read view is found — the resource models door-only groups.
- **DoD on re-enable.** Acceptance create→update→import→destroy + `CheckDestroy` +
  `tfacc-` sweeper + a clean second-empty plan, per the standard.
