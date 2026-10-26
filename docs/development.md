# Development guide

This guide is for anyone adding or changing a resource, human or LLM. It covers the invariants every change must keep, the recipe for a new resource, and the Definition of Done. [CLAUDE.md](../CLAUDE.md) is its short form; the two must agree.

Many people and agents build this over a long time, so the main risk is drift: changes that are each reasonable but together break the design. The invariants below exist to prevent it.

Read [architecture.md](architecture.md) before writing code. For the PR workflow, see [CONTRIBUTING.md](../CONTRIBUTING.md). Report vulnerabilities privately as described in [SECURITY.md](../SECURITY.md).

## Invariants

A change that conflicts with an invariant is a proposal to change it. Make that proposal explicitly: update this file, CLAUDE.md, and any affected docs in the same PR, add a CHANGELOG entry, and explain the reason in the PR description. The PR history is the decision log.

Code comments cite invariants by number, so keep the numbers stable.

1. Resources call the app client: `c.Network()`, `c.Protect()`, `c.Access()`, or `c.Console()`. They never make HTTP calls or touch a transport. Each app's collections carry its surface contract:
   - Network: `LegacyCollection` (id passed separately on Update) and `V2Collection` (id in URL and body).
   - Protect: a PATCH collection under `/proxy/protect/api`.
   - Access: a core that unwraps the `{code,msg,data}` envelope.
   - Console: its own core over unifi-core `/api`.

   There is no per-operation router.
2. Every app shares one cookie + CSRF session. A local admin account (username and password) is required. The Integration API (`X-API-KEY`) and Cloud Site Manager are out of scope.
3. Gate on capability, not firmware version. A missing capability is a plan-time diagnostic from `ModifyPlan` or `ValidateConfig`, never a raw 4xx during apply.
4. Safe by default:
   - `unifi_network_device` adopts the device if needed, then configures it. `data.unifi_network_device` is read-only.
   - Destroying `unifi_network_device` reverts the config it applied. Destroying a Protect camera, light, sensor, chime, or viewer leaves its config as is. Both keep the device adopted unless `forget_on_destroy = true`, which should be guarded with `prevent_destroy`.
   - Objects the controller flags `attr_no_delete`, plus Access doors and door guards, are import/update-only. Destroy removes them from state and leaves the controller alone.
5. Model infrastructure, not clients. The only resources that touch client records are `unifi_network_dhcp_reservation` (fixed IP) and `unifi_network_client_ap_lock` (fixed AP). Both find or create the record by MAC, and destroy clears only their own flag, so both can share one client record.
6. Read reconciles. It refreshes Computed fields, rewrites a configured value only when the controller's value differs after normalization, and removes the resource from state on 404. Put `UseStateForUnknown` on `Optional+Computed` knobs; without it, every unpinned Computed field turns `known after apply` when a sibling changes, and an import never plans clean. This does not hide drift, because Read still reconciles every cycle. Telemetry fields are Computed-only so they never diff.
7. A live console is the bar. Every resource imports real objects to a clean plan, then a second empty plan.
8. Cover the full configurable surface of each app. Validators mirror the controller and are never tighter than it; for example, firewall `rule_index` accepts the full range. Secrets are `Sensitive`. Where the schema deliberately differs from the controller's model, document the difference in `docs/apps/<app>.md`.
9. Write only the fields the provider owns. The legacy `/rest` PUT shallow-merges on the server, so send a sparse PUT and never echo the GET back. Protect writes are a PATCH of owned fields. Two cases need the full document: a replace-style surface (v2 `global/config/network`, used by `unifi_network_mdns`) and an array the controller replaces wholesale (device `port_overrides`). For those, GET the live document, overlay the owned fields, and PUT it back.
10. Schemas use nested attributes, not blocks. Keyed collections are a Map or Set keyed by natural key, never a List. Immutable fields use `RequiresReplace`. No `map[string]any` or dynamic attributes. The only passthrough of unmodeled raw JSON is the `port_overrides` merge in `internal/unifi/network/devices.go`.
11. No code reused from other providers or clients.
12. The API surface is an implementation detail. When a resource moves between legacy and v2, or a firmware release changes an object's wire shape, only the collection and generated types change. The resource keeps its name and schema, and users see no diff or forced replacement.

A breaking schema change needs a deprecation cycle (`DeprecationMessage`) before the old attribute goes away. There is no major-version release path yet; see [releasing.md](releasing.md).

## Resource recipe

A new resource is a feature: branch off `next` and open the PR into `next` ([releasing.md](releasing.md)).

1. Confirm the surface and wire shape against a live controller. Surfaces come from observing the live API (browser devtools, the provider's wire logs, read-only GETs) and from the OpenAPI documents UniFi ships. See [Wire logs](#wire-logs).
2. Map the Terraform model to the generated `wire.<Type>` in `internal/unifi/<app>/wire` ([Wire types](#wire-types)).
3. Add the accessor and collection in `internal/unifi/<app>`. Write a custom collection only for a Read that filters a List or for an array-field merge. No hand-written per-type structs and no `models` package. v2 writes follow the [write contract](architecture.md#4-write-contract-per-surface).
4. Build the schema per invariants 6, 8, and 10. Controller-defaulted knobs are `Optional+Computed` with `UseStateForUnknown`.
5. Implement CRUD and ImportState per invariants 6 and 9. Document the import key and the state `id`.
6. Add examples as hand-written use cases in `examples/<use-case>/*.tf` and `examples/data/`, not per-resource snippets. Use `import {}` blocks for objects users import rather than create, such as WANs and the Default network. Note any state-backend or hardware/firmware caveat.
7. Run `make docs` to regenerate `docs/index.md`, `docs/resources`, and `docs/data-sources` with tfplugindocs. It needs no spec, and CI fails when `docs/` is stale. Don't use `go generate ./...`: it also runs the wire codegen. Resource examples are not injected into the generated docs; only `examples/provider/provider.tf` is, into `docs/index.md`.
8. Write tests to the [Definition of Done](#definition-of-done).
9. Update `docs/apps/<app>.md` and the [roadmap.md](roadmap.md) checklist.

## Definition of Done

- [ ] Routes through the app client (`c.Network()`, `c.Protect()`, `c.Access()`, or `c.Console()`) and targets the right surface.
- [ ] Schema meets invariants 6, 8, and 10.
- [ ] Unit tests (`*_ci_test.go`) for serialization (`httptest`), validators, and plan modifiers. `make test` passes.
- [ ] Acceptance test in `test/acc` (build tag `acc`): create, update, import (`ImportStateVerify`), destroy. Clean plan, then a second empty plan. `CheckDestroy` checks removal, or the revert for a config resource. A sweeper for `tfacc-` objects. Run `make testacc` and paste its summary into the PR description.
- [ ] HIL test in `test/hil` (build tag `hil`) if the resource depends on hardware. Tests that write to the console create only `tfacc-hil-` objects.
- [ ] Without hardware, the Docker acceptance lifecycle is the bar; record the untested hardware behavior in [TODO.md](../TODO.md).
- [ ] A live console imports existing objects to an empty plan (`make testhil`, which defaults to `read` mode). If you have no console, the maintainer runs this.
- [ ] Example added, `make docs` run, `docs/apps/<app>.md` and the roadmap checklist updated.
- [ ] Date-based `CHANGELOG.md` entry.
- [ ] `make check` passes, including the secret scan. Also run `golangci-lint run --build-tags=acc,hil`, since `make check` doesn't lint the acc and HIL test files.
- [ ] Tests and docs use RFC 5737 / RFC 3849 documentation addresses.

Test tiers and commands are in [testing.md](testing.md).

## Wire types

Network, Protect, and Access object types are generated `wire` structs: oapi-codegen, types only, in `internal/unifi/<app>/wire/types.gen.go` (marked DO NOT EDIT). The hand-written layer is the groups and collections, never the types. Console types are hand-written in `internal/unifi/console/types.go` because the UniFi OS surface ships no OpenAPI.

The types are generated from a version-pinned API spec that isn't public. `make generate` needs `UNIFI_SPECS` pointing at it, so only the maintainer runs it. Never hand-edit `types.gen.go`; if a generated type is missing or mistypes a field, open an issue naming the object and field. The generated types carry no vendor description comments.

## Wire logs

The transport logs every request and response through `tflog` at DEBUG under the message `wire HTTP`: method, URL, status, and the exact JSON bodies. Secret-bearing fields are masked; the redaction rules are in [architecture.md](architecture.md#7-logging-and-redaction). MACs, IPs, and names are not masked, so redact a log before sharing it.

```sh
TF_LOG_PROVIDER=DEBUG terraform apply 2>&1 | grep 'wire HTTP'
```

Use it to confirm which fields a surface accepts on PUT or POST versus only returns on GET. Capture the GET, then watch what a write round-trips or rejects; `api.err.InvalidValue` and `api.err.InvalidPayload` name a field the surface won't take.

The logs go through `tflog` rather than raw stderr because the provider runs as a go-plugin child, and the host discards its stderr unless `TF_LOG` is set.

## Docs hygiene

Docs describe the current design only. A decision made and reverted within one changeset is edited away, not recorded.
