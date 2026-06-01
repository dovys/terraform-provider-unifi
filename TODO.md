# TODO

Tracked improvements that aren't blocking a phase. Phase-scoped work lives in
[`docs/roadmap.md`](docs/roadmap.md); cross-cutting polish lives here.

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
