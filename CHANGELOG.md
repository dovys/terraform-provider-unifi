# Changelog

All notable changes to this provider are documented here. The format loosely
follows [Keep a Changelog](https://keepachangelog.com/); releases are tagged
`vX.Y.Z` and published to the Terraform Registry as `dovys/unifi`.

## [Unreleased]

### Added

- **Phase 0 — base / bootstrap.** The provider scaffold and client foundation:
  - `terraform-plugin-framework` provider (Terraform protocol v6) with a
    configuration schema (`host`, `api_key`, `username`, `password`, `site`,
    `allow_insecure`, `timeout`, `max_retries`) and environment-variable
    fallbacks for each.
  - Dual authentication: a local-admin cookie+CSRF session (`classic`
    transport, with `exp`-driven re-auth and a clear 2FA/cloud-account error)
    and an Integration `X-API-KEY` session (`integration` transport). Both honor
    HTTP 429 with `Retry-After` backoff.
  - A hybrid `internal/unifi` client: the legacy, v2, and Integration surfaces
    sit behind a single `Client` interface, with a capability-aware `router`
    that picks the backend per operation (so a future Integration write is a
    one-line routing change).
  - A `Configure`-time capability probe: Network application version detection
    (legacy `/stat/sysinfo`, with an Integration `application-info` fallback),
    the site name↔UUID map (Integration `/sites`, matched on
    `internalReference`), and version-derived feature flags.
  - Build & release tooling: `GNUmakefile`, GoReleaser config, Terraform
    Registry manifest, `tfplugindocs` tool dependency, golangci-lint config, and
    GitHub Actions for build/lint/unit plus a gated Docker-controller acceptance
    harness and an HIL stub.

No resources or data sources yet — those begin in Phase 1 (core networking).
