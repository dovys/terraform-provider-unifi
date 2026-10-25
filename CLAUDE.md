# CLAUDE.md

Operating rules for agents and maintainers in this repo. This is the short form of [docs/development.md](docs/development.md); read that and [docs/architecture.md](docs/architecture.md) before writing code. Design index: [docs/README.md](docs/README.md). Contributor workflow: [CONTRIBUTING.md](CONTRIBUTING.md).

## What this is

A clean-room Terraform provider for UniFi OS, Registry address `dovys/unifi` (not yet published to the Registry). Go, `terraform-plugin-framework`, protocol v6.

- Apps: Network, Protect, Access, Console. Status and next work: [docs/roadmap.md](docs/roadmap.md). Hardware-unverified write surfaces: [TODO.md](TODO.md).
- Resources are app-namespaced: `unifi_network_*`, `unifi_protect_*`, `unifi_access_*`, `unifi_console_*`.
- Surfaces are confirmed by observing the live API and vendor-shipped OpenAPI.

## Architecture

- One console session (`unifi.Client`, `internal/unifi`) owns auth, probed capabilities, site management, and one accessor per app: `c.Network()`, `c.Protect()`, `c.Access()`, `c.Console()`.
- Transport (`internal/unifi/transport`): classic cookie + CSRF, re-auth driven by the `TOKEN` JWT's own expiry, login throttle (`UNIFI_LOGIN_MIN_INTERVAL`).
- Network (`internal/unifi/network`, [docs/apps/network.md](docs/apps/network.md)) has two surfaces on that session:
  - legacy `/api/s/<site>/rest`, `/stat`, `/cmd`: most writes, widest and most stable.
  - v2 `/v2/api/site/...`: zone firewall, traffic, static DNS, AP groups. See the v2 write contract in architecture.md §4.
  - Accessors are groups (`c.Network().WAN().SLAs()`, `WiFi()`, `Firewall()`, `Traffic()`, `DNS()`, `Devices()`, …) or top-level collections (`PortForwards()`, `Networks()`, `StaticRoutes()`, `Settings()`, `PortProfiles()`).
- Protect (`internal/unifi/protect`, [docs/apps/protect.md](docs/apps/protect.md)): `/proxy/protect/api/...`; writes are a PATCH of owned fields.
- Access (`internal/unifi/access`, [docs/apps/access.md](docs/apps/access.md)).
- Console (`internal/unifi/console`, [docs/apps/console.md](docs/apps/console.md)): unifi-core `/api/...`.
- Object types are generated `wire` structs (oapi-codegen, types-only, `wire/types.gen.go`, DO NOT EDIT). The hand-written layer is the groups and collections, never the types. Exception: Console types are hand-written (`console/types.go`) because the OS surface ships no OpenAPI.
- The spec that generates `wire` is maintainer-only; `make generate` needs it. If a generated type is missing or mistypes a field, open an issue naming the object and field. Never hand-edit `types.gen.go`.

## Invariants

Change one only through a reviewed decision: update the docs in the same PR, add a CHANGELOG entry, explain it in the PR description.

1. Resources call the app client (`c.Network()`, `c.Protect()`, `c.Access()`, `c.Console()`), never HTTP or a transport. Each app's collections carry its surface contract: Network's `LegacyCollection` (id passed separately on Update) and `V2Collection` (id in URL and body), Protect's PATCH collection, Access's `{code,msg,data}` envelope core, Console's own core. No per-op router.
2. One cookie + CSRF session for every app. A local admin account (username + password) is required. The Integration API (`X-API-KEY`) and Cloud Site Manager are out of scope.
3. Gate on capability, not firmware. A missing capability is a plan-time diagnostic (`ModifyPlan`/`ValidateConfig`), never a raw 4xx at apply.
4. Safe by default:
   - `unifi_network_device` adopts if needed and configures; `data.unifi_network_device` is read-only.
   - Destroying `unifi_network_device` reverts the config it applied; destroying a Protect camera, light, sensor, chime, or viewer leaves its config as is. Both keep the device adopted unless `forget_on_destroy = true`; guard that with `prevent_destroy`.
   - Objects the controller flags `attr_no_delete`, plus Access doors and door guards, are import/update-only. Destroy removes them from state and leaves the controller alone.
5. Model infrastructure, not clients. The only client-record touchpoints find-or-create by MAC: `unifi_network_dhcp_reservation` owns the fixed IP, `unifi_network_client_ap_lock` the fixed AP. Destroy clears only that flag, so both coexist on one client record.
6. Read refreshes Computed fields, rewrites a configured value only when the controller's differs after normalization, and removes the resource from state on 404. Put `UseStateForUnknown` on `Optional+Computed` knobs, or every unpinned Computed field turns `known after apply` when a sibling changes and an import never plans clean. It does not hide drift; Read still reconciles each cycle.
7. A live console is the bar: every resource imports real objects to a clean plan, then a second empty plan.
8. Cover the full configurable surface. Validators mirror the controller and are never tighter than it (firewall `rule_index` accepts the full range). Secrets are `Sensitive`.
9. Write only the fields the provider owns. Legacy `/rest` PUT shallow-merges server-side, so send a sparse PUT and never echo the GET back. On a replace-style surface (v2 `global/config/network`, used by `unifi_network_mdns`) and for an array the controller replaces wholesale (device `port_overrides`), GET the live document, overlay the owned fields, and PUT it back.
10. Schemas use nested attributes and Map or Set keyed by natural key, never List for keyed collections. Immutable fields use `RequiresReplace`. No `map[string]any` or dynamic schema attributes. The only passthrough of unmodeled raw JSON is the `port_overrides` merge in `network/devices.go`.
11. No code reused from other providers or clients.

## Resource recipe

1. Confirm the surface and wire shape against a live controller. `TF_LOG_PROVIDER=DEBUG` logs every request and response body (`wire HTTP`, secrets masked).
2. Add the accessor and collection in `internal/unifi/<app>` over the generated `wire.<Type>`. Write a custom collection only for a List-filter Read or an array-field merge. No hand-written per-type structs, no `models` package.
3. Document the import key and state `id`.
4. Examples: hand-written use cases in `examples/<use-case>/*.tf` and `examples/data/`, not per-resource snippets. Use `import {}` blocks for objects you import rather than create (WANs, the Default network). Run `make docs` for the schema reference; examples are not template-injected.
5. Update `docs/apps/<app>.md` and the [docs/roadmap.md](docs/roadmap.md) checklist.

## Definition of Done

- Unit tests for serialization, validators, plan modifiers.
- Acceptance: create, update, import (`ImportStateVerify`), destroy; clean plan plus second empty plan; `CheckDestroy`; `tfacc-` sweeper.
- HIL test if hardware-dependent; otherwise the Docker lifecycle is the bar and any hardware gap goes in TODO.md.
- Example, regenerated docs, app doc, and roadmap updated.

## Commands and tests

Three tiers, each a root script ([docs/testing.md](docs/testing.md)). Never test against production.

| Command | Does |
|---|---|
| `make build` | `go install` |
| `make test` / `./test-ci.sh` | Tier 1 unit, no controller. CI runs it on PRs into `main`. |
| `make testacc` / `./test-extended.sh` | Tier 2 acceptance against a Docker controller it boots and configures itself. Subcommands: `up`, `down`, `--fresh`, `-run <Test>`. Run before every PR and paste its `Tier-2 acceptance summary` into the PR description. |
| `make testhil` / `./test-hil.sh read\|write` | Tier 3 against a live console, sparingly. `read` (default) only imports and plans. `write` creates and destroys `tfacc-hil-` objects after an interactive confirm. |
| `make docs` | Regenerate provider docs (tfplugindocs only). CI fails on stale `docs/`. |
| `make check` | Local PR gate: secret hygiene, build, vet, gofmt, `go mod tidy`, govulncheck, terraform fmt, docs, lint, unit. |
| `make generate` | Wire codegen. Maintainer-only (needs the spec). |
| `goreleaser check` | Validate release config. Never run `goreleaser release` by hand. |

- Test layout: unit tests are `*_ci_test.go`; acceptance in `test/acc` (build tag `acc`); HIL in `test/hil` (build tag `hil`).
- Lint the way CI does: `golangci-lint run --build-tags=acc,hil`. `make check` omits the tags.
- Provider env: `UNIFI_HOST`, `UNIFI_USERNAME`, `UNIFI_PASSWORD`, `UNIFI_SITE`, `UNIFI_ALLOW_INSECURE`; optional `UNIFI_TIMEOUT`, `UNIFI_MAX_RETRIES`.
- Run Python through `uv run` (`scripts/redact-capture.py`, codegen preprocessors).
- UniFi OS Server can't run in hosted CI, so Tier 2 and 3 run on a workstation.

## Secret hygiene

Treat every tracked file as public. `scripts/check-secrets.sh` is the first CI step and also runs in `make check`.

- No private IPs (10/8, 100/8; 192.168/16 only under `examples/`). Use RFC 5737 / RFC 3849 documentation ranges in tests and docs.
- No credential-fetch commands in tracked files.
- HIL config lives in gitignored `test/hil/secrets.env` (template: `secrets.env.example`).
- Never commit `unifi-inventory.json`, `/re-captures/`, `/.re/`, `/re/`. Redact captures with `uv run scripts/redact-capture.py`.

## CI and supply chain

- `.github/workflows/test.yml` is one `checks` job (shared Go install and build cache): secret hygiene, build, vet, gofmt, `go mod tidy` clean, govulncheck (deps and stdlib), terraform fmt on examples, docs current, golangci-lint.
- `checks` runs on every PR (it is the required status) and waits for approval on fork PRs.
- Unit tests run only on PRs into `main`. PRs into `next` skip them; they run at the `next → main` merge. Run `make test` locally while iterating.
- CI pins Go with `go-version-file: go.mod`, never `stable`.
- Every job runs on `ubuntu-latest`. Never add a self-hosted runner: a public repo's PRs run their own workflow files on it.
- Third-party actions are pinned to a full commit SHA. GitHub's own `actions/*` float on `@vN`. Service-container images are pinned by digest.
- The release build uses `GOFLAGS=-mod=readonly` and runs no `go mod tidy`.
- Dependabot runs weekly and groups each ecosystem's minor+patch bumps into one PR; majors get their own PR. `dependabot-automerge.yml` squash-merges a green Go-module minor/patch PR, which cuts a patch release.

### CVE policy

Fix every govulncheck advisory in the PR that finds it, whether or not the PR introduced it.

- Stdlib: raise the `toolchain` line in `go.mod`. This patches local builds, CI, and the release binary at once.
- Dependency: bump the module.
- Only if no patched release exists, record the advisory ID and reason in the PR and track a follow-up. Never merge past a red govulncheck.
- Dependabot never raises the toolchain, so a new stdlib advisory turns every Dependabot PR red until someone bumps it.

## Branches and releases

Full detail: [docs/releasing.md](docs/releasing.md).

- Always work through a PR. Branch off the branch you target:
  - Feature (new resource, data source, or behavior): branch off `next`, PR into `next`.
  - Bugfix, docs, CI, or dependency change: branch off `main`, PR into `main`.
- Commit and PR titles use a `<module>: change` prefix (`protect:`, `ci:`, `deps:`, `docs:`). GoReleaser drops `deps:`, `ci:`, `chore:`, `docs:`, `test:` from release notes.
- `CHANGELOG.md` is date-based. Add the entry in your PR; the release never edits it.
- The maintainer's agents open the PR, then merge when confident and tell the maintainer; ask first when unsure. Contributors and their agents open the PR and wait for review.
- Every push to `main` cuts a release; CI tags and builds, so never tag by hand. The bump comes from git topology:
  - Squash-merge into `main` (single parent): patch, e.g. `v0.1.2` → `v0.1.3`.
  - `next → main` PR merged with a real merge commit: minor, patch resets to 0. The bump is minor only if `HEAD^2` is in `next`, so a squash ships as a patch.
  - No major path yet; add one before v1.0.
- `next` never releases and CI never writes branches. Refresh `next` by hand after each release, and before branching if it is behind: `git checkout next && git merge main`.

## Docs hygiene

- Docs describe current state only.
- A decision reverted within the same changeset is edited away, not recorded. No "previously we considered…".
