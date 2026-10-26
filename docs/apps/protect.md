# UniFi Protect

Protect is the video app on a UniFi OS console, next to Network. Its resources are named `unifi_protect_*` ([design decisions](../README.md#design-decisions)). The provider models Protect infrastructure: cameras, lights, sensors, chimes, viewers, NVR settings, live view layouts, and automation webhook headers. It never models media or events.

## Surface

Resources call the private REST API that the Protect web UI uses, under `/proxy/protect/api/`. It runs over the same cookie and CSRF session as Network, so the provider's `username` and `password` cover Protect with no extra credential (see [architecture.md](../architecture.md)). The provider does not use the Protect Integration API (`/proxy/protect/integration/v1`, `X-API-KEY`) and has no `api_key` setting.

Responses are bare JSON with no `{meta, data}` envelope. The client maps a 404, or a 400 whose message says the object was not found, to `ErrNotFound`, and Read then removes the resource from state.

Shapes come from the OpenAPI document Protect ships and from observing the live API: browser devtools on the Protect web UI, the provider's `TF_LOG=DEBUG` wire logs, and read-only GETs. The provider is clean-room: it models only this observed contract, never the vendor's implementation (see [reverse-engineering.md](../reverse-engineering.md)).

## Client

`c.Protect()` returns the client in `internal/unifi/protect`. It groups collections by object type:

| Accessor | Path | Extras |
|---|---|---|
| `Cameras()` | `/cameras` | `GetByMAC`, `Forget` (POST `/cameras/<id>/unadopt`) |
| `Lights()`, `Sensors()`, `Chimes()`, `Viewers()` | `/lights`, `/sensors`, `/chimes`, `/viewers` | `GetByMAC`; Delete unadopts |
| `Liveviews()` | `/liveviews` | `GetByName` |
| `NVR()` | `/nvr` | Singleton get and update |
| `Automations()` | `/automations` | Batch PATCH |

Most groups wrap the generic `Collection[T]`: List, Get, Create, Update, and Delete, where Update is a PATCH.

Types in `internal/unifi/protect/wire` are generated with oapi-codegen (types only) from a version-pinned API spec that isn't public. The spec is projected from the OpenAPI Protect ships, trimmed to an explicit list of root objects. `make generate` needs `UNIFI_SPECS` pointing at the spec. A new resource's type needs its root added to the list in `wire/gen.go`, or regen drops it. Never hand-edit the generated file.

## Write contract

Updates PATCH only the fields the provider owns, never the whole document. The controller merges a PATCH at the top level but replaces nested sub-objects wholesale.

To add a writable resource:

1. Use the generated `*Update` write DTO if the object has one (`omitempty`-tagged, e.g. `wire.NvrUpdate`). Otherwise define an overlay struct of the owned fields with `omitempty`. The generated read types have no `omitempty`, so sending them would write zeros over every unowned field.
2. Build the overlay from the plan over the current object with the `merge*` helpers in `protect_common.go` (`mergeStr`, `mergeBool`, `mergeStrSet`, `mergeFloat` for an Int64 attribute onto a float wire field, `mergeF64` for a Float64 attribute). A null or unknown plan value keeps the current value.
3. For a nested sub-object, read the current object, change the owned keys, and send the whole sub-object back.

Because an omitted field keeps its value, a field can't be cleared by sending its zero value. Network's sparse PUT has the same limit ([roadmap.md](../roadmap.md#open-questions)).

Automations use a batch PATCH. The client sends `PATCH /automations` with a one-element `[{id, body}]` array and reads a 207 response with a status per item. The controller replaces `actions[]` wholesale, so the client echoes every action back and changes only the owned header.

## Resources

All device resources configure objects that are already adopted. Create looks up the device by `mac` and fails if no adopted device has that MAC. The provider can't adopt devices.

| Resource | Path | Configures |
|---|---|---|
| `unifi_protect_camera` | `/cameras/<id>` | `name`, `mic_enabled`, `mic_volume`, `status_light_enabled`, `hdr_type`, `video_mode`, `recording_mode`, `osd_*_enabled`, `smart_detect_object_types`, `smart_detect_audio_types`, `lcd_message_type`, `lcd_message_text`. Computed: `model`, `type`, `state`, `host`, `is_adopted`, `has_speaker`. |
| `unifi_protect_light` | `/lights/<id>` | `name`, `camera`, `mode` (off, motion, always, schedule), `enable_at`, `led_level`, `indicator_enabled`, `pir_duration`, `pir_sensitivity`, `lux_sensitivity`, `force_led_on`. |
| `unifi_protect_sensor` | `/sensors/<id>` | `name`, `mount_type`, `camera`, `motion_enabled`, `motion_sensitivity`, `{temperature,humidity,light}_{enabled,low,high,margin}`, `leak_internal_enabled`, `leak_external_enabled`, `alarm_enabled`, `led_enabled`. Thresholds are floats in °C, % relative humidity, or lux. |
| `unifi_protect_chime` | `/chimes/<id>` | `name`, `volume`, `repeat_times`, `camera_ids`. Ringtones (`ringSettings`) are not modeled. |
| `unifi_protect_viewer` | `/viewers/<id>` | `name`, `liveview`. Computed: `stream_limit`. |
| `unifi_protect_liveview` | `/liveviews/<id>` | `name`, `layout`, `is_default`, `is_global`, and a `slots` list of `cameras`, `cycle_mode`, `cycle_interval`. Full create, update, and delete. |
| `unifi_protect_nvr_settings` | `/nvr` | `name`, `timezone`, `time_format`, `temperature_unit`, `ssh_enabled`, `enable_automatic_backups`, `auto_retention_enabled`, `recording_retention_duration_ms`, `disable_audio`. Computed: `version`, `host`. |
| `unifi_protect_automation` | `/automations` | One webhook header (`header_name`, write-only `header_value`) on the sole `HTTP_REQUEST` action of an automation created in the Protect app. |

`data.unifi_protect_camera` looks up a camera by `id` or `mac` and returns its attributes. It is the only Protect data source.

The example is [examples/protect/protect.tf](../../examples/protect/protect.tf). Reference docs are generated under `docs/resources/protect_*.md` and `docs/data-sources/protect_camera.md`.

## Lifecycle and import

Destroy on a device resource removes it from state and leaves the device with its last-applied config. With `forget_on_destroy = true`, destroy also unadopts it: a camera through `POST /cameras/<id>/unadopt`, other devices through `DELETE` on the device.

Neither `nvr_settings` nor `automation` can be created on the controller. Create configures the existing NVR, and an automation needs `id` set in config or an `import {}` block. Destroy removes them from state. The NVR always exists, so its Read never returns not-found. Automations have no per-id delete, and `DELETE /automations` removes all of them.

| Resource | Import key |
|---|---|
| Devices | MAC with colons, or the 24-hex object id |
| `liveview` | 24-hex id, or the name |
| `nvr_settings` | Any value; import reads `/nvr` |
| `automation` | `<id>/<header_name>`. A bare id works, but `header_name` is unset until the first apply. |

A MAC or name import lists the collection and filters it. The state `id` is always the Protect object id.

The API returns MACs without colons. The provider stores them in canonical colon form, and a `normalizeMAC` plan modifier runs before `RequiresReplace` so a format change doesn't force replacement. Import treats a key as a MAC only if it contains a colon, so a dash or bare-hex MAC is read as an id.

## Gating

Two gates turn unsupported config into a diagnostic instead of a raw 4xx.

At Configure, the provider probes `/proxy/protect/api/nvr`. If the probe fails, `requireProtect` reports "UniFi Protect is not available" on any Protect resource. The gate checks only that Protect is installed. It records the version but enforces no minimum. The tested version is 7.1.69.

At plan time, `unifi_protect_camera` checks config against the camera's feature flags in `ModifyPlan`. The flags live on the controller, so the check reads the camera, which `ValidateConfig` can't do. Doorbells are cameras, so the LCD fields use the same check.

| Attribute | Flag |
|---|---|
| `hdr_type` | `HasHdr` |
| `mic_enabled`, `mic_volume` | `HasMic` |
| `status_light_enabled` | `HasLedStatus` |
| `smart_detect_object_types`, `smart_detect_audio_types` | `HasSmartDetect` |
| `lcd_message_type`, `lcd_message_text` | `HasLcdScreen` |

The check reads config, not plan, so values carried from state by `UseStateForUnknown` don't trigger it. It reads the camera only when a gated attribute is set, and skips the check if the camera can't be read.

## Not modeled

- RTSP and RTSPS streams, snapshots, clip export, talkback.
- PTZ movement and presets.
- The event and detection WebSocket feed.
- Per-user view permissions and cloud account linkage.
- Detection zones.

## Testing

The Docker Network controller used for Tier 2 has no Protect, and no Protect simulator exists, so Protect has no Tier 2 acceptance tests. Writes can only be verified on hardware (Tier 3). See [testing.md](../testing.md).

Client unit tests inject a fake session (`fakeDoer`) and cover paths, the owned-field overlay, not-found mapping, and the automation batch envelope (`internal/unifi/protect/protect_ci_test.go`). Provider unit tests cover wire conversion, merge helpers, capability violations, and schemas (`internal/provider/protect_*_ci_test.go`). The client tests cover the Protect presence probe.

Verified on a live console: every Protect surface decodes through its wire type, cameras, liveviews and NVR settings import to a clean plan, and a liveview with no slots round-trips create, update and destroy. Unverified writes are tracked in [TODO.md](../../TODO.md).
