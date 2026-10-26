# Testing

The provider has three test tiers. Each one has a script at the repo root. Never run any of them against a production console.

| Tier | Script | Target | Covers | Who can run it |
|------|--------|--------|--------|----------------|
| 1, unit | `./test-ci.sh` | none | capability logic, wire (de)serialization against `httptest`, validators, plan modifiers, the v2 write contract | anyone |
| 2, acceptance | `./test-extended.sh` | Docker controller | create, update, import and destroy for each legacy and v2 resource | anyone with Docker |
| 3, hardware-in-the-loop (HIL) | `./test-hil.sh` | a live console you control | decode and import against real firmware, write paths that need real gear | maintainer, or anyone with a spare console |

Don't claim coverage a tier doesn't have. Tier 2 does not cover Protect, Access, real radios, device adoption, or firmware quirks the Docker controller lacks; only Tier 3 reaches those. Tier 3 covers decode, import and additive writes today; device adoption and forget wait on a disposable rig.

## Prerequisites

| Tier | Needs |
|------|-------|
| 1 | Go at the `go.mod` toolchain version |
| 2 | Go, Docker with Compose v2, a `terraform` binary on `PATH` (or `TF_ACC_TERRAFORM_PATH`), port `127.0.0.1:8443` free, about 12 minutes for the first boot |
| 3 | Go, OpenTofu (`tofu`) on `PATH` for the read checks, a `terraform` binary (or `TF_ACC_TERRAFORM_PATH`) for the write checks, network access to a console, and a local UniFi OS admin account. SSO accounts with 2FA fail login with HTTP 499. |

If you can't run Tier 3, skip it and list what you couldn't test in the PR, as [CONTRIBUTING.md](../CONTRIBUTING.md) describes.

## What CI runs

CI is one `checks` job on GitHub-hosted `ubuntu-latest` (`.github/workflows/test.yml`). It runs on every PR, is the required status, and waits for maintainer approval on fork PRs. It lints with `--build-tags=acc,hil`, so the Tier 2 and Tier 3 packages compile, and runs Tier 1 only on PRs into `main`. The full check list and its local equivalents are in [releasing.md](releasing.md#pr-checks).

CI never runs Tier 2 or Tier 3. Run `./test-extended.sh` before you open a PR and paste the summary it prints into the PR description.

## Layout

Each test's tier comes from its package, build tag and file suffix, never from a `-run` name filter. A name filter drops a suite without warning when someone mistypes a prefix.

| Tier | Package | Build tag | File |
|------|---------|-----------|------|
| 1 | the package under test (`internal/provider`, `internal/unifi/...`) | none | `<feature>_ci_test.go` |
| 2 | `test/acc` (`package acc`) | `//go:build acc` | `<feature>_acc_test.go` |
| 3 | `test/hil` (`package hil`) | `//go:build hil` | `<feature>_hil_test.go` |

One unit test breaks the suffix rule: `internal/unifi/protect/wire/flextime_test.go`.

Unit tests are white-box and sit beside the code. Acceptance and HIL tests are black-box: they drive the provider through the Terraform protocol. Both share `internal/acctest`, which holds the provider factory, the env precheck, an env-built client, and the sweeper and `CheckDestroy` helpers. `go test ./...` builds only unit tests; `-tags acc` or `-tags hil` selects a tier.

## Tier 1: unit

```sh
./test-ci.sh                          # go test ./... -timeout 120s
./test-ci.sh -run TestFoo ./internal/provider/...   # extra flags go to go test
```

## Tier 2: acceptance

### The stack

UniFi OS Server needs systemd, `SYS_ADMIN` and host cgroups, so it can't run in an unprivileged container. Tier 2 uses the standalone Network Application instead, which serves the same legacy and v2 surfaces where almost every write lives.

`test/tier2/compose.yaml` defines three containers, with images pinned by digest:

- `mongo`, seeded by `init-mongo.js` with the `unifi` and `unifi_stat` users the controller needs.
- `lscr.io/linuxserver/unifi-network-application`, the standalone controller.
- an nginx bridge (`unifi-os-bridge.conf`), bound to `127.0.0.1:8443`.

The provider speaks the UniFi OS console dialect: `/api/auth/login`, a `/proxy/network/` prefix, and CSRF. The standalone controller uses `/api/login` and bare `/api/...` and `/v2/...` paths. The bridge rewrites the login path, strips `/proxy/network`, and drops the `Secure` cookie flag so the session works over plain HTTP. The provider talks to the bridge as if it were a console.

### Running it

```
./test-extended.sh                  boot the stack if needed, run every acceptance test, print a summary
./test-extended.sh -run TestAccFoo  pass extra go test flags (here, one test)
./test-extended.sh up               boot and seed the stack, run no tests
./test-extended.sh down             remove the stack and its volumes
./test-extended.sh --fresh [...]    remove the stack, then boot fresh and run
```

The script sets `TF_ACC=1` and `UNIFI_T2_MANAGE=1` and runs `go test -tags acc ./test/acc/ -p 1 -count=1 -timeout 60m`. Tests run one at a time against the single controller.

With `UNIFI_T2_MANAGE=1`, `TestMain` in `test/acc` manages the stack (Compose project `tfunifi`):

1. Boots the stack if it is down.
2. Waits up to 12 minutes for the controller's `/status` and for the default site to appear in Mongo.
3. Inserts a local admin straight into Mongo with a pre-hashed `x_shadow`, since the standalone image has no headless way to add one.
4. Logs in once to check the admin works.
5. Exports `UNIFI_HOST`, `UNIFI_USERNAME`, `UNIFI_PASSWORD`, `UNIFI_SITE` and `UNIFI_ALLOW_INSECURE` for the tests.

The stack stays up after a run so reruns start fast. Don't recreate the controller container after seeding: its `/config` re-initializes and the admin disappears. Use `--fresh` instead.

To run against a controller you started yourself, leave `UNIFI_T2_MANAGE` unset and set `TF_ACC=1`, `UNIFI_HOST`, `UNIFI_USERNAME` and `UNIFI_PASSWORD`. `UNIFI_SITE` defaults to `default`; set `UNIFI_ALLOW_INSECURE=true` for a self-signed certificate.

### Sweepers and naming

Every object a test creates must have a name that starts with `acctest.NamePrefix` (`tfacc-`). Register a sweeper with `acctest.RegisterSweeper`; it skips anything without the prefix, so it can never delete an object it didn't create. Each test also sets a `CheckDestroy` that asserts the object is gone, except for the deviations under Coverage.

To sweep leaked objects by hand, export the connection env first. `TestMain` exports it only under `UNIFI_T2_MANAGE=1`, which a sweep doesn't use. For the Docker stack, the credentials are fixed in `test/acc/harness_test.go`:

```sh
UNIFI_HOST=http://localhost:8443 UNIFI_USERNAME=tfadmin UNIFI_PASSWORD=tfadmin-pw-12345 UNIFI_ALLOW_INSECURE=true \
  go test -tags acc ./test/acc -sweep=default
```

### Coverage

These resources run the full lifecycle: create, update, import with `ImportStateVerify`, and destroy, with a clean plan after each step, a second empty plan, `CheckDestroy` and a sweeper.

`network`, `dns_record`, `firewall_group`, `firewall_rule`, `static_route`, `port_forward`, `port_profile`, `user_group`, `radius_profile`, `radius_account`, `traffic_rule`, `ap_group`, `wlan`, `dynamic_dns`

These deviate:

| Resource | Deviation |
|----------|-----------|
| `network_settings`, `threat_management` | Singletons that always exist: destroy only removes state, so there is no sweeper or `CheckDestroy`. The test has no import step: importing a singleton needs a hand-written config (the generated one is empty), so `ImportStateVerify` doesn't apply. |
| `dhcp_reservation` | Client records are never deleted. `CheckDestroy` asserts the fixed IP is cleared, and the sweeper clears the reservation on the test MAC. |
| `mdns` | Singleton: `CheckDestroy` asserts the scope was reset, not that the object is gone. |
| `wan` | Create, import and destroy only. Any WAN update PUT returns `api.err.InvalidPayload`; see [TODO.md](../TODO.md) *Known bugs*. |
| `traffic_route` | Skipped. It needs a VPN or WAN egress network the Docker controller can't provide. Tier 3's W1 write level covers it. |
| `wan_sla` | Skipped. The generated wire type sends the monitor `type` lowercase (`icmp`); the v2 API accepts only `ICMP`, `DNS` or `SPEED_LIMIT`. Remove the skip once that's fixed. |

## Tier 3: hardware-in-the-loop

Tier 3 runs by hand against a live console you control and is not a CI gate. Setup, commands, and safety rails are in [test/hil/README.md](../test/hil/README.md). It has two parts:

- Read verification (`./test-hil.sh read`, the default): decode every Network and Protect collection through its wire type, then import the live objects and expect a clean plan. It sends only GETs and the login POST. A non-empty plan means Read doesn't round-trip a field, which is a provider bug.
- Write verification (`./test-hil.sh write W0|W1|W2|all`): create, update, import and destroy new `tfacc-hil-` objects that have no effect on traffic. It covers resources with nothing to import and checks that sparse updates leave unowned fields intact. It is gated by env vars and a typed confirmation.

W3 writes change existing or shared state (device adoption and port writes, real networks, enabled WLANs, settings singletons, Protect device config). They need a disposable rig and aren't built; [TODO.md](../TODO.md#w3-rig) tracks them.

## When a resource counts as tested

If a resource needs radios or other hardware, it also needs a Tier 3 test, or the PR notes the gap. The full Definition of Done is in [development.md](development.md).

## Test behavior, not mechanics

Test the resource interface and assert what a user sees: the apply succeeds, the field survives, the plan is empty, a controller error surfaces as a clear diagnostic. For the unit layer, that means the config to wire to state round-trip and the validators. Don't assert call counts, request order, the shape of the request body, or which private method ran. Those tests break on a correct refactor and say nothing about whether the resource works.

Each bug fix adds a regression test, written against the interface, that fails before the fix and passes after. It never rewrites an existing test's assertions; if a fix seems to require that, the old test was asserting mechanics. For example: enabling DHCP guard with no trusted server is rejected with a clear error; with a server, it applies. [CONTRIBUTING.md](../CONTRIBUTING.md) states the same rule.
