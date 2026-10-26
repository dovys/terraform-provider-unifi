# Tier 3: hardware-in-the-loop tests

Tier 3 runs the provider against a real UniFi OS console. It is the only tier that reaches real firmware: Protect, radios, and firmware-specific behavior that the Docker controller in Tier 2 can't reproduce. The maintainer runs it by hand; a contributor can run it against a console of their own. It is not a CI gate. See [docs/testing.md](../../docs/testing.md) for how the three tiers fit together.

Contributors without a console skip Tier 3 and list what they couldn't test in the PR, per [CONTRIBUTING.md](../../CONTRIBUTING.md).

## Rungs

The tests form a ladder. Each rung has a wider blast radius than the one before it.

| Rung | Runner | Covers | Effect on the console |
| --- | --- | --- | --- |
| A | `go test -run TestHILRead` | Lists every Network collection and Protect surface and decodes them into the wire types. | Read-only |
| B | [`run.sh`](#read-verification-runsh) | Imports live objects and runs `tofu plan`. | Read-only |
| W0 | [`write.sh`](#write-verification-writesh) | Control-plane data such as groups, RADIUS objects, and DNS records. | No dataplane, RF, or hardware effect |
| W1 | [`write.sh`](#write-verification-writesh) | Gateway config: disabled rules and blackhole routes. | Gateway config reload |
| W2 | [`write.sh`](#write-verification-writesh) | AP and switch groups. | APs reprovision; brief management-plane blip |
| W3 | None | Device writes (including `port_overrides`), real networks and enabled WLANs, settings singletons, Protect device writes, `dynamic_dns`. | Not run; see [Verify against hardware](../../TODO.md#verify-against-hardware) |

## Prerequisites

- A Go toolchain. `run.sh` runs `go install ./...`.
- OpenTofu, with `tofu` on `PATH`, for `run.sh`. It calls `tofu plan` and does not support Terraform.
- A `terraform` binary on `PATH` (or `TF_ACC_TERRAFORM_PATH`) for `write.sh`, which drives the Terraform plugin testing framework.
- Network access from your machine to the console's management address.
- A dedicated local UniFi OS admin account. Ubiquiti cloud/SSO accounts hit 2FA and fail with HTTP 499.

## Setup

From the repo root, copy the template. The copy is gitignored.

```bash
cp test/hil/secrets.env.example test/hil/secrets.env
```

Fill in the fields. Both runners source the file.

| Variable | Value |
| --- | --- |
| `UNIFI_HOST` | `https://<your-console-host>` |
| `UNIFI_USERNAME` | The local admin's username. |
| `UNIFI_PASSWORD` | The password, inline or read from a password manager command so it never lands on disk. |
| `UNIFI_ALLOW_INSECURE` | Accepts the console's self-signed certificate. The scripts default it to `true`. |
| `UNIFI_SITE` | Optional. Defaults to `default`. |
| `UNIFI_HIL_RESERVATION_IP` | W0 only: an unused IPv4 address in the default network's DHCP scope. The `dhcp_reservation` test skips without it. |

You can also export these variables yourself instead of using `secrets.env`. `scripts/check-secrets.sh` scans tracked files for leaks; put extra local patterns in the gitignored `scripts/secret-denylist.local`.

## Read verification (run.sh)

`run.sh` sends only GETs and the session-login POST. It has no `apply` or `destroy` path, so it can't change the console's config.

```bash
./test-hil.sh read    # or: make testhil, or: test/hil/run.sh
```

It runs three steps:

1. Builds and installs the provider, then writes `dev.tfrc` with a `dev_overrides` entry so `tofu` uses the local build without a registry or `init`.
2. Runs `TestHILGenerateImports`, which lists live objects and writes an `import {}` block for each one to `imports.tf`.
3. Runs `tofu plan -generate-config-out=generated.tf` and checks for `0 to add, 0 to change, 0 to destroy`.

`run.sh` skips Rung A. To run it on its own:

```bash
set -a; . test/hil/secrets.env; set +a
go test -tags hil ./test/hil -run TestHILRead -count=1 -v
```

### Coverage

`TestHILGenerateImports` emits import blocks for these resources when they exist on the console:

- Network: `dns_record`, `firewall_group`, `firewall_rule`, `user_group`, `wlan`, `wlan_group`, `ap_group`, `network`, `wan` (a `networkconf` with `purpose=wan`), `wan_sla`, `dhcp_reservation` (users with `use_fixedip=true`), and `client_ap_lock` (users with `fixed_ap_enabled`).
- Protect, when installed: `camera`, `liveview`, and the `nvr_settings` singleton. A console without Protect skips these.

Some resources can't reach a clean plan in a plan-only flow, so the import test leaves them out:

- `radius_profile` and PPPoE WANs carry write-only secrets (`auth_servers[].x_secret`, `x_wan_password`). `-generate-config-out` writes them as null, so the plan always shows a change. They import cleanly with a hand-written config that supplies the secret. DHCP and static WANs are covered.
- `device` reports runtime telemetry (`ip`, `state`) that changes on every read, so the plan shows it as known after apply. Rung A covers device decoding.
- `unifi_network_settings` is a singleton whose blocks are all optional, so an empty generated config would plan clean without testing anything.

### Reading the result

If the plan shows changes for a covered resource, the provider's Read or normalization doesn't round-trip a field from live firmware. That is a provider bug, not a harness failure. `plan.out` holds the diff and `generated.tf` holds what the provider read. Open an issue with the field and firmware version, or fix the Read.

## Write verification (write.sh)

`write.sh` covers what import can't: resources with no live objects to import, and write behavior such as sparse PUTs and the controller's server-side merge. It writes to the console.

Never point `write.sh` at a production console. Use one you can afford to disrupt: W1 reloads the gateway config and W2 reprovisions APs.

```bash
./test-hil.sh write            # all write rungs, W0 then W1 then W2
./test-hil.sh write W0         # one rung: W0, W1, or W2
./test-hil.sh write W0 --yes   # skip the confirmation prompt
```

`test/hil/write.sh` takes the same arguments. Each object goes through create, update one field, import with `ImportStateVerify`, a second plan that must be empty, then destroy and `CheckDestroy`. The script sweeps leftover test objects before and after the tests, and the run log goes to `write.out`.

### Write rungs

The tests live in `write_<rung>_hil_test.go`.

| Rung | Resources | Why it is inert |
| --- | --- | --- |
| W0 | `firewall_group`, `user_group`, `radius_profile` (with `x_secret`), `radius_account` (with `x_password`), `dns_record`, `protect_liveview`, `dhcp_reservation` | Nothing references the object. The DNS name is fake, the liveview has no camera, and the reservation MAC is an unused, locally administered one. |
| W1 | `static_route`, `port_forward`, `traffic_rule`, `traffic_route`, `firewall_rule`, `port_profile` | `enabled = false`. The static route is a `192.0.2.0/24` blackhole, the port forward targets `192.0.2.10`, and no device port uses the `port_profile`. |
| W2 | `wlan_group`, `ap_group` | The member list is empty. |

W1 finds a free `firewall_rule` index in the controller's user range and picks a WAN-purpose network as the `traffic_route` egress, so it needs no extra config.

### Safety

Every test object is:

- Additive: the tests never modify an existing object.
- Inert: disabled rules, blackhole routes on RFC 5737 `192.0.2.0/24`, and unreferenced groups have no effect on traffic.
- Named with the `tfacc-hil-` prefix, so the sweep can find it.

Five gates guard the writes:

1. The `//go:build hil` tag keeps the tests out of `make test` and CI.
2. `UNIFI_HIL_ALLOW_WRITES=1` must be set.
3. `UNIFI_HIL_WRITE_HOST` must equal `UNIFI_HOST`.
4. `write.sh` asks you to type `I understand`.
5. `assertInert` rejects any config that lacks the `tfacc-hil-` name or its rung's inertness tokens, such as `enabled = false` or the blackhole prefix. The `dhcp_reservation` test also checks that its MAC isn't an active client, and the `port_profile` test checks after each apply that no device port uses the profile.

`write.sh` sets gates 2 and 3 itself. They protect direct `go test` runs, which must set both:

```bash
UNIFI_HIL_ALLOW_WRITES=1 UNIFI_HIL_WRITE_HOST="$UNIFI_HOST" TF_ACC=1 \
  go test -tags hil ./test/hil -run TestHILWrite_W0 -count=1 -v
```

`TestHILVerifyDHCPGuard` (`verify_dhcpguard_hil_test.go`) is a separate write test that `write.sh` doesn't run. It creates a `tfacc-hil-` network on `192.0.2.0/24` with DHCP off and deletes it when the test ends. It uses the same gates; the command is in the file header.

W3 is never run here. [Verify against hardware](../../TODO.md#verify-against-hardware) tracks the writes that still need a disposable console.

## Cleanup

`write.sh` sweeps before and after each run. To sweep by hand:

```bash
set -a; . test/hil/secrets.env; set +a
go test -tags hil ./test/hil -sweep=default -timeout 10m
```

The sweepers delete only objects named with `tfacc-hil-`. `dns_record` has no sweeper, so delete a leftover `tfacc-hil-` DNS record by hand. `TestHILVerifyDHCPGuard` has no sweeper either; if its cleanup fails, delete the `tfacc-hil-` network by hand.

The runners leave these files in `test/hil/`. All are gitignored because they hold real object IDs from your console. Delete them when you're done:

```bash
rm -rf test/hil/{imports.tf,generated.tf,plan.out,write.out,dev.tfrc,.terraform,.terraform.lock.hcl} test/hil/*.tfstate*
```
