# terraform-provider-unifi

A Terraform provider for **UniFi OS** — manage a console (the **Network** app
today; **Protect** later) as code against the latest UniFi APIs. Resources are
app-namespaced (`unifi_network_*`, `unifi_protect_*`, …). Built clean-room.

> **Status: early development.** The base (provider scaffold + client) is in
> place; resources are being built out phase by phase. Not yet published to the
> Terraform Registry. See [`docs/roadmap.md`](docs/roadmap.md).

## How it connects

The provider talks to a console over one cookie+CSRF session (a local admin
account), spanning the legacy REST API and the newer v2 API — each resource
targets whichever surface owns its object, with no change to your configuration.
Details in [`docs/architecture.md`](docs/architecture.md).

A **local UniFi OS admin account** is required (cloud/SSO accounts that force
2FA are rejected with a clear message).

```hcl
terraform {
  required_providers {
    unifi = { source = "dovys/unifi" }
  }
}

provider "unifi" {
  host     = "https://192.0.2.1" # or UNIFI_HOST
  username = var.unifi_username  # local admin; or UNIFI_USERNAME
  password = var.unifi_password  # or UNIFI_PASSWORD

  allow_insecure = true # consoles ship self-signed certs on local IPs
}
```

| Setting | Env | Notes |
|---------|-----|-------|
| `host` | `UNIFI_HOST` | Console URL/host; a bare host is assumed `https://`. |
| `username` + `password` | `UNIFI_USERNAME` / `UNIFI_PASSWORD` | Local UniFi OS admin. |
| `site` | `UNIFI_SITE` | Site slug; defaults to `default`. |
| `allow_insecure` | `UNIFI_INSECURE` | Skip TLS verification. |
| `timeout` | `UNIFI_TIMEOUT` | Per-request timeout (Go duration, default `60s`). |
| `max_retries` | `UNIFI_MAX_RETRIES` | HTTP 429 retries (default `3`). |

## Development

Requires **Go 1.26.3+** (the `go.mod` floor); CI always runs the latest stable Go.

```bash
make build   # go install
make test    # unit tests (no controller)
make lint    # go vet + golangci-lint
make testacc # acceptance tests — needs a controller; never against production
```

Design and contributor docs live in [`docs/`](docs/README.md): architecture,
the resource catalog, the device model, operator flows, the testing tiers, and
the development charter (invariants + Definition of Done). Distributed under
[MIT](LICENSE).
