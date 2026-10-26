# Architecture: API surfaces, client, auth and safety

How the provider talks to a UniFi OS console: one authenticated session, a client per app, and the write rules each surface needs. Read this before changing `internal/unifi` or adding a resource. The workflow and Definition of Done are in [development.md](development.md); per-app surfaces are in [apps/](apps/).

## 1. API surfaces

Every request goes to one host over one cookie + CSRF session, logged in as a local UniFi OS admin. All surfaces are undocumented internal APIs that the UniFi web UI uses. The provider learns them by observing a live console and from the OpenAPI documents UniFi ships with some apps.

| App | Base path | Body | Client |
|-----|-----------|------|--------|
| Network, legacy | `/proxy/network/api/s/<site>/rest/<type>`, `/stat/...`, `/cmd/...`, `/get/setting/<key>`, `/set/setting/<key>`; sites at `/proxy/network/api/self/sites` | `{meta:{rc,msg},data:[]}` | `c.Network()` |
| Network, v2 | `/proxy/network/v2/api/site/<site>/<path>` | bare JSON | `c.Network()` |
| Protect | `/proxy/protect/api/<name>` | bare JSON | `c.Protect()` |
| Access | `/proxy/access/api/v2/...` | `{code,msg,data}` | `c.Access()` |
| Console (UniFi OS) | `/api/...`, e.g. `/api/userCertificates` | bare JSON | `c.Console()` |

Resources are app-namespaced: `unifi_network_*`, `unifi_protect_*`, `unifi_access_*`, `unifi_console_*`. The only UniFi OS surface modeled today is the console TLS certificate (`/api/userCertificates`, ACME and BYO upload), as `unifi_console_certificate`; see [apps/console.md](apps/console.md).

Network's legacy surface is the widest and most stable, and carries most writes. v2 covers newer features: traffic rules and routes, QoS rules, static DNS, AP groups, WAN SLAs, WAN Magic, load balancing and network groups, mDNS (`global/config/network`), and the feature list. v2 is a URL prefix on the same session, not a separate transport.

Out of scope:

- The Integration API (`/integration/v1`, `X-API-KEY`). It would add a second auth mechanism, and a local admin session already reaches everything the provider writes.
- The cloud Site Manager API (`api.ui.com`), which is read-only.
- Talk, Connect and InnerSpace.

## 2. Client

### Package map

```
internal/unifi/
├── client.go         Client interface, NewClient, client cache
├── capabilities.go   Configure-time probe (Network version, Protect, console certs)
├── sites.go          site read and management
├── surfaces.go       legacy envelope decode, ErrNotFound
├── paths.go          legacy path builder for console-level calls
├── errors.go         CapabilityError, ConfigError
├── transport/
│   ├── classic.go       cookie + CSRF session, login, re-auth
│   ├── jwt.go           TOKEN claims (exp, csrfToken)
│   ├── loginthrottle.go per-host login spacing
│   └── transport.go     APIError, 429 backoff, wire logging and redaction
├── network/          core.go (session + path builders), collection.go, one file per group, wire/
├── protect/          core.go, collection.go (PATCH), one file per group, wire/
├── access/           core.go ({code,msg,data} envelope), one file per object, wire/
└── console/          core.go, certificates.go, types.go (hand-written)
```

### How a resource reaches the API

1. The provider's `Configure` calls `unifi.NewClient`, which logs in and probes capabilities.
2. A resource's `Configure` calls `clientFromProviderData` to get the `unifi.Client`.
3. CRUD goes through an app accessor and a group or collection, e.g. `c.Network().WAN().SLAs()`, `c.Network().DNS().Records()`, `c.Protect()`, `c.Access()`, `c.Console()`.

App clients are cheap and stateless: each accessor call builds a new one around the shared session and the configured site. Resources never touch the transport.

### Wire types

Object types are generated Go structs in each app's `wire/types.gen.go`, except Console's hand-written `console/types.go`. The hand-written layer is the groups and collections, never per-object structs. Generation and the no-hand-edit rule are in [development.md](development.md#wire-types).

### Collections

Network has two generic collections, one per surface:

| Collection | Surface | Get | Update | Delete |
|------------|---------|-----|--------|--------|
| `LegacyCollection[T]` | `/rest/<type>` | by id | `Update(ctx, id, body)`; id goes in the URL only. Re-GETs when the PUT echoes nothing. | not-found counts as success |
| `V2Collection[T]` | `/v2/.../<path>` | none; List and filter | `Update(ctx, id, body)`; caller puts `_id` in the body (see §4) | not-found counts as success |

Write a custom collection only when the surface needs it:

| Need | Example |
|------|---------|
| No GET-by-id, so Read lists and filters | v2 objects, `RecordCollection` (static DNS) |
| Import by a natural key | `NetworkCollection.GetByName`, `SLACollection.GetByName` |
| `_id` enforced in the v2 PUT body | `RecordCollection.Update`, `SLACollection.Update` |
| Id taken from the object | `RoutingCollection` |
| Non-REST verbs | `SettingCollection` (`get/setting`, `set/setting`) |
| Replace-style document | `GlobalNetworkService` (mDNS) |
| Array the controller replaces wholesale | device `port_overrides` in `devices.go` |

Protect uses one `Collection[T]` over PATCH. Access and Console each have their own small core (see §4).

### Read and refresh

Read refreshes Computed fields, rewrites a configured value only when the controller's differs after normalization, and removes the resource from state on 404. `UseStateForUnknown()` keeps an `Optional+Computed` field's prior value so it doesn't turn `known after apply` when a sibling changes. Drift still shows, because Read runs every cycle. Telemetry is Computed only.

### Per-resource surface (Network)

| Resource | Read | Write |
|----------|------|-------|
| network, WAN (also the network and networks data sources) | legacy `/rest/networkconf` | legacy |
| WAN SLA | v2 `/wan-slas` | v2 |
| WAN network groups (data source) | v2 `/wan/networkgroups` | none |
| WLAN, WLAN group | legacy `/rest/wlanconf`, `/rest/wlangroup` | legacy |
| AP group | v2 `/apgroups` | v2 |
| device | legacy `/stat/device` (also `data.unifi_network_device`) | adopt: `/cmd/devmgr`; config: sparse `PUT /rest/device/<id>`, `port_overrides` GET-merged |
| port profile | legacy `/rest/portconf` | legacy |
| firewall group, firewall rule | legacy `/rest/firewallgroup`, `/rest/firewallrule` | legacy |
| traffic rule, traffic route | v2 | v2 |
| static route, port forward, dynamic DNS | legacy `/rest/{routing,portforward,dynamicdns}` | legacy |
| local DNS record | v2 `/static-dns` | v2 |
| RADIUS profile, RADIUS account, user group | legacy `/rest/{radiusprofile,account,usergroup}` | legacy |
| DHCP reservation, client AP lock | legacy `/rest/user` (`use_fixedip`; `fixed_ap_*`) | legacy, find-or-create by MAC |
| settings | `get/setting/<key>` | `POST set/setting/<key>` (shallow merge) |
| threat management | `get/setting/ips` | `set/setting/ips`; gated on the v2 feature list |
| mDNS | v2 `global/config/network` | v2, replace-style |
| application info (data source) | Configure-time probe (`/stat/sysinfo`) | none |
| site (data source) | `/self/sites` | none |

The client also has site create, update and delete (`/cmd/sitemgr`), but no resource uses them yet. Protect, Access and Console resources are listed in [apps/protect.md](apps/protect.md), [apps/access.md](apps/access.md) and [apps/console.md](apps/console.md).

## 3. Authentication

The provider needs a local UniFi OS admin account. SSO and cloud accounts don't work. The username and password open the one session that every app uses.

### Provider configuration

| Attribute | Env var | Default |
|-----------|---------|---------|
| `host` | `UNIFI_HOST` | required; a bare host gets `https://` |
| `username` | `UNIFI_USERNAME` | required |
| `password` (sensitive) | `UNIFI_PASSWORD` | required |
| `site` | `UNIFI_SITE` | `default`; the site slug (internal name), not the display name |
| `allow_insecure` | `UNIFI_ALLOW_INSECURE` | `false`; only skips TLS verification |
| `timeout` | `UNIFI_TIMEOUT` | 60s |
| `max_retries` | `UNIFI_MAX_RETRIES` | 3 |

`Configure` logs in and runs the capability probe, so bad credentials fail there.

### Session lifecycle

`transport.Classic` owns the session. On each `Do`:

1. If there is no session, log in: `POST /api/auth/login` with `rememberMe: true`. The response sets the `TOKEN` cookie, a JWT. The lifetime comes from its `exp` claim, never a hard-coded value. Login retries on 429.
2. If the token expires within 60 seconds (`reauthSkew`), log in again first.
3. On a mutating method, send the CSRF token. It comes from the `X-Updated-Csrf-Token` or `X-Csrf-Token` response header, or else from the JWT's `csrfToken` claim. A rotated `X-Updated-Csrf-Token` on any response replaces the stored one.
4. On 401 or 403, log in again and retry once.
5. On 429, back off and retry up to `max_retries` times. The wait is `Retry-After` in seconds when the header is present, else 500ms doubling up to 10s.

### Login errors

| Error | Cause | What the user sees |
|-------|-------|--------------------|
| `TwoFactorError` | HTTP 499, `api.err.Ubic2faTokenRequired` (cloud or 2FA account) | use a local admin account |
| `AuthError` | bad credentials | authentication failed |
| `ConfigError` | missing host, username or password | which setting is missing |

`describeClientError` in `internal/provider/provider.go` turns these into diagnostics.

### Login rate limiting

The controller rate-limits `/api/auth/login` and answers 429 after a few attempts. Two guards keep the provider under the limit:

- Client cache: `NewClient` keeps a process-wide cache keyed by a SHA-256 over host, username, password, site, `allow_insecure`, timeout and `max_retries`. Identical configs share one session and one capability probe, so an apply logs in once, and repeated imports or acceptance-test providers don't log in again. Set `UNIFI_DISABLE_CLIENT_CACHE=1` to turn it off.
- Login throttle: logins to the same host are spaced at least 1s apart; the first never waits. Set `UNIFI_LOGIN_MIN_INTERVAL` to a Go duration to change it, or `0` to disable it.

## 4. Write contract per surface

Each app's collection enforces its own body rules.

### Network sparse writes

A legacy `/rest` PUT shallow-merges server-side: fields you omit keep their stored value. So send only the fields the provider owns, and never echo the GET document back.

The merge is top-level only. The controller replaces a nested object or array wholesale; PUTting one key of `wan_provider_capabilities` drops its sibling. Send a nested value only if the resource owns every key or element in it.

Optional generated fields are `omitempty` pointers, so an unset field and an explicit `false` or `0` stay distinct. A null or unknown attribute becomes `nil` and is omitted (`internal/provider/common.go`); a set value, including `false` or `0`, is sent. Required fields are plain values and are always sent.

Two writes are exceptions to sparse PUTs. v2 `global/config/network` (used by `unifi_network_mdns`) validates the whole document on PUT, so `GlobalNetworkService` GETs the live document, overlays the owned fields, and PUTs it back. Device `port_overrides` is an array the controller replaces wholesale, so the device collection GETs the live array, overlays the owned entries, and PUTs it back.

### Network v2

- Put `_id` in both the URL and the body on PUT, or the controller returns 500. `V2Collection.Update` leaves the body to the caller, so a group's collection enforces it (`RecordCollection.Update`, `SLACollection.Update`).
- Treat any 2xx as success. Some deletes return 204 with no body.
- Don't send empty strings for fields you don't set. Generated pointers drop `nil`, but an explicit `""` is still sent.

### Protect

Writes are a PATCH with an owned-field overlay: a small struct with `omitempty` tags that holds only the modeled fields. Most generated Protect fields have no `omitempty`, so marshaling a read object would send every field, including telemetry, and overwrite controller state. PATCH merges top-level fields, so no read is needed first. The resource merges nested sub-objects that the controller replaces wholesale.

### Access and Console

Access wraps every response in `{code,msg,data}`; `access/core.go` unwraps it and maps not-found messages to `ErrNotFound`. Console calls are console-wide, not site-scoped, and return bare JSON with their own error shape (`ConsoleError`).

### Errors and not-found

`transport.APIError` carries the HTTP status and the controller's message from `meta.msg` or `message`. A legacy body with `meta.rc != "ok"` is an error even on HTTP 200.

Not-found arrives either as HTTP 404 or as `rc=error` with an `UnknownObject`, "not found" or "no such" message. Each app package maps both to its own `ErrNotFound`. Resources check with `isNotFoundErr` in `internal/provider/common.go`, which matches every package's sentinel.

## 5. Capability gating

A capability the console lacks must surface at plan time as a diagnostic, never as a raw 4xx during apply. Gate on capability, not firmware version.

`NewClient` probes once and caches the result in `unifi.Capabilities`:

| Field | Source |
|-------|--------|
| `Version`, `VersionRaw` | Network `/stat/sysinfo` |
| `ProtectInstalled`, `ProtectVersion` | Protect NVR endpoint |
| `ConsoleCertSupported` | `/api/userCertificates` |

Resources check these with helpers in `internal/provider/common.go`: `requireProtect` and `requireConsoleCert`, called from `Configure` or `ModifyPlan`.

Capabilities that only matter to one resource are probed lazily by that resource. `c.Network().Features(ctx)` returns the v2 feature list; `Features.Has(name)` means the console supports a feature, not that it is enabled. `unifi_network_threat_management` checks `Has("IPS")` in `ModifyPlan`.

`unifi.CapabilityError` exists and `describeClientError` renders it, but nothing returns it yet.

## 6. Safety defaults

1. Immutable fields use `RequiresReplace()`.
2. Destroying `unifi_network_device` reverts the config it applied. Destroying a Protect camera, light, sensor, chime or viewer leaves its config as is. Both keep the device adopted; forgetting it needs `forget_on_destroy = true`, guarded with `prevent_destroy`.
3. Objects the controller flags `attr_no_delete` (default WLAN group, user group, AP group, RADIUS profile), plus Access doors and door guards, are import and update only. Destroy removes them from state and leaves the controller alone.
4. Validators mirror the controller and are never tighter. Firewall `rule_index` accepts the full range, including the 20000+ band that migrated rules use. A duplicate-index check runs in Create and Update.

## 7. Logging and redaction

`transport.debugHTTP` logs every request and response at DEBUG as `wire HTTP`, with method, URL, status and both bodies. It does nothing unless `TF_LOG` or `TF_LOG_PROVIDER` is set. How to capture and read the log is in [development.md](development.md#wire-logs).

Before logging, JSON bodies are redacted. A value is masked when its field name contains a hint from `secretKeyHints` (`password`, `psk`, `secret`, `token`, `_key`, …, case-insensitive) or when it is a string containing `PRIVATE KEY`. When you add a field that carries a secret under a name no hint matches, add a hint and a test in `transport/debug_ci_test.go`.
