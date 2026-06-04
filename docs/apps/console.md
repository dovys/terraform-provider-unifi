# UniFi OS console (the `unifi_console_*` surface)

Unlike the per-app clients (Network/Protect/Access), this is the **UniFi OS layer itself** —
unifi-core. Its routes are **baseURL-rooted** under `/api/...` (peers of `/api/system`, `/api/users`,
`/api/console`), **not** under `/proxy/<app>/`. Resources are namespaced `unifi_console_*`.

## Surface: unifi-core `/api/...` over the one cookie session

The console surface runs over the **same cookie+CSRF session** as every app — the classic transport
attaches CSRF to any mutating request regardless of path, so `/api/...` is reached unchanged. **No new
credential**: `username`/`password` reach it, so the "one console session serves everything" principle
holds (invariant 2; no `api_key`). The substruct client is `c.Console()` (`internal/unifi/console`); a
resource calls `c.Console().Certificates()…` and never touches a transport (invariant 1).

Responses are **bare JSON** (no `{meta,data}` / `{code,msg,data}` envelope). Errors come back as a
`{code, level, message}` object — on a non-2xx response, or (for the challenge endpoints) a 2xx body —
which the client maps to a typed `console.ConsoleError` (so resources turn the `code` into an actionable
diagnostic, invariant 3) or to `console.ErrNotFound` (→ remove-on-404).

**Reverse-engineered, clean-room.** This surface is in **no** OpenAPI spec (the vendor specs model the
apps, not the OS). It was modeled from the **observed contract** — the console UI bundle's request/
response schemas — never from reading the vendor's implementation
([../reverse-engineering.md](../reverse-engineering.md)). Because there's no spec to codegen from, the
`wire` types are **hand-written** in `internal/unifi/console` — a documented exception to invariant 2
(generating ~4 tiny structs would mean hand-authoring a synthetic spec). The endpoint map + schemas are
recorded in `../unifi-specs/unifi-os` for reproducibility.

## Capability gate

ACME/cert management depends on firmware. At Configure time the client does one cheap, non-fatal
`GET /api/userCertificates`; success sets `Capabilities.ConsoleCertSupported`. The resource gates on it
in `ModifyPlan` (`requireConsoleCert`) so an unsupported console gets a plan-time message, not a raw 4xx.

## `unifi_console_certificate`

Installs a TLS certificate on the console UI itself, replacing the self-signed default. Two sources:

- **`letsEncrypt`** (default) — native ACME with automated DNS-01. The console calls the DNS provider's
  API *itself* to place the `_acme-challenge` TXT (only the public TXT is needed — no public A record for
  the console). The full generic provider set is modeled: `cloudflare, route53, godaddy, digitalocean,
  porkbun, duckdns, dynu, ovh` (each with its own credential fields) plus `manual` (you place the TXT).
- **`upload`** — bring your own: JSON `POST {name, cert, key}`.

A valid certificate only helps if the name resolves to the console. For a console on a private LAN with
no public A record, pair this with a `unifi_network_dns_record` mapping the certificate's `domain` to the
console's LAN IP (see `examples/console/`).

### Issuance is a singleton chain; renewal is device-owned

Create orchestrates the singleton challenge: `start` → `complete` (the client retries `complete` while
the device reports `DNS_RECORD_NOT_PROPAGATED`, mirroring the UI). A freshly issued or uploaded
certificate is **installed but inactive** — the console keeps serving the old cert until the new one is
activated — so Create then **activates** it (`PUT /api/userCertificates/:id/status {active:true}`) unless
you stage it with `active = false`. After issuance the **device owns renewal** (`/renew` +
`ACME_CERTIFICATE_RENEW_TOO_EARLY`) — Terraform installs the machinery once and never re-issues on a
plain `apply`. Two design consequences:

- **DNS credentials (and the BYO private key) are write-only.** They're secret and/or unreadable from the
  API (List/Get never return the PEM, and the cert object never reports which DNS provider issued it), so
  storing+diffing them would both leak secrets into state and break clean import. Write-only values are
  never persisted and never diffed → a re-apply is an empty plan, and import is clean. There is **no
  imperative re-issue trigger** (that wouldn't be infrastructure-as-code): rotate a token or switch
  provider by recreating the resource (`terraform apply -replace=...`).
- **Renewal drift is silent.** `valid_to`/`serial_number`/`fingerprint`/`cert_version` are pure Computed;
  `Read` refreshes them as the device renews under the same `id`, and because nothing *configured*
  changed, the plan stays empty (invariants 6 & 7).

`certificate` (the BYO public PEM) is the exception — stored and diffed, so it chains declaratively with
an upstream `acme_certificate`/`tls_*` resource (when that PEM changes, the cert is re-uploaded).

**Delete** removes the certificate (`DELETE /api/userCertificates/:id`); when it was the active one the
console reverts to its self-signed default (invariant 4: never left without a usable cert). It deletes
rather than CA-revokes — revocation is a permanent, rate-limited CA action and isn't needed to stop
serving the cert. **Requires Terraform ≥ 1.11 / OpenTofu ≥ 1.9** (write-only arguments).

> **Status: shipped.** `c.Console().Certificates()` over the cookie session; resource
> `unifi_console_certificate` (letsEncrypt + upload, the generic DNS-provider union). The read path and
> the empty-list capability probe are confirmed against a live UCG Max (UniFi OS v5.1.12); the
> `start`→`complete` issuance is verified by a live install (DNS-01 via Cloudflare). See `../testing.md`.
