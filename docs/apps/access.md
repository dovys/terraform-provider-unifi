# UniFi Access (the `unifi_access_*` app)

Access is another UniFi OS app on the same console. Resources are namespaced `unifi_access_*`
(see [../README.md](../README.md) app-namespacing). We model **infrastructure** — door groups, doors,
access policies, holidays/schedules — never per-swipe events or runtime activity.

## Surface: the private web-UI API over the one cookie session

Access uses the **private web-UI API the Access app itself calls** — `/proxy/access/api/v1/...` and
`/proxy/access/api/v2/...` — over the **same cookie+CSRF session** as the Network and Protect apps. **No
new credential**: `username`/`password` reach everything, so the "**one console session serves every
app**" principle holds ([../README.md](../README.md), [architecture.md §3](../architecture.md)). This is
the reverse-engineered surface, treated clean-room — modeled from the **observed contract**, never from
reading the vendor's implementation (see [../reverse-engineering.md](../reverse-engineering.md)).

Every Access response is wrapped in a `{code, msg, data}` **envelope**: `code` is the status, `msg` a
human string, `data` the typed payload. The client unwraps it centrally in `internal/unifi/access`
(`do[T]`) and hands groups the bare typed object — callers never see the envelope. A not-found maps to
`access.ErrNotFound` (→ remove-on-404).

**Why not the official Integration API** (`X-API-KEY`)? Access ships **only** an Integration-API
OpenAPI in its package — that's the supported, documented *subset*, and it's the **wrong surface** for
infrastructure modeling (it can't reach the full door-group/policy/door config). Per the
**either-or-per-resource** rule a resource commits to **one** surface and never mixes; the Access
resources commit to the private web-UI API. The Integration API stays a reference cross-check only;
invariant 2 ("one cookie+CSRF session; Integration/X-API-KEY out of scope") **stands** — no `api_key`.

**Lower spec fidelity than Protect, and not live-verifiable yet.** Protect ships a full vendor OpenAPI;
Access does not, so the private-API spec (`../unifi-specs/access/v4.2.27`) is reverse-engineered from the
Access controller **Go binary** (typed structs recovered via redress). The provider's `wire` types are a
deterministic projection of that spec (`access_codegen_spec.py`, reusing the Protect
closure-trim/hoist/widen transforms). Because there is **no live/HIL Access console** this phase
(X-API-KEY is Network-only), the write surface — CRUD paths, the envelope success-code set, the
`/door_ids` shape — is **unconfirmed**; see [`../../TODO.md`](../../TODO.md).

> **Status: P7a shipped — foundation + read-only data source.** The `c.Access()` substruct client is built
> on the cookie session (`internal/unifi/access`, private web-UI API `/proxy/access/api`, `{code,msg,data}`
> envelope, console-global, no `api_key`); generated `wire` types are projected from the RE'd spec.
> `data.unifi_access_door_group` ships (look up by `id`/`name`). The **managed** `unifi_access_door_group`
> resource is written + unit-tested but **intentionally not registered** until a live/HIL console confirms
> its write surface (invariant 7; the repo's zone-firewall / VPN-BGP deferral precedent) — re-enabling is a
> one-line change in `provider.go`. Doors, policies, and holidays follow once the read surface is confirmed
> (their `wire` roots are already projected).

## Resources

A door group is a logical object (not adopted hardware), so it's a plain CRUD resource keyed by the
controller `unique_id`. Members live behind a dedicated `/door_ids` endpoint, so `Read` reconstructs state
from the group object (name, group_type) **plus** that endpoint. Floor groups (member floors) are out of
scope until a read path for floor membership is found — the resource models **door-only** groups.

| Resource | Backs | Notes |
|----------|------|-------|
| `unifi_access_door_group` *(written, not yet registered — gated on live verification)* | `POST/GET/PUT/DELETE /api/v2/access/door_group[/{id}]`, members via `/door_ids` | Keyed by `unique_id`. Manages `name`, `group_type` (controller-assigned), and door membership (`door_ids`). |
| `data.unifi_access_door_group` | `GET /api/v2/access/door_groups` (+ `/door_ids`) | Read-only lookup by `id`/`name`: name, group_type, type, member/floor/policy counts, member `door_ids`. |
