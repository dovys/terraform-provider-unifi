# CLAUDE.md — terraform-provider-unifi

Always-loaded operating instructions. Condensed form of [`docs/development.md`](docs/development.md);
read that + [`docs/architecture.md`](docs/architecture.md) before coding. **Status: Network app
(P1–P4) shipped on the rebuilt client — a console-session `unifi.Client` + a substruct
`network.Client` (`c.Network().WAN().SLAs()…`) over generated `wire` types
([`../unifi-specs`](../unifi-specs)); Protect (P5/P6) next per
[`docs/roadmap.md`](docs/roadmap.md).** Full design in [`docs/`](docs/README.md).

A Terraform provider for **UniFi OS** (Network + Protect today; Access/others later), latest APIs,
clean-room, Registry `dovys/unifi`. Resources are **app-namespaced**: `unifi_network_*`, `unifi_protect_*`, ….

## Invariants (don't violate without an explicit, reviewed decision)
1. **Resources call the substruct `network.Client` (`c.Network().<Group>().<Collection>()`) — never an
   HTTP transport.** The console-session `unifi.Client` owns the authenticated session and exposes
   `Network()`; CRUD goes through generic collections (`LegacyCollection` id-separate-Update,
   `V2Collection` id-in-url+body) carrying the per-surface contract — no per-op router. Object types are
   generated `wire` structs (oapi-codegen, types-only) regenerated from [`../unifi-specs`](../unifi-specs);
   the hand-written facade is the groups/collections, never the types.
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
6. **Read reconciles** (Computed vs configured; normalize; remove-on-404). Use `UseStateForUnknown` on
   `Optional+Computed` knobs to pin their prior value through an unrelated update — otherwise the
   framework marks every un-pinned `Computed` field `known after apply` the moment a sibling changes, so a
   partially-configured import never reaches a clean plan (invariant 7). It can't mask drift the
   controller makes on its own — `Read` still reconciles each cycle.
7. **Live network is the bar** — every resource imports the real objects to a clean (+ second empty) plan.
8. **Comprehensive coverage**; validators mirror the controller (firewall `rule_index` full
   range); secrets `Sensitive`.

## Hard "don'ts"
❌ HTTP from resource code. ❌ Integration/X-API-KEY surface (single classic session only).
❌ `map[string]any` schemas (except generic `unifi_network_setting` + the raw passthrough in the device
`port_overrides` array merge). ❌ client/end-user device modeling. ❌ reused provider/client source.
❌ whole-document PUT or GET-then-merge of a full doc — the legacy `/rest` PUT **shallow-merges
server-side**, so send only the fields the provider owns (a sparse PUT); never echo the controller's GET
back. ❌ List for keyed collections (use Map/Set).
❌ over-tight validators.

## Docs hygiene
Docs describe **current state only**. A decision reverted within the same changeset is **edited away,
not recorded** — no superseded-decision archive, no "previously we considered…". Changing a shipped
invariant: update the docs in-PR + `CHANGELOG` + explain in the PR description.

## Architecture
Two surfaces, one session: **legacy** (`/api/s/<site>/rest` + `/stat` + `/cmd`, the widest, most
stable writes) and **v2** (`/v2/api/site`: zone-firewall, traffic, static-dns, ap-groups — note the
**v2 write contract** in architecture.md). Both run over the **classic** cookie+CSRF transport
(handles CSRF + ~2h JWT re-auth). The top-level `unifi.Client` is the **console session**: the classic
transport + probed `capabilities` + site management + a `Network()` accessor. **`internal/unifi/network`**
is the substruct Network-app client: groups (`WAN`, `WiFi`, `Firewall`, `Traffic`, `DNS`, `Devices`, …)
→ CRUD collections over generated **`wire`** types (`internal/unifi/network/wire`, codegen from the
sibling spec). **Protect** reuses the console session under `/proxy/protect/...` (see
[docs/apps/protect.md](docs/apps/protect.md)). The Integration/X-API-KEY and Cloud Site Manager APIs are
out of scope.

## Resource recipe
1. Confirm the surface (legacy `/rest` vs v2) and wire shape against the live network fixture.
2. Map the TF model to the generated `wire.<Type>` (regenerate from [`../unifi-specs`](../unifi-specs)
   if a field is missing/mistyped — fix the spec, never hand-edit `types.gen.go`). Add the group
   accessor + collection in `internal/unifi/network` (a custom collection only when the surface needs
   a List-filter Read or an array-field merge). No hand-written per-type structs, no `models` package.
3. Schema: nested attributes; Map/Set keyed by natural key (not List); `Optional+Computed` with
   `UseStateForUnknown` on the knobs (no post-import churn — invariant 6); `RequiresReplace` immutables;
   controller-accurate validators; `Sensitive`.
4. CRUD + ImportState: Read reconciles + removes-on-404; Update is a **sparse PUT of owned fields** (the
   legacy `/rest` PUT shallow-merges server-side, so omitted fields keep their stored value); GET-merge
   only an array field that the controller replaces wholesale (device `port_overrides`); document import
   key + state `id`.
5. Hand-written real-use-case examples in `examples/<use-case>/*.tf` (+ `examples/data/`), not
   per-resource snippets; use `import {}` blocks for resources you import rather than create (WANs, the
   Default network). `go generate` for the schema-reference docs (examples are not template-injected).
6. Tests to the Definition of Done.
7. Update `docs/apps/network.md` + `docs/roadmap.md` checklist.

## Definition of Done
Routes via the substruct `network.Client` (`c.Network()…`) · schema per recipe · unit tests (serialization/validators/plan-modifiers) ·
acceptance create→update→import(`ImportStateVerify`)→destroy with clean + **second empty** plan +
`CheckDestroy` + `tfacc-` sweeper · HIL test if hardware-dependent · live instance imports clean ·
example + generated docs · checklist updated.

## Commands (created in Phase 0)
```bash
make build       # go install
make test        # unit (no controller)
make testacc     # TF_ACC=1 go test ./... -timeout 60m  (Docker controller; HIL for hardware verbs)
make docs        # regenerate provider docs (tfplugindocs only; CI gate fails on a stale docs/)
make check       # full PR gate locally: build/vet, gofmt, go mod tidy, govulncheck, terraform fmt, docs, lint, unit
go generate ./...; golangci-lint run; goreleaser check   # releases are automated — never run `goreleaser release` by hand
```
**PR gates** (`.github/workflows/test.yml`, must be green on every PR): build & vet · gofmt · `go mod tidy`
clean · **govulncheck** (deps + stdlib) · **terraform fmt** (examples) · docs current · golangci-lint · unit.
**Supply-chain:** every action is pinned to a full commit **SHA** (not `@vN`) + service-container images pinned
by **digest**; Dependabot bumps both. The release build is `GOFLAGS=-mod=readonly` and runs **no** `go mod tidy`.
Acceptance env: `UNIFI_HOST/USERNAME/PASSWORD/SITE/INSECURE`. Never against production —
tiered tests in [`docs/testing.md`](docs/testing.md) (UniFi OS Server can't run in hosted CI).

## Conventions
- Go + `terraform-plugin-framework` (protocol v6); resources app-namespaced (`unifi_network_*`, `unifi_protect_*`, …).
- Python via `uv run` (global pref); commit only when asked; branch off `main` first (feature work targets `next` — see Releasing).

## Releasing (two-branch; full detail in [`docs/releasing.md`](docs/releasing.md))
Releases are **automatic**; CI **only tags + builds** — never hand-tag, never run `goreleaser release` locally. The
bump is derived from the merged PR's **source branch**, so target PRs by intent:
- **Fix / small change → PR into `main`.** On merge it auto-cuts the next **patch** (`v0.1.2`→`v0.1.3`).
- **Feature (any size, multi-PR) → PRs into `next`** (which never releases). Ship the feature by merging a
  **`next → main` PR** (use a **merge commit** — its CI tests the merged result first): head `next` ⇒ **minor**
  (`v0.2.0`), or add the **`release:major`** label ⇒ **major** (`v1.0.0`). minor/major reset lower parts to 0.
- After patches land on `main`, refresh `next` by hand (`git checkout next && git merge main`) — CI never writes branches.
- The **`release:skip`** PR label skips that release; docs/CI/dep-only merges auto-skip via `paths-ignore`.
- `CHANGELOG.md` is **date-based** — add your entry in the PR (the release never edits it).
