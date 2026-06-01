# Development charter

Built over a long timeframe by many hands/agents. The biggest risk is **drift** — each change
locally reasonable, the sum betraying the design. This is the contract that keeps changes aligned;
[`../CLAUDE.md`](../CLAUDE.md) is the always-loaded condensed form.

> Before coding, read this + [architecture.md](architecture.md). A change that conflicts with an
> invariant below isn't a workaround — it's a proposal to change the invariant, made explicitly.

## Invariants (don't violate without an explicit, reviewed decision)

1. **Resources call the `Client` interface, never a transport.** All routing lives in `router.go` —
   so moving an op onto the Integration API is a one-line change, zero resource edits.
2. **Backend choice is data, not destiny.** New Integration write → flip the routing policy +
   capability flag; never rename or re-architect the resource. Schema stays stable across the API's
   evolution.
3. **Capability-gate, don't assume firmware.** Missing capability → plan-time diagnostic, never a
   raw 4xx mid-apply.
4. **Safe by default; danger is isolated + guarded.** `unifi_network_device` destroy reverts config and
   keeps the device adopted; forgetting needs `forget_on_destroy = true`. `attr_no_delete` objects are
   import/update-only. Immutables `RequiresReplace`.
5. **Read reconciles, it doesn't blind-overwrite.** Refresh Computed; reconcile configured only when
   canonically different (normalize on read); remove-on-404. `UseStateForUnknown` only on
   post-create-immutable values. Telemetry is **Computed-only** (so it never diffs).
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

1. Confirm backend + shape against the live network fixture (and UniFi OS Server if needed); add
   the `router.go` policy entry.
2. Typed models in `internal/unifi/models` — no `map[string]any` except the generic `unifi_network_setting`
   and raw passthrough of unmodeled fields in GET-merge-PUT. v2 writes use minimal structs honoring
   the [v2 write contract](architecture.md).
3. Schema: nested **attributes** (not blocks); Map/Set keyed by a natural key, not List;
   `Optional+Computed` for API-defaulted; `UseStateForUnknown` only on post-create-immutable values;
   `RequiresReplace` immutables; controller-accurate validators; `Sensitive` secrets.
4. CRUD + ImportState: Read reconciles + removes-on-404; Create/Update match the plan (GET-merge-PUT
   for partly-owned docs); document the import key + state `id`.
5. Example `.tf` + `import.sh`; `go generate` docs; state backend + hardware/firmware caveats.
6. Tests to the Definition of Done.
7. Update [resources.md](resources.md) + the [roadmap.md](roadmap.md) checklist.

## Definition of Done

- [ ] Routes via `Client`; routing policy + capability flag set.
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
- **Quarterly:** regenerate the local network fixture, diff against [my-network.md](my-network.md);
  new paths/shapes become roadmap items.
- **Capability watch:** when a Network version moves a write onto Integration, flip the routing policy
  + flag (invariant 2). Backend migrations stay invisible to users (no schema change / forced recreate).
