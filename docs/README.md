# terraform-provider-unifi — Design Docs

> **Status: design / planning. No provider code exists yet.** These docs describe the **current
> intended design** (not its history). Implementation starts after review.

Manages a **UniFi OS** console as code — **Network** and **Protect** today (Access and the other apps
later) — against the latest UniFi APIs. Resources are **app-namespaced** (`unifi_network_*`,
`unifi_protect_*`, …). Built clean-room (no reused third-party code).

| Doc | Covers |
|-----|--------|
| [architecture.md](architecture.md) | API surfaces, hybrid client, auth, per-resource backend routing, capability gating, safety. |
| [resources.md](resources.md) | **Network** app resource & data-source catalog: the resource design and what we don't model. |
| [protect.md](protect.md) | **Protect** app: cameras, doorbells, lights, sensors, chimes, viewers, liveviews, NVR settings — shares the client & key. |
| [devices-and-ports.md](devices-and-ports.md) | The device model (one adopt-and-configure resource + data source) and per-port config. |
| [flows.md](flows.md) | Operator runbooks: add a device, VLAN, DHCP reservation, firewall rule, WLAN, import. |
| [my-network.md](my-network.md) | The live network (a local, gitignored fixture) → coverage map, drift, migration order. |
| [testing.md](testing.md) | Tiered tests: unit + Docker controller + gated hardware-in-the-loop. |
| [development.md](development.md) | The long-term charter: invariants, the per-resource recipe, Definition of Done, docs hygiene. Condensed in [`CLAUDE.md`](../CLAUDE.md). |
| [roadmap.md](roadmap.md) | Phases, coverage checklist, open questions. |

## Decisions

- **App-namespaced resources** — `unifi_network_*`, `unifi_protect_*`, … One provider and **one
  console API key** serve every app (mirrors AWS/Google service prefixing).
- **API: official-where-capable, legacy/v2 fallback.** The router prefers the Integration API
  (`/integration/v1`) when firmware supports the write, else legacy `/rest/` + v2 (`/v2/api/site`).
  Dual auth; a **local admin account is effectively required** today.
- **Coverage:** comprehensive — everything in the live network, plus first-class per-switch-port
  config.
- **Devices:** one `unifi_network_device` resource that adopts-if-needed and configures (plus a
  read-only `data.unifi_network_device`); destroy reverts config and never forgets unless opted in.
  Client/end-user devices are **not** modeled — except fixed-IP DHCP reservations
  (`unifi_network_dhcp_reservation`).
- **Framework:** `terraform-plugin-framework` (protocol v6), Go, hand-written typed client.
- **Testing:** unit + Docker controller (legacy/v2) + gated self-hosted/HIL (Integration & radios).
- **Distribution:** private → public; Registry `dovys/unifi`; MIT.

## Non-goals

- Client/end-user device inventory, presence, or blocking.
- Cloud **Site Manager API** (`api.ui.com`, read-only).
- The other UniFi OS apps — **Access, Talk, Connect, InnerSpace** — future `unifi_<app>_*` namespaces,
  not in initial scope. (**Protect is in scope** — see [protect.md](protect.md).)
- Drop-in state compatibility with existing third-party UniFi providers.
