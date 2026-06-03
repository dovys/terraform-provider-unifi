# Tier-3 HIL — live-console verification

Two runners against the operator's **live** UniFi console
([`../../docs/testing.md`](../../docs/testing.md)):

- **`run.sh` — read-only** (Rung B): imports the live objects and asserts a
  **clean plan**, the truest regression. Pairs with the read smoke test
  (`internal/provider/hil_read_test.go`, Rung A). **Never writes.**
- **`write.sh` — write verification** (bucket 2): **creates then destroys**
  additive, inert objects to exercise write paths the read rungs can't reach.
  Opt-in and gated. See [Write verification](#write-verification-writesh) below.

## Read-only guarantee (run.sh)

Import-discovery and `tofu plan` issue only **GETs** (plus the provider's
session-login POST). `run.sh` **never** runs `apply` or `destroy` — there is no
such codepath. Safe against a production console; no config is written.

## Run

First time, point the runner at your console (gitignored — never committed):

```bash
cp secrets.env.example secrets.env   # then edit: UNIFI_HOST + a local-admin user/pass
```

Then, from the workstation that can reach the console's management address:

```bash
./run.sh
```

`run.sh` (1) builds + installs the provider and points a `dev_overrides` config
at it (no registry, no `init`), (2) runs the `hil`-tagged `TestHILGenerateImports`
to discover live object IDs and write `import {}` blocks to `imports.tf`, and
(3) runs `tofu plan -generate-config-out=generated.tf`, reporting whether every
object imports with **no** add/change/destroy.

Config (host + a **local-admin** username/password) comes from
`test/hil/secrets.env` — gitignored, copied from `secrets.env.example`, sourced by
`run.sh`. Supply the password any way you like (inline, or fetched from a password
manager). Or just export `UNIFI_HOST` / `UNIFI_USERNAME` / `UNIFI_PASSWORD`
yourself. Use a **local** UniFi OS admin, not a cloud/SSO account (2FA → HTTP 499).

`secrets.env`, `imports.tf`, `generated.tf`, state, and `dev.tfrc` are
**gitignored** — they
contain real controller object IDs (the live console is a local fixture, never
committed).

## Coverage

Imports the `_id`-passthrough resources that exist on the console:

- **Network:** `dns_record`, `firewall_rule`, `user_group`, `wlan`, `wlan_group`,
  `ap_group`, `network`, `wan` (a `networkconf` with `purpose=wan`), and
  `dhcp_reservation` (the `users` carrying a fixed-IP reservation,
  `use_fixedip=true`).
- **Protect** (when installed — best-effort, skipped on a Protect-less console):
  `camera`, `liveview`, and the `nvr_settings` singleton.

`firewall_group` and `wan_sla` are emitted too but are empty on the recon console
(nothing to import). The Rung A read smoke (`hil_read_test.go`) additionally
forces a wire decode of every Network collection + all Protect surfaces
(lights/sensors/chimes/viewers are empty here).

**Excluded — write-only secrets** (`-generate-config-out` nulls sensitive values,
so the round-trip can't be clean; they import fine with a hand-written config
supplying the secret):
- `radius_profile` — `auth_servers[].x_secret`.
- PPPoE WANs — `x_wan_password` (DHCP/static WANs are covered).

**Excluded — runtime-volatile telemetry** (a plan-only flow can't be clean
without an `apply` + second plan, which this read-only harness deliberately
omits):
- `device` — computed read-only telemetry (`ip`, `state`) drifts every read, so
  `-generate-config-out` always reports it "known after apply". Rung A
  (`internal/provider/hil_read_test.go`) already verifies device read/decode.

A non-empty plan for any covered resource is a **HIL finding** — a field the
provider's Read/normalize doesn't round-trip against live firmware — not a harness
failure.

## Write verification (write.sh)

Reaches the **write paths** the read-only rungs can't: resources whose live
collection is empty (nothing to import) and write semantics (GET-merge-PUT,
update-merge). It **writes to the live console**, so every object is **additive**
(never mutates an existing object), **inert** (disabled rules / RFC-5737 blackhole
routes / unreferenced groups — zero effect on live traffic while it exists), and
**named `tfacc-hil-`** so the sweep can always reclaim it. A deliberate one-shot,
**not** a CI gate.

```bash
./write.sh          # all tiers (W0 → W1 → W2), interactive "I understand" confirm
./write.sh W0       # one tier:  W0 data · W1 gateway-inert · W2 AP/switch
./write.sh W0 --yes # skip the confirmation
```

Config is the same `test/hil/secrets.env` as `run.sh`; the W0 `dhcp_reservation`
test additionally needs `UNIFI_HIL_RESERVATION_IP` (an unused IP in your DHCP
scope) and the W1 `firewall_rule` test honors `UNIFI_HIL_FW_RULE_INDEX` (default
`4000`) — both can live in `secrets.env`.

The harness (`internal/provider/hil_write_*_test.go`, build tag `hil`) drives the
standard **create → update-one-field → import(`ImportStateVerify`) → second empty
plan → destroy + `CheckDestroy`** lifecycle per object against real firmware.
`write.sh` runs a **pre-run sweep**, the tests, then a **post-run sweep** (the
final net for anything a failed destroy left behind).

**Safety gates** (all required to write): the `//go:build hil` tag keeps these out
of `make test`/CI; `UNIFI_HIL_ALLOW_WRITES=1` is the opt-in; `UNIFI_HIL_WRITE_HOST`
must equal `UNIFI_HOST` (a host allowlist — a mistyped/production host is refused);
and `write.sh` adds the interactive confirmation. `assertInert` fails any config
before apply that isn't named `tfacc-hil-` or is missing its inertness tokens
(`enabled = false`, a blackhole prefix).

**Tiers** (one per `hil_write_<tier>_test.go`; blast radius on the live network):

- **W0** — control-plane data, no dataplane/RF/hardware effect even if a teardown
  leaks: `firewall_group`, `user_group`, `radius_profile` (+`x_secret`),
  `radius_account` (+`x_password`), `dns_record`, `protect_liveview`,
  `dhcp_reservation` (a fixed-IP for an unused, locally-administered MAC).
- **W1** — installed into the gateway config (a provision/reload) but inert:
  `static_route` (blackhole), `port_forward`/`traffic_rule`/`traffic_route`/
  `firewall_rule` (`enabled=false`), `port_profile` (created **unassigned**).
- **W2** — AP/switch provisioning (a brief management-plane blip; run during low
  usage): `wlan_group`, `ap_group` (empty/unreferenced).

`write.out`, `dev.tfrc`, and any generated state are **gitignored** (they reference
real console object IDs). W3 resources (device PUT, real network/wlan, settings
singletons, Protect device writes, `dynamic_dns`) are **never** run here — see
[`../../TODO.md`](../../TODO.md) *Verify against hardware*.
