# Testing & sandbox strategy

Never test against production. Three tiers — because **UniFi OS Server can't run in GitHub-hosted
runners** (needs systemd, `SYS_ADMIN`, host cgroups; runners are unprivileged), but the plain Docker
controller covers the legacy + v2 surfaces where ~all writes live.

| Tier | Runs | Controller | Auth | Covers |
|------|------|:----------:|------|--------|
| **Unit** | every PR | none | — | router/capability logic, model (de)serialization (`httptest`), validators, plan modifiers, the **v2 write contract**. Most bugs live here. |
| **Acceptance (legacy/v2)** | merge / nightly | `linuxserver/unifi-network-application` (Docker) | classic | networks, WAN, firewall (classic), routing, port profiles, devices + port overrides, reservations, user groups, RADIUS, DNS. The bulk. |
| **Integration + HIL** | gated `workflow_dispatch` / self-hosted | UniFi OS Server (VM) or real gear | dual | Integration-API writes, real radios (WLAN broadcasting), zone-firewall enforcement, firmware quirks the Docker controller can't reproduce. |

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
AP). The **only** place that exercises Integration writes, real radios, zone-firewall enforcement,
and firmware-specific behavior (e.g. local DNS moving from v2 `/static-dns` to Integration
`dns/policies` as firmware advances). It's also
where the operator's **own live console** — captured as a local, gitignored fixture, never
committed — is imported end-to-end to a clean (and second consecutive empty) plan: the truest
regression.

## Honesty rule

The matrix states plainly that **Tier 2 does not cover the Integration API or several v2 firmware
quirks** — those are Tier 3 only. No claimed-but-absent coverage.

## "Tested" for a resource

Unit tests (schema/validators/serialization); a Tier-2 test doing **create → update → import
(`ImportStateVerify`) → destroy** with a clean intermediate plan **and a second consecutive empty
plan**; `CheckDestroy`; a sweeper; a Tier-3 HIL test if it needs radios/hardware (else noted as
sim-covered). Full Definition of Done in [development.md](development.md).
