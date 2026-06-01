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
