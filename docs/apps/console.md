# UniFi OS console

The console app covers the UniFi OS layer itself (unifi-core), not an app that runs on it. Resources are named `unifi_console_*`. Today there is one: `unifi_console_certificate`, which installs the TLS certificate the console's management UI serves.

Status: shipped, with `letsEncrypt` and `upload` and every DNS provider listed below.

## Surface

UniFi ships OpenAPI documents for some apps but none for the OS layer. The contract was observed from the console UI's own requests (browser devtools), the provider's `TF_LOG_PROVIDER=debug` wire log ([development.md](../development.md)), and read-only GETs.

Routes sit at the root of the console under `/api/...`, next to `/api/system` and `/api/users`. There is no `/proxy/<app>/` prefix and no site: the surface is global.

The client is `c.Console()` in `internal/unifi/console`. A resource calls `c.Console().Certificates()` and never a transport, and runs over the same cookie + CSRF session as every other app ([invariants](../development.md#invariants) 1 and 2). The classic transport adds CSRF to any mutating request whatever its path, so no new credential is needed.

Responses are bare JSON with no envelope. Errors are a `{code, level, message}` object. It can arrive on a non-2xx response or, for the challenge endpoints, on a 2xx body, so `do` in `core.go` checks every body for a top-level `code`. Success bodies never carry one. A `NOT_FOUND` code or an HTTP 404 becomes `console.ErrNotFound`, and Read drops the resource from state. Every other code becomes a typed `*console.ConsoleError`, which the resource turns into a diagnostic.

### Endpoints

All paths are under `/api/userCertificates`.

| Method | Path | Client method | Used by the resource |
| --- | --- | --- | --- |
| GET | `/` | `List` | Configure-time probe |
| POST | `/` | `Upload` | Create and Update (`upload`) |
| GET | `/:id` | `Get` | Read, Update, after activation |
| DELETE | `/:id` | `Delete` | Delete |
| PUT | `/:id/status` | `SetActive` | Create and Update |
| POST | `/challenge/start` | `StartChallenge` | Create (`letsEncrypt`) |
| POST | `/challenge/complete` | `CompleteChallenge` | Create (`letsEncrypt`) |
| POST | `/challenge/cancel` | `CancelChallenge` | Create, recovery and cleanup |
| GET | `/challenge/current` | `CurrentChallenge` | no |
| POST | `/:id/renew` | `Renew` | no |
| POST | `/:id/revoke` | `Revoke` | no |

`Delete`, `Revoke` and `CancelChallenge` treat "already absent" as success.

### Wire types are hand-written

Every other app uses generated `wire` structs ([Wire types](../development.md#wire-types)). The console's types live in `internal/unifi/console/types.go`, because the OS layer has no OpenAPI to generate from. The surface is a handful of small structs, and writing a synthetic spec just to feed codegen would cost more than the structs.

Field casing differs by direction. Response objects are snake_case (`serial_number`, `valid_to`, `acme_renew_error`). The DNS-provider request union is camelCase (`apiToken`, `accessKeyId`, `consumerKey`). `Certificate.source` is `lets_encrypt` or `uploaded`.

## Capability gate

Certificate management depends on the UniFi OS version. At Configure the client makes one non-fatal `GET /api/userCertificates`. Any success sets `Capabilities.ConsoleCertSupported`, including an empty list on a console still serving its self-signed default.

The resource calls `requireConsoleCert` in `ModifyPlan`. When the probe failed, the plan stops with "Console certificate management is not available" and tells the operator to upgrade UniFi OS or remove the resource.

## `unifi_console_certificate`

The resource replaces the console's self-signed certificate. `source` picks one of two ways to get it:

- `letsEncrypt` (default): native ACME with DNS-01. The console calls the DNS provider's API itself to place the `_acme-challenge` TXT record. Only that public TXT record is needed; the console needs no public A record.
- `upload`: bring your own PEM certificate and key, sent as JSON `{name, cert, key}`.

A valid certificate helps only if its name resolves to the console. On a private LAN, pair it with a `unifi_network_dns_record` that maps `domain` to the console's address. See `examples/console/console.tf`.

### DNS providers

`dns_provider.name` selects the provider. Each needs exactly its own credentials:

| Provider | TF attributes | Wire fields |
| --- | --- | --- |
| `cloudflare`, `digitalocean` | `api_token` | `apiToken` |
| `duckdns` | `token` | `token` |
| `dynu` | `api_key` | `apiKey` |
| `godaddy` | `api_key`, `api_secret` | `apiKey`, `apiSecret` |
| `porkbun` | `api_key`, `secret_key` | `apiKey`, `secretKey` |
| `route53` | `access_key_id`, `secret_access_key`, `region` | `accessKeyId`, `secretAccessKey`, `region` |
| `ovh` | `application_key`, `application_secret`, `consumer_key`, `endpoint` | `applicationKey`, `applicationSecret`, `consumerKey`, `endpoint` |
| `manual` | none | `dnsProvider` omitted |

`region` must be one of the AWS regions the console UI offers. `endpoint` is one of `ovh-eu`, `ovh-us`, `ovh-ca`, `kimsufi-eu`, `kimsufi-ca`, `soyoustart-eu`, `soyoustart-ca`.

### Plan-time validation

`ValidateConfig` enforces which fields go with which source:

- `letsEncrypt` requires `domain` and `dns_provider`, and rejects `certificate` and `private_key`. A wildcard `domain` such as `*.example.com` is allowed.
- `upload` requires `certificate` and `private_key`, and rejects `domain` and `dns_provider`.
- Within `dns_provider`, every credential the provider needs must be set and every other one must be unset.

While `source` is unknown the check assumes `letsEncrypt`; while `dns_provider.name` is unknown it skips the credential check. Write-only credentials read as unknown at plan time, so any non-null value counts as set.

### Issuance

The ACME challenge is a console-wide singleton. Create for `letsEncrypt` runs it in two steps:

1. `challenge/start` with the name, domain, and DNS-provider union. If the console answers `ACME_CHALLENGE_IN_PROGRESS`, the resource cancels the stale challenge and starts again once.
2. `challenge/complete`. While the console reports `DNS_RECORD_NOT_PROPAGATED`, the client retries every 10s, up to 10 attempts (about 100s), as the UI does. The context bounds the loop.

If complete fails, the resource cancels the challenge on a best-effort basis so it doesn't block the next one.

Two certificates applied in parallel collide on the singleton: one cancels the other's challenge. Apply them one at a time.

With `manual`, the operator must publish the TXT record within the retry window. The resource doesn't surface the record's name and value. They come back in the `challenge/start` response, visible in the console UI or the debug wire log.

### Activation

A newly issued or uploaded certificate is installed but inactive, and the console keeps serving its old certificate. Create therefore calls `PUT /:id/status {active: true}` unless `active = false`, which stages the certificate without serving it. Activating one certificate supersedes the one active before it. Update reconciles `active` the same way.

### Renewal belongs to the console

After issuance the console renews the certificate itself under the same `id`, and the active flag survives renewal. Terraform never re-issues on a plain apply, and the resource never calls `Renew`. `valid_to`, `serial_number`, `fingerprint` and `cert_version` are Computed and refresh on every Read, so renewal changes no configured value and the plan stays empty.

`acme_renew_error` holds the code of the console's last failed renewal, such as `ACME_RENEW_CERTIFICATE_FAILED`, and is empty when renewal is healthy. It is the only place a broken renewal shows up.

### Secrets are write-only

`dns_provider` (the whole block) and `private_key` are write-only: Terraform sends them once and never stores or diffs them. The credential fields and `private_key` are also `Sensitive`. Write-only arguments need Terraform 1.11+ or OpenTofu 1.9+. The API never returns these values anyway: List and Get omit the PEM, and the certificate object doesn't record which DNS provider issued it. So a re-apply plans empty.

The PEM and key that the upload and complete responses carry are never written to state. The wire-debug log masks the camelCase DNS credentials by field name and any PEM private key by value. Public certificates stay visible.

There is no re-issue trigger attribute. To rotate a token or switch provider, recreate the resource: `terraform apply -replace=unifi_console_certificate.<name>`.

### Updates and replacement

`name`, `source` and `domain` are `RequiresReplace`; there is no rename API. Two things change in place:

- `active`, through `PUT /:id/status`.
- `certificate` for `upload`. The public PEM is stored and diffed so it can chain from an upstream `acme_certificate` or `tls_*` resource. A changed PEM triggers a new upload, which takes `private_key` from config, so the key must still be set.

### Delete

Delete calls `DELETE /:id`. If the certificate was active, the console falls back to its self-signed default. Deleting an inactive certificate doesn't change what the console serves. The resource doesn't revoke at the CA: revocation is permanent and rate-limited, and the console stops serving the certificate either way.

### Import

The import key is the certificate `id`. Find it with `GET /api/userCertificates` or in the debug wire log. Read maps `source` back from the wire value and fills `domain` from the subject CN.

`dns_provider` and `private_key` are absent after import (see [Secrets are write-only](#secrets-are-write-only)). An imported `upload` certificate shows a one-time re-upload diff, because the console never returns the PEM to compare against.

### Error diagnostics

`addConsoleCertError` maps console codes to diagnostics:

| Code | Diagnostic |
| --- | --- |
| `CALL_DNS_PROVIDER_API_FAILED` | DNS provider rejected the request; check the credentials can edit the zone |
| `ACME_CHALLENGE_IN_PROGRESS` | Another challenge is running; wait or cancel it in the UI |
| `ACME_CHALLENGE_VALIDATION_FAILED`, `ACME_CHALLENGE_ISSUE_FAILED` | Let's Encrypt could not validate the domain |
| `DNS_RECORD_NOT_PROPAGATED` | TXT record not visible before the retry budget ran out |
| `USER_CERTIFICATE_DUPLICATE` | A certificate with this `name` already exists |
| `USER_CERTIFICATE_KEY_MISMATCH` | `certificate` and `private_key` don't match |
| `USER_CERTIFICATE_UNSUPPORTED_TYPE`, `USER_CERTIFICATE_INVALID_CN`, `USER_CERTIFICATE_EXPIRED` | Uploaded certificate rejected |

Any other code, `ACTION_FORBIDDEN` included, surfaces as `Failed to <action>: <code>: <message>`.

## Testing

`internal/unifi/console/console_ci_test.go` unit-tests the client against a fake `doer`. `internal/provider/console_certificate_ci_test.go` unit-tests config validation and the resource logic.

Tier 3 (`test/hil/read_hil_test.go`) lists `/api/userCertificates` on a live console and decodes the result through the hand-written types. Start-to-complete issuance has been checked by hand on a live console.

There is no Tier 2 acceptance test and no `tfacc-` sweeper: the Docker controller runs only the Network application and has no unifi-core surface. No automated test covers the create, update, import and destroy lifecycle; [TODO.md](../../TODO.md#console) tracks the gap.

## Adding a console resource

1. Observe the contract: drive the feature in the console UI with browser devtools open, then confirm shapes with read-only GETs.
2. Add a group to `internal/unifi/console` with an accessor on `Client`, like `Certificates()`. Build paths with `core.apiPath` and send through `do` so error and 404 mapping stay uniform.
3. Hand-write the types in `types.go` in the wire's casing, and add the API's error codes as constants.
4. If older UniFi OS versions lack the surface, add a non-fatal probe to the Configure path in `internal/unifi/client.go`, a flag on `Capabilities`, and a `require…` check called from `ModifyPlan`.
5. Map known error codes to diagnostics in the resource, and mark every credential write-only.
