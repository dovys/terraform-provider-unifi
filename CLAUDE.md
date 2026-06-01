# CLAUDE.md — terraform-provider-unifi

Always-loaded operating instructions. Condensed form of [`docs/development.md`](docs/development.md);
read that + [`docs/architecture.md`](docs/architecture.md) before coding. **Status: in development —
Phase 0 base (provider scaffold + `internal/unifi` client) landed; resources build per
[`docs/roadmap.md`](docs/roadmap.md).** Full design in [`docs/`](docs/README.md).

A Terraform provider for **UniFi OS** (Network + Protect today; Access/others later), latest APIs,
clean-room, Registry `dovys/unifi`. Resources are **app-namespaced**: `unifi_network_*`, `unifi_protect_*`, ….

## Invariants (don't violate without an explicit, reviewed decision)
1. **Resources call the `internal/unifi` `Client` interface — never an HTTP transport.** Routing
   lives in `router.go`.
2. **Official-where-capable, legacy/v2 fallback.** Router prefers Integration when firmware supports
   the write, else legacy/v2. Dual auth; a **local admin account is effectively required** today.
   New Integration write later = one-line routing change, zero resource edits.
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
❌ HTTP from resource code. ❌ `map[string]any` schemas (except generic `unifi_network_setting` + raw
passthrough in GET-merge-PUT). ❌ client/end-user device modeling. ❌ reused provider/client source.
❌ whole-document PUT (GET-merge-PUT owned fields only). ❌ List for keyed collections (use Map/Set).
❌ `UseStateForUnknown` on controller-mutable fields. ❌ over-tight validators.

## Docs hygiene
Docs describe **current state only**. A decision reverted within the same changeset is **edited away,
not recorded** — no superseded-decision archive, no "previously we considered…". Changing a shipped
invariant: update the docs in-PR + `CHANGELOG` + explain in the PR description.

## Architecture
Three surfaces: **Integration v1** (`/integration/v1`, `X-API-KEY`, firmware-gated writes),
**legacy** (`/api/s/<site>/rest`, cookie+CSRF, widest writes), **v2** (`/v2/api/site`, cookie+CSRF:
zone-firewall, traffic, static-dns, ap-groups — note the **v2 write contract** in architecture.md).
Client = `Client` interface + `integration`/`classic` transports (classic handles CSRF + ~2h JWT
re-auth) + `router` + `capabilities` (version, feature flags, site name↔UUID). **Protect** reuses the
same client + console key under `/proxy/protect/...` (see [docs/apps/protect.md](docs/apps/protect.md)). Cloud
Site Manager API out of scope.

## Resource recipe
1. Confirm backend/shape against the live network fixture; add `router.go` entry.
2. Typed models in `internal/unifi/models` (minimal structs for v2 writes).
3. Schema: nested attributes; Map/Set keyed by natural key (not List); `Optional+Computed`;
   `UseStateForUnknown` only on post-create-immutable; `RequiresReplace` immutables; controller-accurate
   validators; `Sensitive`.
4. CRUD + ImportState: Read reconciles + removes-on-404; GET-merge-PUT for partly-owned docs; document
   import key + state `id`.
5. Example `.tf` + `import.sh`; `go generate`.
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
make testacc     # TF_ACC=1 go test ./... -timeout 60m  (Docker controller; HIL for Integration)
go generate ./...; golangci-lint run; goreleaser release --clean
```
Acceptance env: `UNIFI_HOST/USERNAME/PASSWORD/API_KEY/SITE/INSECURE`. Never against production —
tiered tests in [`docs/testing.md`](docs/testing.md) (UniFi OS Server can't run in hosted CI).

## Conventions
- Go + `terraform-plugin-framework` (protocol v6); resources app-namespaced (`unifi_network_*`, `unifi_protect_*`, …).
- Python via `uv run` (global pref); commit only when asked; branch off `main` first.
