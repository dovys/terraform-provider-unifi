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

> **Status: P7 complete (P7a–P7c shipped).** The `c.Access()` substruct client is built on the cookie
> session (`internal/unifi/access`, private web-UI API `/proxy/access/api`, `{code,msg,data}` envelope,
> console-global, no `api_key`); generated `wire` types are projected from the RE'd spec. Ships the
> managed resources `unifi_access_door_group`, `unifi_access_policy`, `unifi_access_holiday_group`,
> `unifi_access_door`, and `unifi_access_door_guard`, each with a read-only data source. Every Access
> write surface — CRUD paths, the envelope success-code set, the `/door_ids` shape — is
> **HIL-unverified** (no live Access console; X-API-KEY is Network-only — see
> [`../../TODO.md`](../../TODO.md)).

## Resources

Two shapes appear. **Logical objects** (door groups, policies, holiday groups) are plain CRUD resources
keyed by the controller `unique_id` — Terraform owns their lifecycle. **Discovered hardware** (doors, and
the per-door guard set) has no create/delete API, so those resources are **update/import-only**: they adopt
an existing door by `door_id`, configure it, and on destroy do a **no-op state removal** — the door keeps
its last-applied config (the safe-by-default, never-forget posture; `RequiresReplace` on `door_id`).

Where a resource's write shape and its read shape diverge (the policy and door write DTOs carry fields the
read view omits, e.g. `schedule_id`/`resources` on a policy, `floor_name` on a door), `Read` reconciles the
fields the read view returns and **preserves** the write-only fields from the prior model — no phantom diff.

| Resource | Backs | Notes |
|----------|------|-------|
| `unifi_access_door_group` | `POST/GET/PUT/DELETE /api/v2/access/door_group[/{id}]`, members via `/door_ids` | Logical object keyed by `unique_id`. Manages `name`, `group_type` (controller-assigned), door membership (`door_ids`). |
| `unifi_access_policy` | `POST/GET/PUT/DELETE /api/v2/access_policy[/{id}]`, list `/access_policies` | Logical object. Manages `name`, `enabled`, `schedule_id`, and `resources` (typed door/door-group references); reads back computed subject/resource counts. |
| `unifi_access_holiday_group` | `POST/GET/PUT/DELETE /api/v2/holiday_group[/{id}]` | Logical object. Manages `name`, `description`, `is_private`, `template_name`, and member `holiday_ids` (existing holiday `unique_id`s). |
| `unifi_access_door` | `GET/PUT /api/v2/locations/door[/{id}]`, list `/locations/doors` | Discovered hardware — **update/import-only**. Manages `name`, `floor_name` (write-time association), `extra_type`; exposes live `is_open`/`dps_connected`. Destroy = no-op. |
| `unifi_access_door_guard` | `GET/PUT /api/v2/locations/door/{id}/door_guard` | Discovered hardware — **update/import-only**. Manages the guard `user_ids` of a door (opaque user references; users not modeled). Set `user_ids = []` to clear; destroy = no-op. |
| `data.unifi_access_door_group` | `GET /api/v2/access/door_groups` (+ `/door_ids`) | Read-only lookup by `id`/`name`: name, group_type, type, member/floor/policy counts, member `door_ids`. |
| `data.unifi_access_policy` | `GET /api/v2/access_policies` | Read-only lookup by `id`/`name`: name, enabled, creator, subject/resource counts, validity window. |
| `data.unifi_access_holiday_group` | `GET /api/v2/holiday_groups` | Read-only lookup by `id`/`name`: name, description, member `holiday_ids`, counts. |
| `data.unifi_access_door` | `GET /api/v2/locations/doors` | Read-only lookup by `id`/`name`: full name, floor, location type, live `is_open`/`dps_connected`. |
| `data.unifi_access_door_guard` | `GET /api/v2/locations/door/{id}/door_guard` | Read-only lookup by `door_id`: the guard `user_ids`. |

Floor groups (member floors) remain out of scope until a read path for floor membership is found — door
groups model **door-only** membership. The provider models **infrastructure**, so Access users, credentials,
and NFC cards are out of scope; `user_ids` on a policy/door guard are opaque references the operator supplies.
Every write surface above is **HIL-unverified** — see [`../../TODO.md`](../../TODO.md).
