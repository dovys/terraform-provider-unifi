# Tier-3 HIL — import → clean-plan regression (Rung B)

Imports the operator's **live** UniFi console with the freshly-built provider and
asserts a **clean plan** — the truest regression ([`../../docs/testing.md`](../../docs/testing.md)).
Pairs with the read smoke test (`internal/provider/hil_read_test.go`, Rung A).

## Read-only guarantee

Import-discovery and `tofu plan` issue only **GETs** (plus the provider's
session-login POST). `run.sh` **never** runs `apply` or `destroy` — there is no
such codepath. Safe against a production console; no config is written.

## Run

From the Mac (the mgmt VLAN is workstation-only):

```bash
./run.sh
```

`run.sh` (1) builds + installs the provider and points a `dev_overrides` config
at it (no registry, no `init`), (2) runs the `hil`-tagged `TestHILGenerateImports`
to discover live object IDs and write `import {}` blocks to `imports.tf`, and
(3) runs `tofu plan -generate-config-out=generated.tf`, reporting whether every
object imports with **no** add/change/destroy.

Credentials: the `network-admin` local admin, password from 1Password
(`op … --vault <VAULT>`). Pre-set `UNIFI_PASSWORD` to skip `op`. Host defaults
to `https://192.0.2.1`; override with `UNIFI_HOST`.

`imports.tf`, `generated.tf`, state, and `dev.tfrc` are **gitignored** — they
contain real controller object IDs (the live console is a local fixture, never
committed).

## Coverage

Imports the `_id`-passthrough resources that exist on the console: `dns_record`,
`firewall_rule`, `user_group`, `wlan`, `wlan_group`, `ap_group`, `network`, and
`wan` (a `networkconf` with `purpose=wan`).

**Excluded** (write-only secrets — `-generate-config-out` nulls sensitive values,
so the round-trip can't be clean; they import fine with a hand-written config
supplying the secret):
- `radius_profile` — `auth_servers[].x_secret`.
- PPPoE WANs — `x_wan_password` (DHCP/static WANs are covered).

**Not yet covered:** `device` and `dhcp_reservation` — MAC-keyed with heavy
computed/stat fields; a later cut.

A non-empty plan for any covered resource is a **HIL finding** — a field the
provider's Read/normalize doesn't round-trip against live firmware — not a harness
failure.
