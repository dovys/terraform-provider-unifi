# terraform-provider-unifi docs

This provider manages a UniFi OS console as code against the current UniFi APIs: the console itself plus its Network, Protect, and Access apps. `docs/` is also the Terraform Registry docs root, so it holds two kinds of docs. tfplugindocs generates the user reference. The rest, including this file, is for maintainers. These docs describe the current design, not its history.

## Status

The Network app and console certificate issuance are shipped and verified on a live console. Protect and Access cover their full configurable surface and use the same login session as Network. On a live console, Protect reads are verified, and so is creating a liveview with no slots. The other Protect writes are built from UniFi's shipped Protect OpenAPI and the observed API. Access is built from the version-pinned API spec and has never run against a live console. [roadmap.md](roadmap.md) tracks phases, and [../TODO.md](../TODO.md) lists what is still unverified.

## User reference (generated)

`make docs` generates these from the provider schemas. Don't edit them by hand. CI fails when they are stale.

| Path | Covers |
|------|--------|
| [index.md](index.md) | Provider configuration and authentication. |
| [resources/](resources/) | One page per resource. |
| [data-sources/](data-sources/) | One page per data source. |

## Design and maintainer docs

| Doc | Covers |
|-----|--------|
| [architecture.md](architecture.md) | API surfaces, the client, auth, which surface each resource uses, capability gating, safety. |
| [development.md](development.md) | Invariants, the per-resource recipe, Definition of Done, docs rules. [../CLAUDE.md](../CLAUDE.md) is the short form. |
| [apps/network.md](apps/network.md) | Network resources and data sources, and what isn't modeled. |
| [apps/protect.md](apps/protect.md) | Protect: cameras, doorbells, lights, sensors, chimes, viewers, liveviews, automations, NVR settings. |
| [apps/access.md](apps/access.md) | Access: door groups, doors, door guards, policies, holiday groups. |
| [apps/console.md](apps/console.md) | The UniFi OS layer (`unifi_console_*`), such as the console certificate. |
| [devices-and-ports.md](devices-and-ports.md) | The device model (one adopt-and-configure resource plus a data source) and per-port config. |
| [flows.md](flows.md) | Operator walkthroughs: add a device, VLAN, DHCP reservation, firewall rule, or WLAN, and import existing objects. |
| [testing.md](testing.md) | The three test tiers: unit, acceptance against a Docker controller, and hardware-in-the-loop against a live console. |
| [releasing.md](releasing.md) | The two-branch model and automatic releases. |
| [reverse-engineering.md](reverse-engineering.md) | How API shapes are captured from the live API and UniFi's OpenAPI documents. |
| [roadmap.md](roadmap.md) | Phases, the coverage checklist, open questions. |

## Design decisions

- Namespacing: resource names carry the app (`unifi_console_*`, `unifi_network_*`, `unifi_protect_*`, `unifi_access_*`). One provider and one session serve every app, the way the AWS and Google providers prefix by service.
- Infrastructure, not clients: the provider leaves end-user devices alone. Two Network resources are the exception, both attributes of a client record found by MAC: `unifi_network_dhcp_reservation` (fixed IP) and `unifi_network_client_ap_lock` (AP pinning).
- Auth: one cookie and CSRF session as a local admin account.
- API surfaces: Network has the legacy `/api/s/<site>/rest` surface and the v2 `/v2/api/site` surface, and each resource uses whichever serves its object ([architecture.md](architecture.md) lists each). Protect and Access live under `/proxy/protect` and `/proxy/access`. The console layer uses UniFi OS endpoints such as `/api/userCertificates`.
- Coverage: the full configurable surface of each app, including per-switch-port config.
- Devices: `unifi_network_device` adopts if needed and configures. Destroy always reverts the device's config. It also forgets the device only when `forget_on_destroy = true`.
- Stack: Go and `terraform-plugin-framework` (protocol v6). The client facade is hand-written over Go types generated from a version-pinned API spec that isn't public. `make generate` needs `UNIFI_SPECS` pointing at it.
- Clean-room: no reused third-party provider or client code.
- Distribution: Terraform Registry address `dovys/unifi`. MIT license.

## Non-goals

- Client inventory, presence, or blocking.
- The cloud Site Manager API (`api.ui.com`) and the Integration API (`X-API-KEY`).
- Other UniFi OS apps such as Talk, Connect, and InnerSpace, for now. Each would get its own `unifi_<app>_*` namespace.
- State compatibility with other UniFi providers.

## Elsewhere in the repo

- [../README.md](../README.md): install and usage.
- [../examples/](../examples/): examples by use case. The provider block is in `examples/provider/provider.tf`.
- [../CONTRIBUTING.md](../CONTRIBUTING.md): contributor workflow.
- [../CHANGELOG.md](../CHANGELOG.md): changes by date.
- [../SECURITY.md](../SECURITY.md): how to report a vulnerability.
