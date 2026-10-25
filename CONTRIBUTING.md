# Contributing

This is a clean-room Terraform provider for UniFi OS. All of its code is written by an LLM working from the project docs, so the most useful contribution is usually a precise issue that lets us reproduce the problem without access to your network.

## Issues

Open bugs and feature requests with the issue forms. A bug report needs:

- Provider version, Terraform or OpenTofu version (1.5+), console hardware, and the UniFi OS, Network, Protect, and Access versions you run.
- A minimal config that reproduces the problem.
- What you expected, what happened, and the full Terraform output.
- The request the provider sent and the controller's response, if you can get them. Run with `TF_LOG_PROVIDER=debug` and grep the output for `wire HTTP`. The provider masks password, passphrase, PSK, and token fields, but check the output yourself and redact API keys, passwords, PSKs, MACs, and public IPs before you paste it.

For a feature, describe the UniFi feature, where it lives in the UI, and which fields you need.

The provider authenticates as a local UniFi OS admin (username and password). It does not use API keys or the Integration API, so a report that depends on either is out of scope.

## Security issues

Don't open a public issue. Report it privately through [GitHub's advisory form](https://github.com/dovys/terraform-provider-unifi/security/advisories/new).

## Ground rules

- **Clean-room.** Don't copy code from other UniFi providers or client libraries. The API model comes from observing the live API and the OpenAPI documents UniFi ships.
- **Generated wire types.** The types in `internal/unifi/*/wire/types.gen.go` are generated from a version-pinned API spec that is not public. Never hand-edit them, and don't run `go generate`, which needs that spec. If a field is missing or mistyped, open an issue or say so in your PR, and the maintainer will regenerate.
- **Never write to a production controller.** Tier 2 and `test/hil/write.sh` create and destroy objects, so point them only at a controller you can afford to break.
- **No private data in tracked files.** `scripts/check-secrets.sh` runs in `make check` and CI and rejects private IP ranges (10/8, 100/8, and 192.168/16 outside `examples/`). Use documentation ranges in tests and docs: 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24, and 2001:db8::/32.
- **LLM-authored code.** Contributed code is no exception: produce it with an LLM working from [docs/development.md](docs/development.md) (invariants, recipe, Definition of Done) and [docs/architecture.md](docs/architecture.md).

## Build

You need Go, the Terraform CLI, golangci-lint v2 (CI uses v2.12.2), and Docker for acceptance tests. `go.mod` pins the Go toolchain, and Go fetches it, govulncheck, and tfplugindocs automatically.

```sh
make build   # go install ./...
```

To try a local build, point Terraform at it with a dev override, as shown in [README.md](README.md#from-source).

## Tests

Tests run in three tiers. The tier is set by package, build tag, and file suffix.

| Tier | Command | Needs | Where tests live |
|---|---|---|---|
| 1, unit | `make test` (`./test-ci.sh`) | nothing | beside the code |
| 2, acceptance | `make testacc` (`./test-extended.sh`) | Docker | `test/acc`, `//go:build acc`, `*_acc_test.go` |
| 3, hardware-in-the-loop | `make testhil` (`./test-hil.sh`) | a live console | `test/hil`, `//go:build hil`, `*_hil_test.go` |

Tier 2 boots a UniFi Network controller in Docker and runs create, update, import, and destroy against it:

```sh
./test-extended.sh                  # boot if needed, run every suite
./test-extended.sh -run TestAccFoo  # one suite
./test-extended.sh up               # boot and seed, run nothing
./test-extended.sh down             # tear down
./test-extended.sh --fresh          # tear down, boot fresh, run
```

The Docker controller covers the Network app's legacy and v2 APIs only. Protect, Access, radios, device adoption, and zone-firewall enforcement can't be exercised there, and some suites skip for that reason. Tier 3 needs a live console you control and a local admin account (see [test/hil/README.md](test/hil/README.md)). `run.sh` is read-only: it imports and plans, sending only GETs. `write.sh` creates and destroys objects and is opt-in. Without a console, skip Tier 3 and list what you couldn't test in your PR.

### Writing tests

A bug isn't fixed until a test covers it. Wire types are regenerated whenever the provider moves to a newer UniFi version, which can silently undo a fix that has no test.

- Each fix adds a regression test that fails before the fix and passes after.
- Test through the resource: create, update, or delete it and assert what the user sees. Unit tests can also cover the config-to-wire-to-state round-trip and validators. Don't assert internals such as HTTP call counts or whether an update sends a sparse PUT.
- Add tests; don't edit existing assertions to make a fix pass. If an existing test pinned the buggy behavior, replace it with a new resource-level test and say why in the PR.

See [docs/testing.md](docs/testing.md) for the full conventions.

## Adding a resource

1. Confirm which API the object lives on (legacy `/rest` or v2) and its JSON shape, against the Tier 2 controller (`./test-extended.sh up`) or with browser devtools on the UniFi UI.
2. Map the Terraform model to the generated `wire.<Type>`. If the wire type lacks a field, stop and say so in the issue or PR (see Ground rules). Add the group accessor and collection in `internal/unifi/<app>`. Resources never make HTTP calls themselves.
3. Schema:
   - Nested attributes.
   - Map or Set keyed by a natural key, not List.
   - `Optional` + `Computed` knobs with `UseStateForUnknown`.
   - `RequiresReplace` on immutable fields.
   - Validators that match what the controller accepts, no tighter.
   - `Sensitive` on secrets.
4. CRUD:
   - Read reconciles state with the controller and removes the resource from state on 404.
   - Update sends only the fields the provider owns. The legacy PUT merges server-side, so never echo back the full GET.
   - Document the import ID.
5. Add a real-world example under `examples/<use-case>/` and run `make docs`.
6. Update `docs/apps/<app>.md` and the [docs/roadmap.md](docs/roadmap.md) checklist.

A resource is done when it has unit tests and a Tier 2 lifecycle: create, update, import with `ImportStateVerify`, and destroy with `CheckDestroy`. A re-plan right after import must show no changes. Name test objects with a `tfacc-` prefix and register a sweeper that deletes leftovers. If the resource depends on hardware, add a HIL test; the maintainer checks that the live network imports to a clean plan before merge.

## Branches and releases

- New resources, data sources, and behavior: branch off `next` and open the PR against `next`.
- Bug fixes and docs, CI, or dependency changes: branch off `main` and open the PR against `main`.

Releases are automatic. Every merge to `main` cuts one: a fix is a patch release, and merging `next` into `main` is a minor release. Don't tag or run `goreleaser`. See [docs/releasing.md](docs/releasing.md).

## Pull requests

Before you open a PR:

- Run `make check`. It runs build, vet, the secret scan, gofmt, `go mod tidy`, govulncheck, `terraform fmt` on examples, the docs-freshness check, golangci-lint, and unit tests. CI may skip unit tests on your PR, so `make check` is the gate. CI also lints the `acc` and `hil` build tags, which `make check` doesn't: run `golangci-lint run --build-tags=acc,hil ./...` too. `make fmt` fixes formatting.
- Run `./test-extended.sh` and paste its printed summary into the PR description. CI doesn't run Tier 2, so a PR without the summary hasn't cleared the bar.
- Add a `CHANGELOG.md` entry under today's date, newest first.
- Fix any govulncheck finding in the same PR, including ones you didn't introduce: raise the `toolchain` line in `go.mod` for a stdlib advisory, or bump the module for a dependency.
- Prefix commit and PR titles with the area, such as `docs:` or `fix(protect):`.

If you edit CI, pin third-party actions to a full commit SHA and container images by digest.

## License

Contributions are released under the [MIT License](LICENSE).
