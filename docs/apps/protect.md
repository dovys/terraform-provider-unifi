# UniFi Protect (the `unifi_protect_*` app)

Protect is a second UniFi OS app on the same console, so it reuses the **same client, host, and
classic console session** as Network — no new credentials. Resources are namespaced `unifi_protect_*`
(see [../README.md](../README.md) app-namespacing). This is **Phase 5** ([../roadmap.md](../roadmap.md)); it
ships after the Network app is published.

We model **infrastructure** (the cameras/sensors/NVR and their persistent config), not runtime media.

## API surfaces

Under `/proxy/protect/...`, over the same classic session:

| Surface | Base path | Auth | Role |
|---------|-----------|------|------|
| **Private API** | `/proxy/protect/api/...` (`/bootstrap`, `/cameras/<id>`, …) | cookie + CSRF (shared classic session) | The write surface: device-config **PATCH**, the full `/bootstrap` document, NVR settings, detection **zones**, users/permissions. Reverse-engineered. |
| **WebSocket** | `/proxy/protect/ws/updates` | cookie | Live event stream. **Not used by the provider** — Terraform manages config, not an event feed. |

The classic session (CSRF + ~2h JWT re-auth, local-admin requirement) is exactly the one in
[../architecture.md §3](../architecture.md); Protect adds no new auth. As with Network, the
Integration/X-API-KEY surface is out of scope.

## Resources

Same device shape as Network — a single adopt-and-configure resource (plus an optional read-only
data source) — because a camera is adopted hardware with mutable config (see
[../devices-and-ports.md](../devices-and-ports.md) for the rationale).

| Resource | Backend | Notes |
|----------|:------:|-------|
| `unifi_protect_camera` | adopt + **PATCH** (GET-merge-PATCH) | One resource per camera, keyed by MAC: adopts if pending, then manages config — `name`, `osd` (on-screen text/flags), `recording_settings` (mode/retention), `smart_detect` (person/vehicle/package), `mic_volume`, `video_mode`, `hdr`, `ir_led`; **doorbell fields gated by `feature_flags.is_doorbell`:** `lcd_message`, `chime_duration`, `ring_settings`. Computed telemetry exposed. Destroy reverts config; `forget_on_destroy` opts into forget. |
| `data.unifi_protect_camera` | private `/bootstrap` | Read-only lookup for an unmanaged camera: state, model, firmware, `feature_flags`, RTSP availability. |
| `unifi_protect_light` | PATCH | Floodlight: brightness, PIR sensitivity, light-on-motion schedule. |
| `unifi_protect_sensor` | PATCH | UP-Sense: motion/temperature/humidity/leak thresholds, mount type. |
| `unifi_protect_chime` | PATCH | Chime: volume, paired doorbell associations, ringtones. |
| `unifi_protect_viewer` | PATCH | Viewport device: the `unifi_protect_liveview` it displays. |
| `unifi_protect_liveview` | **CRUD** | Logical multi-camera layout (grid of camera slots). Pure config object — full create/read/update/delete. |
| `unifi_protect_nvr_settings` | singleton | NVR/console-level: recording retention defaults, time-lapse, smart-detect global toggles. One doc, always present → singleton lifecycle (like Network `setting_*`). |

**Write path.** Config writes are **PATCH** against the Protect private API (`/proxy/protect/api/...`)
over the shared classic session — GET-merge-PATCH on the owned fields, never a whole-document write.

**Doorbells are cameras.** There is no separate doorbell device type; a doorbell is a camera whose
`feature_flags.is_doorbell` is true. The doorbell-only attributes above are validated/sent **only**
when that flag is set (plan-time diagnostic otherwise) — same capability-gating pattern as Network
([../architecture.md §5](../architecture.md)).

**Detection zones** (per-camera motion/smart-detect regions) live in the `/bootstrap` camera doc. If
modeled, they're a nested attribute on `unifi_protect_camera` written via the GET-merge-PATCH path.
Deferred unless needed.

## Not modeled

Runtime media and ephemeral actions — not configuration:

- Live RTSP/RTSPS streams, snapshots, recorded-clip export, talkback audio.
- PTZ **movements** (a command, not state); PTZ **presets** could be config later.
- Event/detection feed (the WebSocket) — events aren't Terraform state.
- Per-user view permissions and cloud (UI.com remote) account linkage — later, legacy-only.

## Testing

**HIL-only (Tier 3).** The Docker `linuxserver/unifi-network-application` controller has **no Protect
app**, and there's no Protect simulator — so unlike Network there is no Tier-2 coverage. Acceptance
runs against real Protect hardware (a console + one camera, ideally one doorbell for the flag-gated
fields) on the self-hosted runner. Unit tests still cover model (de)serialization, the PATCH
merge-and-mask logic, and the doorbell feature-flag gating. The testing matrix
([../testing.md](../testing.md)) states this gap plainly — no claimed-but-absent coverage.
