# UniFi Protect (the `unifi_protect_*` app)

Protect is a second UniFi OS app on the same console. Resources are namespaced `unifi_protect_*`
(see [../README.md](../README.md) app-namespacing). It ships as **0.2.0**, after the Network app's
`0.1.0`. We model **infrastructure** (the cameras/lights/sensors/chimes/viewers, the NVR config, and
liveview layouts) — never runtime media or events.

## Surface: the private REST API over the one cookie session

Protect uses the **private REST API the web UI itself calls** — `/proxy/protect/api/...` (bootstrap,
`/cameras/<id>`, `/lights/<id>`, …) — over the **same cookie+CSRF session** as the Network app. **No new
credential**: `username`/`password` reach everything, so the "**one console session serves every app**"
principle holds ([../README.md](../README.md), [architecture.md §3](../architecture.md)). This is the
reverse-engineered surface, treated clean-room — modeled from the **observed contract**, never from
reading the vendor's implementation (see [../reverse-engineering.md](../reverse-engineering.md)). It's
the same posture the Network app already takes toward legacy REST.

**Why not the official Integration API** (`/proxy/protect/integration/v1`, `X-API-KEY`)? It's the
supported, documented *subset*, but a subset — it can't reach the full device/config surface, and
feature-completeness is the goal. Per the **either-or-per-resource** rule a resource commits to **one**
surface and never mixes; the Protect resources commit to the private REST API. The Integration API
remains a *permitted* option for some future resource, but is **currently unused**, so invariant 2
("one cookie+CSRF session; Integration/X-API-KEY out of scope") **stands unchanged** — no `api_key`.

**Shapes are captured by reverse-engineering, not yet modeled.** The exact `/proxy/protect/api` request/
response structures, write verbs, enums, and behavior come from the RE workstream
([../reverse-engineering.md](../reverse-engineering.md)) — SSH source-discovery confirmed by live
observation. The resource set and intent below are stable; the **field-level schemas land once RE
captures them** (this doc is updated then, with the per-resource field tables).

> **Status: P5a + P5b shipped.** The `c.Protect()` substruct client is built on the cookie session
> (`internal/unifi/protect`, private REST `/proxy/protect/api`, generated `wire` types projected from the
> shipped Protect OpenAPI). Resources: `unifi_protect_camera` (+ `data.unifi_protect_camera`) and
> `unifi_protect_nvr_settings` (P5a); `unifi_protect_light`, `unifi_protect_sensor`,
> `unifi_protect_chime`, `unifi_protect_viewer` (configure-existing, keyed by MAC), and
> `unifi_protect_liveview` (full CRUD; `slots` as a nested block) (P5b);
> `unifi_protect_automation` (manage a webhook header on an existing automation; write-only token).
> **No `api_key`, one session.**
> **Config writes are HIL-unverified** (recon is read-only — see [`../../TODO.md`](../../TODO.md)); the
> liveview `slots` shape and chime `ringSettings` (deferred) await a live/HIL capture.

## Resources

A camera is adopted hardware with mutable config, so — like `unifi_network_device` — it's a single
**configure-existing (+ adopt-if-needed, HIL-gated)** resource plus a read-only data source.
Lights/sensors/chimes/viewers are configure-existing resources; liveview is a pure CRUD config object;
nvr_settings is a singleton. All over `/proxy/protect/api/...`; field schemas per RE.

| Resource | Backs | Notes (fields confirmed via RE) |
|----------|------|-------|
| `unifi_protect_camera` | `/proxy/protect/api/cameras/<id>` | Keyed by `mac`. Configures name, status-LED, on-screen-display overlays, mic enable/volume, video mode, HDR, smart-detect object/audio types, and **doorbell-gated** LCD message. Computed telemetry: state, model, capability flags. Destroy reverts managed config; **never forgets** unless `forget_on_destroy=true`. |
| `data.unifi_protect_camera` | `/proxy/protect/api/cameras` | Read-only lookup by `mac`/`id`: state, model, capability flags. |
| `unifi_protect_liveview` | `/proxy/protect/api/liveviews` | Multi-camera layout: layout + slots (cameras, cycle mode/interval), default/global. Full **CRUD** config object. Import by `id` or `name`. |
| `unifi_protect_nvr_settings` | `/proxy/protect/api/nvr` | **Singleton** — import/update-only, no create, **destroy is state removal** (the shipped `attr_no_delete` no-op-destroy pattern, e.g. `user_group_resource.go`). Always present, so **Read never 404s**. Recording/retention defaults, arm mode, doorbell default messages. |
| `unifi_protect_light` | `/proxy/protect/api/lights/<id>` | Floodlight: brightness, PIR sensitivity, light-on-motion. **No live instance → HIL.** |
| `unifi_protect_sensor` | `/proxy/protect/api/sensors/<id>` | UP-Sense: motion/temp/humidity/leak thresholds, mount type. **HIL.** |
| `unifi_protect_chime` | `/proxy/protect/api/chimes/<id>` | Chime: volume, paired-doorbell associations, ringtones. **HIL.** |
| `unifi_protect_viewer` | `/proxy/protect/api/viewers/<id>` | Viewport: the liveview it displays. **HIL.** |
| `unifi_protect_automation` | `/proxy/protect/api/automations` | Manages one webhook header on an existing automation's sole `HTTP_REQUEST` action. Import/update-only (created in-app); `destroy` is state-removal (no per-id delete — `DELETE` wipes all automations); `header_value` write-only. **HIL.** |

**Data sources.** Only `data.unifi_protect_camera` ships initially (the object most likely referenced
without being managed). Add `liveview`/`nvr` data sources later if asked.

**Write path.** PATCH of the **owned fields only** (the exact verb per RE), never a
whole-document write — the Protect analogue of the Network sparse PUT (the controller partial-merges
at the top level; nested sub-objects it replaces wholesale are merged client-side). Read reconciles
Computed-vs-configured and removes-on-404 for the device resources; the singleton `nvr_settings` never
404s. The known `structToRawMap` set-but-not-clear limitation ([roadmap.md open question 6](../roadmap.md))
applies here too; same mitigations (Optional+Computed for the common path; an explicit-empty allow-list
per confirmed-clearable field).

## Capability & version gating

Two gates (invariant 3 — diagnose, don't emit a raw 4xx):

1. **Protect version — Configure-time.** The capability probe reads the Protect application version
   from the private API (the bootstrap/NVR doc, over the cookie session — endpoint per RE; live console
   runs **7.1.69**). Config-only, so it gates like the existing Network version check. Floor confirmed
   on 7.1.69, provisional below (no older build to test).
2. **Per-camera capability flags — apply-time / `ModifyPlan`-with-read.** A camera advertises its
   capabilities (HDR / mic / status-LED / speaker / supported smart-detect + video modes). Refusing a
   write to an unsupported field needs the camera's **live flags**, which the provider only has *after a
   read* — so this gate **can't be pure `ValidateConfig`**; it runs in `ModifyPlan` (which reads the
   camera) or as an apply-time diagnostic. **Doorbells are cameras** — no separate type; the doorbell
   fields (LCD message, chime) are gated by the same capability check. (Both are **new patterns** for
   this codebase — today gating lives only in `Configure`.)

## Safety, IDs, lifecycle

- **Safe by default** (invariant 4). `terraform destroy` on a camera **reverts the managed config** and
  does **not** unadopt the (physical, expensive) camera unless `forget_on_destroy=true` — same posture
  as `unifi_network_device`. Lights/sensors/chimes/viewers follow suit. `nvr_settings` and a device's
  adoption are never "deleted" by destroy.
- **Adoption.** Whether the private API exposes an adopt verb is **HIL-unverified** (the live camera is
  already adopted); until confirmed, `protect_camera` is **configure-existing + import**, adopt-if-needed
  HIL-gated (mirrors `unifi_network_device`).
- **Import keys.** Devices import by **`mac`** (friendly) or the 24-hex `id`; `liveview` by `id`/`name`;
  `nvr_settings` is the singleton NVR `id`. A `mac`/`name` import **resolves by list-and-filter** (GET
  the collection, match, take its `id`) — the same friendliness `unifi_network_device` provides over the
  id-keyed `/stat/device`. State `id` is always the Protect object id.

## Not modeled

Runtime media and ephemeral actions are not configuration:

- Live RTSP/RTSPS streams, snapshots, recorded-clip export, talkback audio.
- PTZ **movements** (a command, not state); PTZ **presets** may become config later.
- The event/detection WebSocket feed — events aren't Terraform state.
- Per-user view permissions and cloud account linkage — later, only if cleanly modelable.
- Detection **zones** (per-camera regions) — deferred until RE shows a clean shape.

## Testing

**HIL-only for writes (Tier 3).** The Docker `unifi-network-application` controller has **no Protect**
and there's no Protect simulator, so there is no Tier-2 acceptance for Protect (see
[../testing.md](../testing.md)). Coverage:

- **Unit** (no hardware): client CRUD over `httptest` incl. the owned-field overlay mask, model (de)serialization,
  schema validation, the capability/version gating, and conversion round-trips — against **redacted RE
  fixtures**.
- **Live read** (this console, read-only): `camera`, `liveview`, and `nvr_settings` **import clean** once
  their RE shapes are modeled (instances exist live).
- **HIL** (real hardware): all **writes**, adoption/forget, and the four zero-instance resources
  (`light`/`sensor`/`chime`/`viewer`).
