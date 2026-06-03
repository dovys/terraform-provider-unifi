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
local-admin user; `test/hil/`). Console address and credentials come from a **gitignored**
`test/hil/secrets.env` (copy `secrets.env.example`) — the maintainer's network topology and
password-manager layout never land in this repo; the `secret-hygiene` CI job enforces it
(`scripts/check-secrets.sh`):

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

### Write verification — ephemeral, additive, opt-in (planned, staged)

The read-only rungs can't reach two things: the **write paths** of resources whose live collection is
*empty* (nothing to import — `port_profile`, `radius_account`/`radius_profile`, `static_route`,
`port_forward`, `traffic_rule`/`traffic_route` are modeled from docs, never seen populated — the
biggest provisional risk in [`../TODO.md`](../TODO.md)), and **write semantics** (GET-merge-PUT,
partial-merge, replace-vs-merge). Verifying these needs a write — done *safely* against the operator's
**own active home console** (no separate test site) by making every test object **additive, inert, and
guaranteed-torn-down**. It is a **deliberate one-shot** from the workstation (`run.sh`, gated), **not** a
CI gate — the `tier3-hil` job stays a stub; an occasional leaked-and-swept object or momentary blip is
acceptable.

**Five rails.**

1. **Additive only — never mutate an existing object.** Create brand-new objects; never PUT/modify
   anything already on the console. Worst case of a failed teardown is one orphaned *new* object, never
   a corrupted existing one.
2. **Inert by construction, enforced as a runtime precondition.** Each test asserts its inertness
   invariant *before* `apply` and refuses otherwise (the tier table's last column) — "inert" is a
   literal guard a careless future edit can't slip past, not a convention.
3. **Unmistakable naming + sweeper.** Every object name carries a `tfacc-hil-` prefix (extends the
   Tier-2 `tfacc-` sweeper). A **pre-run sweep** clears leftovers from a crashed run; a **post-run
   sweep** is the net. A leaked object is non-fatal — logged, swept next run.
4. **Guaranteed teardown.** `apply` then *always* `destroy` in a LIFO `t.Cleanup` (runs on
   panic/failure too, reverse dependency order); the sweeper backstops anything destroy missed.
5. **Triple gate.** `UNIFI_HIL_ALLOW_WRITES=1` **and** an interactive `type 'I understand'` confirm
   (which names the AP-reprovision blip) **and** a **host allowlist** (refuse unless `UNIFI_HOST` is the
   known home console) — a mistyped prod host can't be written to. A dry-run `plan` prints the additive
   diff before any `apply`.

Per-object lifecycle (full DoD against live firmware): pre-sweep → `apply` (verify create + wire decode
+ field round-trip) → `plan` clean (idempotency) → mutate one field + `apply` (update path) → fresh
`import` + `ImportStateVerify` → `t.Cleanup` `destroy` + `CheckDestroy` → post-sweep. The tiers below
land **one per PR** (W0 → W1 → W2); every object additionally asserts the `tfacc-hil-` name prefix.

| Tier | Touches (on the live network) | Resources | Precondition asserted before `apply` |
|------|-------------------------------|-----------|--------------------------------------|
| **W0** | control-plane data only — no dataplane/RF/hardware effect even if a teardown leaks | `firewall_group`, `user_group`, `radius_profile` (+`x_secret`), `radius_account` (+`x_password`), `dns_record`, `protect_liveview`, `dhcp_reservation` | created **unreferenced**; DNS a fake name; reservation a MAC absent from active clients; liveview touches no camera |
| **W1** | installed into the **gateway** config (provision/reload) but inert | `static_route`, `port_forward`, `traffic_rule`, `traffic_route`, `firewall_rule`, `port_profile` | `enabled=false`; route dests ∈ RFC-5737/RFC-3849 blackhole (`192.0.2.0/24`, …, `2001:db8::/32`); `port_profile` **not** referenced by any device `port_override` |
| **W2** | **AP/switch provisioning** — a brief mgmt-plane blip (no client drop expected); run during low usage | `wlan_group`, `ap_group` | member list **empty** |
| **W3** | mutates existing/shared state or has an outward effect — **rig only, never the home console** | `device` adopt/forget/PUT/`port_overrides`; real `network`; enabled `wlan`; `network_settings`; `protect_nvr_settings`/`camera`/`light`/`sensor`; `dynamic_dns` | — |

W3 stays in [`../TODO.md`](../TODO.md) *Verify against hardware* until a throwaway controller exists.

## Honesty rule

The matrix states plainly that **Tier 2 does not cover real radios, hardware adoption, or several v2
firmware quirks** — those are Tier 3 only. No claimed-but-absent coverage.

## "Tested" for a resource

Unit tests (schema/validators/serialization); a Tier-2 test doing **create → update → import
(`ImportStateVerify`) → destroy** with a clean intermediate plan **and a second consecutive empty
plan**; `CheckDestroy`; a sweeper; a Tier-3 HIL test if it needs radios/hardware (else noted as
sim-covered). Full Definition of Done in [development.md](development.md).
