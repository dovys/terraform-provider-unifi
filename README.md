# terraform-provider-unifi

A Terraform provider for UniFi OS consoles. It manages the Network, Protect, and Access apps and the console's own OS settings through one local admin login.

Resources are namespaced by app: `unifi_network_*`, `unifi_protect_*`, `unifi_access_*`, and `unifi_console_*`.

## What it covers

| Area | Resources | Built against | Live-tested | Design notes |
|------|-----------|--------------------|-------------|--------------|
| Network | Networks, WANs and WAN SLAs, WLANs and AP groups, firewall rules and groups, traffic rules and routes, static routes, port forwards, DNS records, dynamic DNS, DHCP reservations, client AP locks, user groups, devices and port profiles, RADIUS, mDNS, threat management, site settings | 10.4.57 | Imports and a subset of writes | [docs/apps/network.md](docs/apps/network.md) |
| Protect | Cameras, NVR settings, lights, sensors, chimes, viewers, live views, automation webhook headers | 7.1.69 | Imports and live view writes | [docs/apps/protect.md](docs/apps/protect.md) |
| Access | Doors, door groups, door guards, policies, holiday groups | 4.2.27 | No, unit tests only | [docs/apps/access.md](docs/apps/access.md) |
| Console | TLS certificate | Not version-pinned | No | [docs/apps/console.md](docs/apps/console.md) |

Read-only data sources cover sites, networks, devices, WAN network groups, application info, Protect cameras, and the Access objects. The full reference is in [docs/resources/](docs/resources/) and [docs/data-sources/](docs/data-sources/), generated from the provider schema.

Network resources run full create, update, import, and destroy lifecycles in acceptance tests against a UniFi controller in Docker. On a live console, existing objects import to a clean plan and a subset of writes is checked with temporary `tfacc-hil-` objects. Protect device config writes (cameras, NVR, lights, sensors, chimes, viewers) are not live-tested yet.

The provider targets recent UniFi releases and aims to cover each app's full configurable surface. It does not enforce a minimum version, but older firmware is untested and unsupported. If a newer release adds something you need, [open an issue](https://github.com/dovys/terraform-provider-unifi/issues).

Out of scope: end-user clients, except two client records keyed by MAC (`unifi_network_dhcp_reservation` and `unifi_network_client_ap_lock`), and the UniFi Integration API (`X-API-KEY`).

## Requirements

- Terraform 1.5 or later. Write-only arguments (`unifi_console_certificate` DNS credentials, `unifi_protect_automation` header values) need Terraform 1.11+ or OpenTofu 1.9+.
- A UniFi OS console reachable from where Terraform runs.
- A local admin account on that console (Settings → Admins → add a local-only admin). No API key is needed. Cloud (UI account) and SSO logins require 2FA, which the provider can't complete, so login fails with an error telling you to use a local admin.

## Install

The provider is not on the Terraform Registry yet, so `source = "dovys/unifi"` resolves only through a local mirror or a dev override.

### From a release

Each [GitHub Release](https://github.com/dovys/terraform-provider-unifi/releases) ships zips for Linux, macOS, and Windows on amd64 and arm64, plus a `SHA256SUMS` file with a detached GPG signature. Drop the zip, unextracted, into Terraform's local plugin directory:

```bash
VERSION=X.Y.Z        # latest from the Releases page
TARGET=darwin_arm64  # or darwin_amd64, linux_amd64, linux_arm64, windows_amd64, windows_arm64
DIR=~/.terraform.d/plugins/registry.terraform.io/dovys/unifi
mkdir -p "$DIR"
curl -L -o "$DIR/terraform-provider-unifi_${VERSION}_${TARGET}.zip" \
  "https://github.com/dovys/terraform-provider-unifi/releases/download/v${VERSION}/terraform-provider-unifi_${VERSION}_${TARGET}.zip"
```

Then run `terraform init` as usual. On Windows the directory is `%APPDATA%\terraform.d\plugins`. See [Provider installation](https://developer.hashicorp.com/terraform/cli/config/config-file#implied-local-mirror-directories) for other layouts.

### From source

Build with Go 1.26.3 or later (the `go.mod` toolchain directive pins 1.26.10, which Go downloads automatically) and point Terraform at the binary with a [dev override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers):

```bash
make build   # go install ./... into $(go env GOBIN), or $(go env GOPATH)/bin
```

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "dovys/unifi" = "/path/to/your/go/bin"
  }
  direct {}
}
```

With an override active, skip `terraform init` and run `plan` or `apply` directly. Terraform prints a warning about the override; that is expected.

## Configure

```hcl
terraform {
  required_providers {
    unifi = {
      source = "dovys/unifi"
    }
  }
}

provider "unifi" {
  host     = "https://192.0.2.1"
  username = var.unifi_username
  password = var.unifi_password

  allow_insecure = true # consoles ship self-signed certs on local IPs
}
```

Every argument can come from an environment variable instead:

| Argument | Environment variable | Default | Notes |
|----------|----------------------|---------|-------|
| `host` | `UNIFI_HOST` | — | A bare host gets `https://`. |
| `username` | `UNIFI_USERNAME` | — | Local admin. |
| `password` | `UNIFI_PASSWORD` | — | |
| `site` | `UNIFI_SITE` | `default` | Network site slug. |
| `allow_insecure` | `UNIFI_ALLOW_INSECURE` | `false` | Skip TLS verification. |
| `timeout` | `UNIFI_TIMEOUT` | `60s` | Per-request timeout, as a Go duration. |
| `max_retries` | `UNIFI_MAX_RETRIES` | `3` | Retries on HTTP 429. |

The provider reuses one login across provider blocks with the same settings and spaces out repeat logins to stay under the console's rate limit. Two environment variables tune this ([details](docs/architecture.md#3-authentication)):

| Environment variable | Default | Effect |
|----------------------|---------|--------|
| `UNIFI_LOGIN_MIN_INTERVAL` | `1s` | Minimum gap between logins. `0` disables it. |
| `UNIFI_DISABLE_CLIENT_CACHE` | unset | `1` logs in fresh for every provider block. |

## Examples

[examples/provider/provider.tf](examples/provider/provider.tf) is the minimal config above. The other directories under [examples/](examples/) are organized by use case (`network/`, `protect/`, `access/`, `console/`, `dns/`, `data/`). [examples/real-home/](examples/real-home/) is a complete home network: WANs, VLANs, Wi-Fi, firewall, DNS, reservations, devices, and the console certificate.

## Safety

Destroying resources is designed not to break a running network.

- Destroying `unifi_network_device` reverts the config Terraform applied and leaves the device adopted. Destroying a Protect device (camera, light, sensor, chime, viewer) removes it from state and leaves it configured. Either is unadopted only with `forget_on_destroy = true`; guard those resources with `lifecycle { prevent_destroy = true }`.
- Other objects that are discovered rather than created, such as Access doors, the NVR, and built-in Network objects, are import and update only. Destroy removes them from state and leaves them as they are.
- Changing an immutable field plans a replacement instead of failing on apply.
- A feature the console doesn't support fails at plan time with a diagnostic, not mid-apply with a raw HTTP error.
- Secrets are marked `Sensitive`.

Details are in [docs/architecture.md](docs/architecture.md#6-safety-defaults).

## How it's built

The API model comes from observing the live UniFi API and from the OpenAPI documents UniFi ships with its apps. It is a clean-room implementation, not derived from other providers.

The Network, Protect, and Access wire types are generated from an API spec that the maintainer keeps outside this repo; the few console types are hand-written. The generated code is checked in, so contributors never regenerate it. Report a missing or wrong field in an issue.

All of the code is written by Claude, an LLM, following [docs/development.md](docs/development.md) and [docs/architecture.md](docs/architecture.md). The design docs are indexed in [docs/README.md](docs/README.md).

## Contributing

An issue with a reproducible config and the request/response pair is the fastest path to a fix. Code PRs must be LLM-authored too. [CONTRIBUTING.md](CONTRIBUTING.md) covers what to include, how to build and test, and the PR checklist.

Every merge to `main` cuts a release ([docs/releasing.md](docs/releasing.md)). Changes are logged in [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE).
