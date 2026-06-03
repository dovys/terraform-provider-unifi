# Development charter

Built over a long timeframe by many hands/agents. The biggest risk is **drift** — each change
locally reasonable, the sum betraying the design. This is the contract that keeps changes aligned;
[`../CLAUDE.md`](../CLAUDE.md) is the always-loaded condensed form.

> Before coding, read this + [architecture.md](architecture.md). A change that conflicts with an
> invariant below isn't a workaround — it's a proposal to change the invariant, made explicitly.

## Invariants (don't violate without an explicit, reviewed decision)

1. **Resources call the substruct `network.Client` (`c.Network()…`), never a transport.** The
   console-session `unifi.Client` owns the session and exposes `Network()`; CRUD runs through the
   network groups' generic collections (`LegacyCollection`/`V2Collection`), which target the legacy
   `/rest` or v2 surface directly over one cookie+CSRF session. There is no per-op router and no
   Integration/X-API-KEY surface.
2. **Surface choice is an implementation detail.** Moving a resource between legacy and v2 (or a new
   firmware moving an object's wire shape) changes only the group's collection/codegen — never rename
   or re-architect the resource. Schema stays stable across the API's evolution.
3. **Capability-gate, don't assume firmware.** Missing capability → plan-time diagnostic, never a
   raw 4xx mid-apply.
4. **Safe by default; danger is isolated + guarded.** `unifi_network_device` destroy reverts config and
   keeps the device adopted; forgetting needs `forget_on_destroy = true`. `attr_no_delete` objects are
   import/update-only. Immutables `RequiresReplace`.
5. **Read reconciles, it doesn't blind-overwrite.** Refresh Computed; reconcile configured only when
   canonically different (normalize on read); remove-on-404. `UseStateForUnknown` pins an
   `Optional+Computed` knob's prior value so an unconfigured field doesn't churn to `known after apply`
   on an unrelated update (it can't mask controller-side drift — `Read` still reconciles). Telemetry is
   **Computed-only** (so it never diffs).
6. **The live network is the bar.** Every resource round-trips the real objects from the live
   network to a clean plan (and a second empty plan).
7. **Comprehensive coverage is a floor.** Model the full UniFi Network feature surface; design
   differences are deliberate and documented.
8. **Validators mirror the controller, not folklore** (e.g. firewall `rule_index` full range).
9. **Clean-room only** (no reused provider/client code); **secrets always `Sensitive`**.

## Docs hygiene (current state only)

- Docs describe the **current intended design**, not how we got there. A decision made and reverted
  **within the same changeset** (one PR or a set of PRs before release) is **edited away, not
  recorded** — no "previously we considered…", no superseded-decision archive.
- Changing a shipped invariant: update this file + the affected docs in the same PR, note it in
  `CHANGELOG`, and explain the rationale in the PR description. Breaking schema changes also need a
  deprecation cycle (`DeprecationMessage`) + a major-version bump. We don't keep a parallel decision
  log; the PR history is the trail.

## Resource recipe (every resource, same way)

1. Confirm the surface (legacy `/rest` vs v2) + wire shape against the live network fixture (and
   UniFi OS Server if needed).
2. Map the TF model to the generated `wire.<Type>` (`internal/unifi/network/wire`); if a field is
   missing or mistyped, fix the per-object spec in [`../unifi-specs`](../unifi-specs) and regenerate —
   never hand-edit `types.gen.go`. Add the group accessor + collection in `internal/unifi/network`
   (custom collection only for id-in-body merge or List-filter Read). No separate `models` package, and
   no `map[string]any` except the generic `unifi_network_setting` and raw passthrough of unmodeled
   fields in GET-merge-PUT. v2 writes honor the [v2 write contract](architecture.md).
3. Schema: nested **attributes** (not blocks); Map/Set keyed by a natural key, not List;
   `Optional+Computed` for API-defaulted, with `UseStateForUnknown` on the knobs so a partially-configured
   import still reaches a clean plan; `RequiresReplace` immutables; controller-accurate validators;
   `Sensitive` secrets.
4. CRUD + ImportState: Read reconciles + removes-on-404; Create/Update match the plan (GET-merge-PUT
   for partly-owned docs); document the import key + state `id`.
5. Hand-written real-use-case examples in `examples/<use-case>/*.tf` (+ `examples/data/`), not
   per-resource snippets; use `import {}` blocks for resources you import rather than create (WANs, the
   Default network). `go generate` produces the schema-reference docs (examples are not template-injected).
   Note state backend + hardware/firmware caveats.
6. Tests to the Definition of Done.
7. Update [apps/network.md](apps/network.md) + the [roadmap.md](roadmap.md) checklist.

## Wire-level debugging (`UNIFI_DEBUG_HTTP`)

Set `UNIFI_DEBUG_HTTP=1` to dump every request/response — method, URL, and the
exact JSON body — to stderr, with secret-bearing fields (`password`,
`passphrase`, `psk`, `token`, …) masked. This is the tool for **reverse-engineering
a surface**: which fields the controller actually accepts on PUT/POST vs only
returns on GET. The recipe step-1 confirmation ("confirm the surface + wire
shape") is exactly this — capture the live GET, then watch what a write
round-trips or rejects (`api.err.InvalidPayload` names a field the surface won't
take). Off by default; never on in normal runs.

```
UNIFI_DEBUG_HTTP=1 tofu apply 2>capture.log   # stderr carries the dump
```

## Definition of Done

- [ ] Routes via `Client`; targets the correct surface (legacy `/rest` or v2).
- [ ] Schema per recipe; immutables `RequiresReplace`; secrets `Sensitive`.
- [ ] Unit tests: serialization (`httptest`), validators, plan modifiers.
- [ ] Acceptance test: create → update → import (`ImportStateVerify`) → destroy, clean intermediate
      plan + **second empty plan**; `CheckDestroy` (removal, or overlay-revert for config resources);
      `tfacc-`-prefixed sweeper.
- [ ] HIL test if hardware-dependent (else noted as sim-covered).
- [ ] Live instance imports to an empty plan.
- [ ] Example + generated docs; checklist updated.

## Long-horizon cadence

- **Per release:** run the live-network import as a smoke test (owner opt-in) — the truest regression.
- **Quarterly:** regenerate the local network fixture; anything new it surfaces (API paths, object
  shapes, resource types) becomes a roadmap item.
- **Capability watch:** when a Network version changes an object's surface or wire shape, update the
  client method only (invariant 2). Such migrations stay invisible to users (no schema change / forced recreate).
