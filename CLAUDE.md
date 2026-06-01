# CLAUDE.md — terraform-provider-unifi

Always-loaded operating instructions. Condensed form of [`docs/development.md`](docs/development.md);
read that + [`docs/architecture.md`](docs/architecture.md) before coding. **Status: in development —
Phase 0 base (provider scaffold + `internal/unifi` client) landed; resources build per
[`docs/roadmap.md`](docs/roadmap.md).** Full design in [`docs/`](docs/README.md).

A Terraform provider for **UniFi OS** (Network + Protect today; Access/others later), latest APIs,
clean-room, Registry `dovys/unifi`. Resources are **app-namespaced**: `unifi_network_*`, `unifi_protect_*`, ….

## Invariants (don't violate without an explicit, reviewed decision)
1. **Resources call the `internal/unifi` `Client` interface — never an HTTP transport.** Each typed
   client method targets its surface directly (`legacy*` / `v2*` helpers in `surfaces.go`); there is
   no per-op router.
2. **One cookie+CSRF session over legacy `/rest` + v2.** A **local admin account (username+password)
   is required.** Most writes are legacy REST (widest, most stable); v2 carries the newer
   zone-firewall / traffic / static-DNS / AP-group surfaces — same session, different URL prefix. No
   Integration/X-API-KEY surface.
3. **Capability-gate, don't assume firmware.** Missing capability → plan-time diagnostic (via
   `ModifyPlan`/`ValidateConfig`), never a raw 4xx.
4. **Safe by default.** One `unifi_network_device` resource adopts-if-needed + configures (+ read-only
   `data.unifi_network_device`); **destroy reverts config, never forgets** unless `forget_on_destroy=true`
   (guard with `prevent_destroy`). `attr_no_delete` → import/update-only, destroy = no-op state removal.
   Immutables `RequiresReplace`.
5. **Model infrastructure, not clients.** Only client touchpoint: `unifi_network_dhcp_reservation`
   (fixed-IP, find-or-create by MAC).
6. **Read reconciles** (Computed vs configured; normalize; remove-on-404); `UseStateForUnknown` only
   on post-create-immutable values.
7. **Live network is the bar** — every resource imports the real objects to a clean (+ second empty) plan.
8. **Comprehensive coverage**; validators mirror the controller (firewall `rule_index` full
   range); secrets `Sensitive`.

## Hard "don'ts"
❌ HTTP from resource code. ❌ Integration/X-API-KEY surface (single classic session only).
❌ `map[string]any` schemas (except generic `unifi_network_setting` + raw passthrough in GET-merge-PUT).
❌ client/end-user device modeling. ❌ reused provider/client source. ❌ whole-document PUT
(GET-merge-PUT owned fields only). ❌ List for keyed collections (use Map/Set).
❌ `UseStateForUnknown` on controller-mutable fields. ❌ over-tight validators.

## Docs hygiene
Docs describe **current state only**. A decision reverted within the same changeset is **edited away,
not recorded** — no superseded-decision archive, no "previously we considered…". Changing a shipped
invariant: update the docs in-PR + `CHANGELOG` + explain in the PR description.

## Architecture
Two surfaces, one session: **legacy** (`/api/s/<site>/rest` + `/stat` + `/cmd`, the widest, most
stable writes) and **v2** (`/v2/api/site`: zone-firewall, traffic, static-dns, ap-groups — note the
**v2 write contract** in architecture.md). Both run over the **classic** cookie+CSRF transport
(handles CSRF + ~2h JWT re-auth). Client = `Client` interface + `classic` transport + `surfaces.go`
(`legacy*`/`v2*` helpers) + `capabilities` (controller version). **Protect** reuses the same client +
console key under `/proxy/protect/...` (see [docs/apps/protect.md](docs/apps/protect.md)). The
Integration/X-API-KEY and Cloud Site Manager APIs are out of scope.

## Resource recipe
1. Confirm the surface (legacy `/rest` vs v2) and wire shape against the live network fixture.
2. Typed structs beside their client methods in `internal/unifi` (e.g. `NetworkConf` in `networks.go`,
   `StaticDNSRecord` in `dns.go`) — minimal structs for v2 writes. No separate `models` package.
3. Schema: nested attributes; Map/Set keyed by natural key (not List); `Optional+Computed`;
   `UseStateForUnknown` only on post-create-immutable; `RequiresReplace` immutables; controller-accurate
   validators; `Sensitive`.
4. CRUD + ImportState: Read reconciles + removes-on-404; GET-merge-PUT for partly-owned docs; document
   import key + state `id`.
5. Hand-written real-use-case examples in `examples/<use-case>/*.tf` (+ `examples/data/`), not
   per-resource snippets; use `import {}` blocks for resources you import rather than create (WANs, the
   Default network). `go generate` for the schema-reference docs (examples are not template-injected).
6. Tests to the Definition of Done.
7. Update `docs/apps/network.md` + `docs/roadmap.md` checklist.

## Definition of Done
Routes via `Client` · schema per recipe · unit tests (serialization/validators/plan-modifiers) ·
acceptance create→update→import(`ImportStateVerify`)→destroy with clean + **second empty** plan +
`CheckDestroy` + `tfacc-` sweeper · HIL test if hardware-dependent · live instance imports clean ·
example + generated docs · checklist updated.

## Commands (created in Phase 0)
```bash
make build       # go install
make test        # unit (no controller)
make testacc     # TF_ACC=1 go test ./... -timeout 60m  (Docker controller; HIL for hardware verbs)
go generate ./...; golangci-lint run; goreleaser release --clean
```
Acceptance env: `UNIFI_HOST/USERNAME/PASSWORD/SITE/INSECURE`. Never against production —
tiered tests in [`docs/testing.md`](docs/testing.md) (UniFi OS Server can't run in hosted CI).

## Conventions
- Go + `terraform-plugin-framework` (protocol v6); resources app-namespaced (`unifi_network_*`, `unifi_protect_*`, …).
- Python via `uv run` (global pref); commit only when asked; branch off `main` first.
