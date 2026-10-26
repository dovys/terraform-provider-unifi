# UniFi Access

UniFi Access is the door-access app on a UniFi OS console. The provider manages its infrastructure objects (door groups, policies, holiday groups, doors and door guards) as `unifi_access_*` resources. It does not model users, credentials, NFC cards or door events.

None of the Access resources has run against a live console. They are built from the API spec and covered by unit tests only; see [Verification status](#verification-status).

## Surface

Access resources call the private API the Access web UI uses, under `/proxy/access/api/v2/...`. Requests go over the same cookie and CSRF session as Network and Protect, so the provider's `username` and `password` are the only credentials (see [architecture.md](../architecture.md)).

UniFi also publishes an Integration API (`X-API-KEY`) OpenAPI document for Access. The provider doesn't call it, because it can't reach the full door group, policy and door settings. It serves only as a reference to cross-check field names and shapes.

Access is console-global, not site-scoped, so the provider's `site` setting doesn't apply.

Access has no capability probe and no field on `unifi.Capabilities`. This is a known gap against capability gating (invariant 3), tracked in [`TODO.md`](../../TODO.md#access). On a console without Access installed, apply and data-source reads fail with a raw API error instead of a plan-time diagnostic, and a 404 on refresh drops existing resources from state.

## Response envelope

Every Access response wraps its payload in `{code, msg, data}`. `do[T]` in `internal/unifi/access/core.go` unwraps it, so service methods return the bare typed object.

- Success: `code` is `""`, `0`, `200`, `"SUCCESS"` or `null`. The spec types `code` as an integer; string codes such as `"SUCCESS"` are accepted too, pending a live capture (see [`TODO.md`](../../TODO.md#access)).
- An empty body or a `null` `data` counts as success with nothing to decode.
- Not found: an HTTP 404, an HTTP 400 whose message reads as not-found, or a non-success envelope whose `msg` reads as not-found. All three return `access.ErrNotFound`, and `Read` removes the resource from state.
- Any other non-success code returns an error carrying `msg` and `code`.

## Client layout

`unifi.Client.Access()` returns the `access.Client`. Each group lives in its own file in `internal/unifi/access`:

| Accessor | File |
|----------|------|
| `c.Access().DoorGroups()` | `door_groups.go` |
| `c.Access().AccessPolicies()` | `access_policies.go` |
| `c.Access().HolidayGroups()` | `holiday_groups.go` |
| `c.Access().Doors()` | `doors.go` |
| `c.Access().DoorGuards()` | `door_guards.go` |

The `wire` types in `internal/unifi/access/wire` are generated from a version-pinned API spec (Access 4.2.27) that isn't public. `make generate` needs `UNIFI_SPECS` pointing at it. The generated code is checked in; don't hand-edit it. If a field is missing or mistyped, open an issue.

## Resources

- Logical objects (door groups, policies, holiday groups) have full CRUD. Terraform owns their lifecycle, and they import by the controller's `unique_id`.
- Discovered hardware (doors and their guard sets) has no create or delete API. These resources adopt an existing door by `door_id`, configure it, and import by `door_id`. Changing `door_id` forces replacement. Destroy only removes the resource from state; the door keeps its last-applied config.

Every path below is relative to `/proxy/access/api/v2/`.

| Resource | API paths | Manages | Read-only |
|----------|-----------|---------|-----------|
| `unifi_access_door_group` | `POST/GET/PUT/DELETE access/door_group[/{id}]`, list `access/door_groups`, members `GET access/door_group/{id}/door_ids` | `name`, `group_type` (optional; the controller assigns one if unset), `door_ids` | |
| `unifi_access_policy` | `POST/GET/PUT/DELETE access_policy[/{id}]`, list `access_policies` | `name`, `enabled`, `schedule_id`, `resources` (typed door and door group references) | `description`, `creator_name`, `user_count`, `door_count`, `group_count`, `elevator_count`, `valid_start_time`, `valid_end_time` |
| `unifi_access_holiday_group` | `POST/GET/PUT/DELETE holiday_group[/{id}]`, list `holiday_groups` | `name`, `description`, `is_private`, `template_name`, `holiday_ids` | `create_time`, `holiday_count`, `is_default` |
| `unifi_access_door` | list `GET locations/doors`, update `PUT locations/door/{id}` | `name`, `floor_name`, `extra_type` | `full_name`, `location_type`, `timezone`, `is_open`, `dps_connected` |
| `unifi_access_door_guard` | `GET/PUT locations/door/{id}/door_guard` | `user_ids` (set `[]` to clear) | |

Per-resource notes:

- Door group: membership is written in the create and update body but read back from `access/door_group/{id}/door_ids`. Groups hold doors only; floor groups are out of scope until there is a way to read floor membership.
- Door: there is no per-door GET, so Read lists all doors and filters by `location.unique_id`.
- Door guard: `user_ids` are opaque references to Access users, which the provider doesn't model.
- Write-only fields: `schedule_id` and `resources` on a policy and `floor_name` on a door are sent on write but absent from the read view. Read keeps them from prior state, so they show no diff, but a refresh can't detect drift on them.

## Data sources

Each resource has a read-only data source. Paths are relative to `/proxy/access/api/v2/`.

| Data source | Reads | Lookup | Returns |
|-------------|-------|--------|---------|
| `data.unifi_access_door_group` | `access/door_groups`, `access/door_group/{id}/door_ids` | `id` or `name` | `name`, `group_type`, `type`, `door_count`, `floor_count`, `policy_count`, `door_ids` |
| `data.unifi_access_policy` | `access_policies` | `id` or `name` | `name`, `enabled`, `description`, `creator_name`, `user_count`, `door_count`, `group_count`, `elevator_count`, `valid_start_time`, `valid_end_time` |
| `data.unifi_access_holiday_group` | `holiday_groups` | `id` or `name` | `name`, `description`, `is_private`, `is_default`, `template_name`, `holiday_count`, `create_time`, `holiday_ids` |
| `data.unifi_access_door` | `locations/doors` | `id` or `name` | `name`, `full_name`, `floor_id`, `location_type`, `extra_type`, `timezone`, `is_open`, `dps_connected` |
| `data.unifi_access_door_guard` | `locations/door/{id}/door_guard` | `door_id` | `user_ids` |

The policy data source doesn't return `schedule_id` or `resources`, and the door data source returns `floor_id`, not `floor_name`.

The generated reference pages are in [`docs/resources`](../resources/) and [`docs/data-sources`](../data-sources/) (`access_*.md`). A worked example is in [`examples/access/access.tf`](../../examples/access/access.tf).

## Verification status

Tests cover the client paths and envelope handling (`internal/unifi/access/*_ci_test.go`) and the provider resources (`internal/provider/access_*_ci_test.go`). Nothing has confirmed the write surface on a live console: the CRUD paths, the envelope success codes, the `door_ids` response shape, and the preserved write-only fields. The open checks are listed under Access in [`TODO.md`](../../TODO.md).

If you have an Access console, capture real traffic and report differences from the tables above in an issue. Log the wire requests with `TF_LOG_PROVIDER=DEBUG terraform apply 2>&1 | grep 'wire HTTP'` (see [Wire logs](../development.md#wire-logs)), watch the Access web UI in browser devtools, or issue read-only GETs.

## Adding an Access resource

Follow the resource recipe in [development.md](../development.md). For Access:

1. Add the path constants and a service type in `internal/unifi/access`, calling `do[T]` so the envelope and not-found handling stay central.
2. Add the group accessor on `access.Client` in `client.go`.
