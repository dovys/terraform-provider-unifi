# terraform-provider-unifi — Design Docs

> **Status.** The **Network** app (Phases 0–4) is shipped, usable, and hardware-verified on a live
> console; **Protect** (`unifi_protect_*`) and **Access** (`unifi_access_*`) are feature-complete on
> the same console session. Protect's reads are hardware-verified (live camera + NVR) and its
> `liveview` write round-trips; its other device-config writes — and all of Access (no live console on
> hand) — are reverse-engineered and **HIL-unverified** ([roadmap.md](roadmap.md), `TODO.md`).
> These docs describe the **current intended design** (not its history).

Manages a **UniFi OS** console as code — **Network**, **Protect**, and **Access** today (the other apps
later) — against the latest UniFi APIs. Resources are **app-namespaced** (`unifi_network_*`,
`unifi_protect_*`, `unifi_access_*`, …). Built clean-room (no reused third-party code).

| Doc | Covers |
|-----|--------|
| [architecture.md](architecture.md) | API surfaces, client, auth, the per-resource surface map, capability gating, safety. |
| [apps/network.md](apps/network.md) | **Network** app resource & data-source catalog: the resource design and what we don't model. |
| [apps/protect.md](apps/protect.md) | **Protect** app: cameras, doorbells, lights, sensors, chimes, viewers, liveviews, NVR settings — shares the client & session. |
| [apps/access.md](apps/access.md) | **Access** app: door groups, policies, holiday groups, doors, door guards — shares the client & session; private web-UI API behind a `{code,msg,data}` envelope. |
| [devices-and-ports.md](devices-and-ports.md) | The device model (one adopt-and-configure resource + data source) and per-port config. |
| [flows.md](flows.md) | Operator runbooks: add a device, VLAN, DHCP reservation, firewall rule, WLAN, import. |
| [testing.md](testing.md) | Tiered tests: unit + Docker controller + gated hardware-in-the-loop. |
| [development.md](development.md) | The long-term charter: invariants, the per-resource recipe, Definition of Done, docs hygiene. Condensed in [`CLAUDE.md`](../CLAUDE.md). |
| [roadmap.md](roadmap.md) | Phases, coverage checklist, open questions. |

## Decisions

- **App-namespaced resources** — `unifi_network_*`, `unifi_protect_*`, … One provider and **one
  console session** serve every app (mirrors AWS/Google service prefixing).
- **API: one cookie+CSRF session over legacy `/rest/` + v2 (`/v2/api/site`).** Each resource targets
  whichever surface owns its object. A **local admin account is required**; the Integration/X-API-KEY
  surface is out of scope.
- **Coverage:** comprehensive — everything in the live network, plus first-class per-switch-port
  config.
- **Devices:** one `unifi_network_device` resource that adopts-if-needed and configures (plus a
  read-only `data.unifi_network_device`); destroy reverts config and never forgets unless opted in.
  Client/end-user devices are **not** modeled — except fixed-IP DHCP reservations
  (`unifi_network_dhcp_reservation`).
- **Framework:** `terraform-plugin-framework` (protocol v6), Go, hand-written typed client.
- **Testing:** unit + Docker controller (legacy/v2) + gated self-hosted/HIL (real radios & hardware verbs).
- **Distribution:** private → public; Registry `dovys/unifi`; MIT.

## Non-goals

- Client/end-user device inventory, presence, or blocking.
- Cloud **Site Manager API** (`api.ui.com`, read-only).
- The other UniFi OS apps — **Access, Talk, Connect, InnerSpace** — future `unifi_<app>_*` namespaces,
  not in initial scope. (**Protect is in scope** — see [apps/protect.md](apps/protect.md).)
- Drop-in state compatibility with existing third-party UniFi providers.
