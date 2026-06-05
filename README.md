# terraform-provider-unifi

A Terraform provider for **UniFi OS** — manage your UniFi console as code. It
covers the **Network**, **Protect**, and **Access** applications, is
app-namespaced (`unifi_network_*`, `unifi_protect_*`, `unifi_access_*`, …), and
tracks the latest UniFi APIs.

The aim is an **up-to-date, feature-complete** provider that models the full
configurable surface of each app — including surfaces like guest networks — and
keeps pace with new UniFi releases. It's built with an unusual process:

- **Every line is written by Claude (an LLM).** Schema, client, tests, and docs
  are all model-authored, following the charter in
  [`docs/development.md`](docs/development.md) and
  [`docs/architecture.md`](docs/architecture.md) — no code is written by hand.
- **Derived from openly published server software.** The API model comes from
  UniFi's published server artifacts and from observing the live controller. It's
  clean-room: the model is derived from UniFi's own software, not from existing
  code.
- **Regenerated, not hand-maintained.** Wire types come from a version-anchored
  spec and are regenerated when we move the provider to a newer UniFi release,
  rather than maintained by hand.

> **Status.** The Network app is built out, usable, and **verified against a live
> console** (every resource imports to a clean plan, and the additive write paths
> pass an end-to-end create→update→import→destroy on real firmware). Protect is
> feature-complete: cameras (`unifi_protect_camera` + data source), the NVR
> singleton, lights, sensors, chimes, viewers, and liveviews. Access is
> feature-complete: door groups, policies, holiday groups, doors, and door
> guards (each with a data source). The Protect and Access surfaces are
> reverse-engineered; the support table below notes what's hardware-verified for
> each (Protect's reads are, its device writes aren't yet; Access has no live
> console on hand). See [`docs/roadmap.md`](docs/roadmap.md) for what's shipped and
> what's next.

## Getting started

### Requirements

- **Terraform 1.5+** (the examples use `import {}` blocks).
- A **UniFi OS console** reachable on your network.
- A **local UniFi OS admin account** (username + password). Cloud accounts are
  not supported.

### Configuration

```hcl
terraform {
  required_providers {
    unifi = {
      source = "dovys/unifi"
    }
  }
}

provider "unifi" {
  host     = "https://192.0.2.1" # your console; or set UNIFI_HOST
  username = var.unifi_username   # local admin; or UNIFI_USERNAME
  password = var.unifi_password   # or UNIFI_PASSWORD

  allow_insecure = true # consoles ship self-signed certs on local IPs
}
```

Every setting also reads from an environment variable, so you can keep
credentials out of your configuration:

| Setting | Env | Notes |
|---------|-----|-------|
| `host` | `UNIFI_HOST` | Console URL/host; a bare host is assumed `https://`. |
| `username` + `password` | `UNIFI_USERNAME` / `UNIFI_PASSWORD` | Local UniFi OS admin (not cloud). |
| `site` | `UNIFI_SITE` | Site slug; defaults to `default`. |
| `allow_insecure` | `UNIFI_ALLOW_INSECURE` | Skip TLS verification (self-signed LAN certs). |
| `timeout` | `UNIFI_TIMEOUT` | Per-request timeout (Go duration, default `60s`). |
| `max_retries` | `UNIFI_MAX_RETRIES` | HTTP 429 retries (default `3`). |

The provider also reuses one authenticated session across identical configs and
throttles repeat logins, so back-to-back runs don't trip the console's login rate
limit (HTTP 429). Two env-only knobs tune this — `UNIFI_LOGIN_MIN_INTERVAL`
(minimum spacing between logins to one console, Go duration, default `1s`) and
`UNIFI_DISABLE_CLIENT_CACHE=1` (disable session reuse) — but the defaults suit
almost everyone. See [`docs/architecture.md`](docs/architecture.md#3-authentication).

See the [`examples/`](examples/) directory for complete, real-world
configurations organized by use case.

## Supported UniFi versions

The provider is modeled against — and tested against — these UniFi application
versions:

| App | Modeled against | Provider support |
|-----|-----------------|------------------|
| [UniFi Network](docs/apps/network.md) | `10.4.57` | ✅ Supported |
| [UniFi Protect](docs/apps/protect.md) | `7.1.69` | ✅ Supported <sup>1</sup> |
| [UniFi Access](docs/apps/access.md) | `4.2.27` | ✅ Supported <sup>2</sup> |

<sup>1</sup> **Protect** — reads are hardware-verified on a live console (Protect
7.1.69 with a camera + NVR import to a clean plan; the `liveview` write round-trips
create → update → import → destroy). The reverse-engineered **device config
writes** — camera/NVR settings and the light/sensor/chime/viewer surfaces (no live
instance) — aren't hardware-verified yet; they need a disposable rig.

<sup>2</sup> **Access** — not yet hardware-verified: there's no live Access console
on hand, so the reverse-engineered read + write surface is HIL-unverified
end-to-end. The resources ship and are registered.

> **We aim at recent UniFi releases and don't support older versions.** The
> client is generated from a version-anchored spec and updated to track current
> UniFi, not every release. Older firmware may happen to work but is neither
> tested nor supported. **If a newer UniFi release adds functionality you need,
> [open an issue](CONTRIBUTING.md)** and we'll model it against that version.

## Local build

Until the provider is on the Terraform Registry, build it and use it through a
[development override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers).

1. Build and install the binary into your Go bin directory:

   ```bash
   make build   # go install ./...
   ```

2. Point Terraform at it with a `dev_overrides` block in your `~/.terraformrc`
   (use your `go env GOBIN`, or `$(go env GOPATH)/bin` if `GOBIN` is unset):

   ```hcl
   provider_installation {
     dev_overrides {
       "dovys/unifi" = "/path/to/your/go/bin"
     }
     direct {}
   }
   ```

3. With a dev override in place, **skip `terraform init`** — just run
   `terraform plan` / `terraform apply`. Terraform prints a warning noting the
   override is active, which is expected.

See [`docs/development.md`](docs/development.md) for the full workflow.

## How it connects

The provider talks to your console over a **single cookie + CSRF session** (the
local admin account). That one session spans every app — the Network **legacy
REST** and **v2** APIs, plus the **Protect** (`/proxy/protect/api`) and
**Access** (`/proxy/access/api`) private APIs — with **no `api_key`** and no
extra credential. Each resource targets whichever surface owns its object —
transparently, with nothing to configure. Details in
[`docs/architecture.md`](docs/architecture.md).

## Development

Requires **Go 1.26.3+** (the `go.mod` floor); CI always runs the latest stable Go.

```bash
make build   # go install
make test    # unit tests (no controller)
make lint    # go vet + golangci-lint
make testacc # acceptance tests — needs a controller; never against production
```

## Contributing

The most valuable contribution is a **detailed issue**, not a pull request — see
[`CONTRIBUTING.md`](CONTRIBUTING.md) for why and how.

## License

Distributed under the [MIT License](LICENSE).
