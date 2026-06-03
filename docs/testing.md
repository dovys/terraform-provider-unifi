# Testing & sandbox strategy

Never test against production. Three tiers — because **UniFi OS Server can't run in GitHub-hosted
runners** (needs systemd, `SYS_ADMIN`, host cgroups; runners are unprivileged), but the plain Docker
controller covers the legacy + v2 surfaces where ~all writes live.

| Tier | Runs | Controller | Auth | Covers |
|------|------|:----------:|------|--------|
| **Unit** | every PR | none | — | capability logic, model (de)serialization (`httptest`), validators, plan modifiers, the **v2 write contract**. Most bugs live here. |
| **Acceptance (legacy/v2)** | merge / nightly | `linuxserver/unifi-network-application` (Docker) | classic | networks, WAN, firewall (classic), routing, port profiles, devices + port overrides, reservations, user groups, RADIUS, DNS. The bulk. |
| **HIL** | gated `workflow_dispatch` / self-hosted | UniFi OS Server (VM) or real gear | classic | Real radios (WLAN broadcasting), device adoption/forget, zone-firewall enforcement, firmware quirks the Docker controller can't reproduce. |

## Tier 2 (workhorse)

Boot the Docker controller (+ MongoDB) as a CI service, pin the version, poll for readiness
(generous timeout + retry). Bootstrap a local admin headlessly →
`UNIFI_HOST/USERNAME/PASSWORD/SITE/INSECURE`. Enable **simulation mode** to fabricate hardware
without devices:
```
is_simulation=true
demo.num_ugw=1   demo.num_usw=2   demo.num_uap=0
```
Run `TF_ACC=1 go test ./... -timeout 60m`. Dispose per run; **sweepers** clean leaked objects via a
mandatory `tfacc-` name prefix; `CheckDestroy` per resource. Version matrix over a current build +
the firmware floor.

## Tier 3 (gated)

A self-hosted runner with UniFi OS Server on a systemd VM, or real hardware (disposable gateway +
AP). The **only** place that exercises real radios, device adoption/forget, zone-firewall
enforcement, and firmware-specific behavior the Docker controller can't reproduce. It's also
where the operator's **own live console** — captured as a local, gitignored fixture, never
committed — is imported end-to-end to a clean (and second consecutive empty) plan: the truest
regression.

### Read-only rungs (run today, against the operator's live console)

Two read-only rungs run from the workstation against the live console (cookie auth via a dedicated
local-admin user; `test/hil/`):

- **Rung A — read smoke** (`internal/provider/hil_read_test.go`, build tag `hil`): logs in and
  `List()`s every Network collection + the Protect surfaces (cameras/nvr/liveviews/…), forcing each
  element through its generated `wire` type. A field the clean-room spec got wrong shows up as a
  decode failure against real firmware. (This is how the user `last_seen`/`blocked`, device
  `stp_priority`/`radio_table.channel`, and Protect NVR `systemInfo` telemetry mistypes were found.)
- **Rung B — import → clean-plan** (`internal/provider/hil_import_test.go` + `test/hil/run.sh`):
  discovers live `_id`s, emits `import {}` blocks, and runs `tofu plan -generate-config-out`,
  asserting *N to import, 0 to add/change/destroy*. A non-empty plan is a HIL finding (a field Read
  doesn't round-trip), not a harness failure.

Both are **read-only by construction** — GETs plus the one session-login POST; there is no
apply/destroy codepath. They cover only objects that **exist** on the console (invariant 7), and only
those that round-trip without a write: resources with write-only secrets (`radius_profile`,
PPPoE-WAN passwords) or a singleton-with-à-la-carte-blocks import (`network_settings`) need a
hand-written config, and `device` is excluded (runtime-volatile `ip`/`state` telemetry can't be
clean in a plan-only flow). See [`test/hil/README.md`](../test/hil/README.md).

### Write verification — ephemeral, additive, opt-in (planned)

The read-only rungs can't reach two things: the **write paths** of resources whose live collection
is *empty* (so there's nothing to import — `port_profile`, `radius_account`/`radius_profile`,
`static_route`, `port_forward`, `traffic_rule`/`traffic_route` are modeled from docs, never seen
populated — the biggest provisional risk in [`../TODO.md`](../TODO.md)), and **write semantics**
(GET-merge-PUT, partial-merge, replace-vs-merge). Verifying these needs a write — which we can do
*safely* against the operator's own home console by making every test object **additive, inert, and
guaranteed-torn-down**:

1. **Additive only — never mutate an existing object.** Create brand-new objects; never PUT/modify
   anything already on the console. The worst case of a failed teardown is one orphaned *new* object,
   never a corrupted existing one.
2. **Inert by construction — zero effect on live traffic/RF/clients while it exists.** Traffic rule /
   port forward / firewall rule → `enabled = false`; static route → a blackhole to a **RFC 5737**
   documentation prefix (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) that nothing routes to;
   port profile / firewall group / RADIUS account/profile / user group → created **unreferenced** (not
   bound to any port/rule/WLAN), so they change no behavior.
3. **Unmistakable naming + sweeper.** Every object's name carries a `tfacc-hil-` prefix (extends the
   Tier-2 `tfacc-` sweeper). A **pre-run sweep** removes leftovers from a crashed run; a **post-run
   sweep** is the final safety net.
4. **Guaranteed teardown.** Drive with `terraform apply` then *always* `terraform destroy` in a
   `t.Cleanup` (runs on panic/failure too); the sweeper backstops anything destroy missed.
5. **Explicit, separate opt-in.** A distinct gate (`UNIFI_HIL_ALLOW_WRITES=1`) plus an interactive
   confirmation in the runner — separate from the read rungs, which never write. Prefer a dedicated
   **test site** on the console when one exists, so writes are isolated from the default site.

Per-object lifecycle (the full Definition of Done, against live firmware): pre-sweep → `apply` (verify
create + wire decode + field round-trip) → `plan` clean (idempotency) → mutate one field + `apply`
(update path) → fresh `import` + `ImportStateVerify` → `t.Cleanup` `destroy` + `CheckDestroy` →
post-sweep.

**In scope** (safe to create-and-drop on the home console): `traffic_rule`, `traffic_route`,
`port_forward` (all `enabled=false`); `static_route` (RFC-5737 blackhole); `firewall_group`,
`port_profile`, `radius_profile` (+ its write-only `x_secret` round-trip), `radius_account` (+
`x_password`), `user_group`, `wlan_group`, `ap_group` (unreferenced/additive).

**Out of scope for the home console — needs a disposable rig** (these mutate existing/shared state or
have an outward effect): `device` adopt/forget + config PUT + `port_overrides`; `network_settings`
writes + GET-merge-PUT empty-field clearing; `dynamic_dns` (registers with an external provider);
Protect camera/nvr/light/sensor writes (mutate a real device — do with a read-original/restore
pattern on a rig); an `enabled` `wlan` (broadcasts a real SSID) and a real `network` (live VLAN/DHCP
scope). These stay in [`../TODO.md`](../TODO.md) *Verify against hardware* until a throwaway
controller exists.

## Honesty rule

The matrix states plainly that **Tier 2 does not cover real radios, hardware adoption, or several v2
firmware quirks** — those are Tier 3 only. No claimed-but-absent coverage.

## "Tested" for a resource

Unit tests (schema/validators/serialization); a Tier-2 test doing **create → update → import
(`ImportStateVerify`) → destroy** with a clean intermediate plan **and a second consecutive empty
plan**; `CheckDestroy`; a sweeper; a Tier-3 HIL test if it needs radios/hardware (else noted as
sim-covered). Full Definition of Done in [development.md](development.md).
