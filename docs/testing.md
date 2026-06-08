# Testing & sandbox strategy

Never test against production. Three tiers, each a script at the repo root — because **UniFi OS Server
can't run in GitHub-hosted runners** (needs systemd, `SYS_ADMIN`, host cgroups; runners are
unprivileged), but a plain Docker controller covers the legacy + v2 surfaces where ~all writes live.

| Tier | Script | Runs | Controller | Covers |
|------|--------|------|:----------:|--------|
| **1 — unit** | `./test-ci.sh` | every PR (CI) | none | capability logic, model (de)serialization (`httptest`), validators, plan modifiers, the **v2 write contract**. Fast, no behaviour reproduction. |
| **2 — acceptance** | `./test-extended.sh` | **locally, before every PR** | Docker (`linuxserver/unifi-network-application` + Mongo + nginx bridge) | concrete create → update → import → destroy lifecycles for the legacy/v2 resources. The behaviour bar. |
| **3 — HIL** | `./test-hil.sh` | sparingly, by hand | the operator's live console / real gear | real radios, device adoption/forget, zone-firewall enforcement, write paths of empty live collections, firmware quirks Docker can't reproduce. |

CI runs **Tier 1 only** (`./test-ci.sh`); Tier 2 is expensive and stateful, so it runs on the
workstation. **Run `./test-extended.sh` before opening any PR and paste its printed summary into the PR
description** — that summary is the evidence the behaviour suite passed.

## Layout — packages and tags, not name prefixes

The tier a test belongs to is encoded by its **package + build tag + file suffix**, never by a
`-run` name-prefix filter (a prefix is a fragile selector — one mistyped `TestAcc…` and a suite silently
drops out):

| Tier | Package | Build tag | Files |
|------|---------|-----------|-------|
| 1 — unit | the package under test (`internal/provider`, `internal/unifi/…`) | none | `<feature>_ci_test.go` |
| 2 — acceptance | `test/acc` (`package acc`) | `//go:build acc` | `<feature>_acc_test.go` |
| 3 — HIL | `test/hil` (`package hil`) | `//go:build hil` | `<feature>_hil_test.go` |

Unit tests are **white-box** — they reach into unexported internals (serialization, validators), so Go
binds them to their package directory; they stay beside the code. Acceptance and HIL tests are
**black-box** — they drive the provider through its public Terraform protocol, so they live in their own
packages and share one harness, **`internal/acctest`** (the in-process provider factory, env precheck,
env-built client, and the sweep/`CheckDestroy` plumbing). `go test ./...` (no tag) compiles only the
unit tests; `-tags acc`/`-tags hil` select a tier with no `-run` filtering.

## Tier 2 (the behaviour bar) — `./test-extended.sh`

The stack is a Docker Compose file (`test/tier2/compose.yaml`): Mongo + the standalone
`unifi-network-application` controller + a small **nginx bridge**. The provider only speaks the
**UniFi-OS console** dialect (`/api/auth/login`, `/proxy/network/...`, CSRF), but the only
CI-runnable controller is the **standalone** Network Application (`/api/login`, bare `/api/...` +
`/v2/...`, no CSRF). The bridge (`unifi-os-bridge.conf`) translates between them — rewrites the login
path, strips the `/proxy/network` prefix, and drops the `Secure` cookie flag so the session rides plain
HTTP. The provider connects to the bridge on `:8443`; nothing else changes. Mongo is seeded
(`init-mongo.js`) with the `unifi`/`unifi_stat` users the controller's connection string requires.

**Lifecycle lives in Go, not bash.** `TestMain` (`test/acc/`, gated on `UNIFI_T2_MANAGE=1`)
boots the stack if it's down, polls for readiness (controller `/status` up **and** the default site in
Mongo, 12-min deadline), seeds a local admin **directly into Mongo** (the standalone image has no
headless add-admin verb — insert the admin + privilege docs with a pre-hashed `x_shadow`), verifies a
login, and exports `UNIFI_HOST/USERNAME/PASSWORD/SITE/INSECURE`. The bash script is a thin trigger
(`go test -tags acc ./test/acc/`):

```
./test-extended.sh                  boot (if needed) + run every acceptance test, print a summary
./test-extended.sh -run TestAccFoo  forward extra `go test` flags (one suite)
./test-extended.sh up               boot + seed the stack, run no tests
./test-extended.sh down             tear the stack down (-v)
./test-extended.sh --fresh [...]    tear down, then boot fresh + run
```

Suites run serial (`-p 1`) against the single controller. **Sweepers** clean leaked objects via a
mandatory `tfacc-` name prefix; each resource has a `CheckDestroy`. Don't recreate the controller
container after seeding — its `/config` re-inits and wipes the admin; `--fresh` does a clean
`down -v` + reboot instead.

### Tier-2 coverage

Each resource below runs the full lifecycle — **create → update → import (`ImportStateVerify`) →
destroy**, with a clean intermediate plan and a **second consecutive empty plan**, plus `CheckDestroy`
and a sweeper:

`network`, `dns_record`, `firewall_group`, `firewall_rule`, `static_route`, `port_forward`,
`port_profile`, `user_group`, `radius_profile`, `radius_account`, `traffic_rule`, `ap_group`, `wlan`,
`dhcp_reservation`, `dynamic_dns`, `network_settings`, `wan`.

Three carry a documented deviation:

- **`wan`** — create → import → destroy only (no update step): a WAN-update PUT currently returns
  `api.err.InvalidPayload` for any field. That's a separate known provider bug with its own fix in
  flight; the test stops short of it rather than asserting broken behaviour.
- **`traffic_route`** — `t.Skip`ped: it needs a VPN/WAN egress network the Docker controller can't
  provide (the controller rejects every reachable target as an invalid type/target combination). HIL
  covers it.
- **`wan_sla`** — `t.Skip`ped: a real provider bug. The monitor `type` is sent lowercase (`icmp`) but
  the v2 wan-slas API only accepts the uppercase enum (`ICMP`/`DNS`/`SPEED_LIMIT`) — the clean-room
  spec guessed the casing. The skip carries the finding; un-skip once it's fixed.

## Tier 3 (gated) — `./test-hil.sh`

`./test-hil.sh read` runs the read-only rungs (import → clean-plan); `./test-hil.sh write` runs the
additive write verification (triple-gated). Both wrap the rungs in `test/hil/` and take config +
credentials from a gitignored `test/hil/secrets.env`.

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

- **Rung A — read smoke** (`test/hil/read_hil_test.go`, build tag `hil`): logs in and
  `List()`s every Network collection + the Protect surfaces (cameras/nvr/liveviews/…), forcing each
  element through its generated `wire` type. A field the clean-room spec got wrong shows up as a
  decode failure against real firmware. (This is how the user `last_seen`/`blocked`, device
  `stp_priority`/`radio_table.channel`, and Protect NVR `systemInfo` telemetry mistypes were found.)
- **Rung B — import → clean-plan** (`test/hil/import_hil_test.go` + `test/hil/run.sh`):
  discovers live `_id`s, emits `import {}` blocks, and runs `tofu plan -generate-config-out`,
  asserting *N to import, 0 to add/change/destroy*. A non-empty plan is a HIL finding (a field Read
  doesn't round-trip), not a harness failure.

Both are **read-only by construction** — GETs plus the one session-login POST; there is no
apply/destroy codepath. They cover only objects that **exist** on the console (invariant 7), and only
those that round-trip without a write: resources with write-only secrets (`radius_profile`,
PPPoE-WAN passwords) or a singleton-with-à-la-carte-blocks import (`network_settings`) need a
hand-written config, and `device` is excluded (runtime-volatile `ip`/`state` telemetry can't be
clean in a plan-only flow). See [`test/hil/README.md`](../test/hil/README.md).

### Write verification — ephemeral, additive, opt-in (W0–W2 run; W3 deferred)

The read-only rungs can't reach two things: the **write paths** of resources whose live collection is
*empty* (nothing to import — `port_profile`, `radius_account`/`radius_profile`, `static_route`,
`port_forward`, `traffic_rule`/`traffic_route` were modeled from docs, never seen populated), and
**write semantics** (sparse PUT, server-side partial-merge, replace-vs-merge). Verifying these needs a
write — done *safely* against the operator's **own active home console** (no separate test site) by
making every test object **additive, inert, and guaranteed-torn-down**. **W0–W2 have been run green**,
so those empty-collection wire shapes are now HIL-confirmed end-to-end (the run also caught and fixed one
provider bug — `traffic_rule` echoed empty match criteria back as empty sets instead of null). It is a
**deliberate one-shot** from the workstation (`write.sh`, gated), **not** a CI gate — the `tier3-hil` job
stays a stub; an occasional leaked-and-swept object or momentary blip is acceptable.

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
**have been run** (W0 → W1 → W2); every object additionally asserts the `tfacc-hil-` name prefix.

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
plan**; `CheckDestroy`; a sweeper; a Tier-3 HIL test if it needs radios/hardware (else the Docker
lifecycle is the bar, with any hardware gap noted). Full Definition of Done in
[development.md](development.md).

## Test what the user sees, not how the code works

Every test targets the **resource interface** — create / update / delete a resource (or, for the
serialization unit layer, the config→wire→state round-trip and the validators) and asserts the
**user-visible outcome**: the apply succeeds, the field survives, the plan is empty, the controller
error surfaces as a clear diagnostic. **Never assert internal mechanics** — call counts, "issues a GET
then a PUT", GET-merge-PUT vs sparse-PUT, which private method ran. Those pin a test to one
implementation: a legitimate refactor breaks them even though behaviour is unchanged, and they verify
*how* rather than *whether it works*.

Two consequences for bug fixes (also in [`CONTRIBUTING.md`](../CONTRIBUTING.md)):

- **A fix may only ADD a test, never REWRITE one.** If a fix forces you to change an existing test's
  assertions, that test was coupled to the buggy implementation — replace it via a *new* added test and
  keep the suite green, rather than editing the old one to match the new code.
- **Each fix adds a specific regression test** that fails on the old behaviour and passes on the new,
  written against the interface (e.g. *enabling DHCP guard with no trusted server is rejected with a
  clear error; with a server it applies*), not against the wire call sequence.
