# TODO

Tracked improvements that aren't blocking a phase. Phase-scoped work lives in
[`docs/roadmap.md`](docs/roadmap.md); cross-cutting polish lives here.

## Verify against hardware

The live console allows **read-only** recon, so anything that needs a real write
to hardware — or hardware in a state we don't currently have — is modeled from
read-only GETs + documented behavior and must be confirmed once a hardware test
rig (HIL) exists. Open items:

- **Port profile (`portconf`) wire shape.** The live site has **zero** custom
  port profiles, so `/rest/portconf` returned nothing to confirm against. The
  `unifi_network_port_profile` fields are modeled from the confirmed
  `port_overrides` field set (which shares `portconf`'s schema) plus documented
  enums. Verify the exact accepted values for `forward`
  (`all`/`native`/`customize`/`disabled`), `poe_mode` (`auto`/`pasv24`/
  `passthrough`/`off`), and `op`/dot1x once a profile can be created.
- **Device adoption / forget verbs.** All 5 live devices are already adopted
  (`adopted=true`), so the adopt and forget write paths are unverified — confirm
  the exact command endpoint (`/cmd/devmgr` `adopt`/`force-provision` vs
  `/api/.../devices/...`) and the forget verb against a pending/adoptable device.
- **Device `state` enum.** Only `state=1` (connected) is observable live; the
  other states (provisioning, upgrading, disconnected, heartbeat-missed, …) and
  their integer values need confirming before any state-dependent logic.
- **`port_overrides` write effects.** PoE mode changes, port-profile binding
  (`portconf_id`), and port enable/disable can be modeled from the read shape but
  their write behavior (and the device-doc GET-merge-PUT round-trip) can't be
  validated without a managed switch under test.

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
See `docs/roadmap.md` open question #9.

## Decide whether to keep the Integration (official) API at all

The provider currently does dual auth: an Integration `X-API-KEY` session and a
local-admin `username`+`password` (cookie+CSRF) session, with the router
preferring the official Integration API where firmware supports a write and
falling back to legacy/v2 otherwise (invariant #2). In practice the Integration
API is **very limited** — most meaningful writes (networkconf, wlanconf, port
profiles, firewall, …) only exist on the legacy/v2 surfaces, so a local admin
account is effectively required for anything beyond reads. The API key buys us
little today: a bit of read coverage (`/sites`, `application-info`) and local
DNS routing that still falls back to v2.

Decide whether the Integration path earns its complexity:
- **Keep it** — the one-line-routing-change promise (invariant #2) is real and
  cheap to retain; new official writes land with zero resource edits as Ubiquiti
  expands the Integration API. The cost is a second transport + the capability
  gating to maintain.
- **Drop it** — make `username`+`password` the sole auth, delete the
  `integration` transport / `X-API-KEY` config / router backend-flip machinery,
  and simplify the docs and capability model. Re-add later if/when the official
  API covers enough writes to matter.

If we keep it, document plainly that the API key is optional and read-only-ish
today (so users don't expect it to unlock writes). Revisit once we know how much
of Phase 2/3 the Integration API can actually serve.
