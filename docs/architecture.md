# Architecture: API surfaces, client, auth & safety

## 1. Two API surfaces, one session

The provider uses two HTTP surfaces behind one host, both authenticated by a single cookie+CSRF
session (a local UniFi OS admin). Paths below are verbatim from the live controller (captured in the
local network fixture's `meta.api_surfaces`).

| Surface | Base path | Envelope | Auth | Role |
|---------|-----------|----------|------|------|
| **Legacy REST** | `/proxy/network/api/s/<site>/rest/<type>`, `/stat/...`, `/cmd/...`, `/self/...` | `{meta:{rc},data:[]}` | cookie + CSRF | Internal API; **widest write surface**. Reverse-engineered, version-unstable. |
| **v2** | `/proxy/network/v2/api/site/<site>/<path>` | bare JSON | cookie + CSRF | Newer: zone firewall, traffic rules/routes, static-DNS, AP groups. |

**One classic session covers both.** v2 is not a separate transport or auth — it's the same cookie
session, a different URL prefix. Most writes are legacy REST (the widest, most stable surface); v2
owns only the newer feature areas above. Each typed `Client` method targets its surface directly via
the `legacy*`/`v2*` helpers — there is no per-operation router.

The console's official **Integration v1** API (`/integration/v1`, `X-API-KEY`) is **out of scope**:
it is firmware-gated, read-mostly on the writes the provider needs, and would add a second auth
mechanism and a routing layer for little gain — a local admin session reaches everything. Also out of
scope: the cloud **Site Manager API** (`api.ui.com`, read-only).

**UniFi Protect** (cameras, doorbells, …) is **in scope** under the `unifi_protect_*` namespace — it
reuses this client and console session under `/proxy/protect/...` (see [apps/protect.md](apps/protect.md)).
The other UniFi OS apps (Access/Talk/Connect/InnerSpace) are out of scope for now.

## 2. Client

```
internal/unifi/
├── client.go        // Client interface: typed methods per logical operation
├── transport/
│   └── classic.go      // cookie+CSRF → /rest (legacy) AND /v2; handles re-auth
├── surfaces.go      // legacy + v2 request helpers + wire envelopes
├── networks.go      // NetworkConf struct + network CRUD
├── dns.go           // StaticDNSRecord struct + local-DNS CRUD
├── sites.go         // Site/LegacySite + site CRUD
└── capabilities.go  // controller version detection
```

Request/response structs live beside the methods that use them (no separate `models` package).

Resources call `Client` methods only — never a transport. Each method picks its surface (legacy
`/rest` or v2) directly through the `legacy*`/`v2*` helpers in `surfaces.go`. Client is hand-written
and typed (no generated MPL dependency).

### Per-resource surface

| Resource | Read | Write |
|----------|------|-------|
| networks/VLANs, WAN | legacy `/rest/networkconf` | **legacy** `/rest/networkconf` |
| WLAN, WLAN group | legacy `/rest/wlanconf`, `/rest/wlangroup` | legacy |
| AP group | **v2** `/apgroups` | **v2** |
| device (adopt + config + ports) | legacy `/stat/device` (+ read-only `data.unifi_network_device`) | adopt: legacy `/cmd/devmgr`; config: **legacy** `PUT /rest/device/<id>` (GET-merge-PUT, owned fields) |
| port profile | legacy `/rest/portconf` | legacy |
| firewall group/rule (classic) | legacy `/rest/firewallgroup`,`/rest/firewallrule` | legacy |
| firewall zone/policy, traffic rule/route | **v2** | **v2** |
| static route, port forward, dynamic DNS | legacy `/rest/{routing,portforward,dynamicdns}` | legacy |
| local DNS | **v2** `/static-dns` | **v2** `/static-dns` |
| radius profile/account, user group | legacy `/rest/{radiusprofile,account,usergroup}` | legacy |
| DHCP reservation | legacy `/rest/user` (use_fixedip) | **legacy** |
| settings (per key) | legacy `/rest/setting` | legacy |
| sites | legacy `/self/sites` | legacy `/cmd/sitemgr` |

## 3. Authentication

A **local UniFi OS admin account** (not SSO/cloud) is required: `username` + `password` authorize the
single cookie+CSRF session that serves both the legacy and v2 surfaces.

**Classic session lifecycle (specified, not "transparent"):**
- `POST /api/auth/login` → `TOKEN` cookie (JWT; **~2h default — read the `exp` claim, don't hard-code
  the lifetime**); requests proxy under `/proxy/network/...`.
- CSRF read from the response header (`X-CSRF-Token`/`X-Updated-CSRF-Token`), **falling back to the
  JWT `csrfToken` claim** (some firmwares only surface it there). Mutating requests echo it + cookie.
- **Re-auth + retry once** on 401/403 (or near `exp`) so a long apply can't die when the token lapses.
- **Reject cloud/2FA accounts clearly:** cloud accounts force MFA; 2FA returns HTTP `499`
  (`api.err.Ubic2faTokenRequired`) → emit "use a local admin account", not a raw error.

**Provider schema:** `host`, `username`, `password` (sensitive), `site` (default `"default"`),
`allow_insecure`, `timeout`, `max_retries`. Env fallbacks for each (`UNIFI_HOST`, `UNIFI_USERNAME`,
`UNIFI_PASSWORD`, …). Connectivity + capability detection run in `Configure`.

## 4. v2 write contract

Default `encoding/json` reproduces controller-crashing bugs. The v2 transport uses minimal request
structs that:
- send `_id` in **both** URL and body on PUT (else `500`);
- treat all 2xx as success (zone/policy DELETE returns `204`);
- **omit empty strings** (e.g. `domain_name:""` → `IllegalArgumentException`);
- model `port` as a **string** (`"443"`);
- force `setting_preference:"manual"` on network writes (sending `"auto"` clobbers intent);
- never send a bare `false` that silently clears a setting.

Every v2 model has a unit test asserting the wire bytes.

## 5. Capability & version gating

`Configure` builds a cached `capabilities` struct:
- **Version from legacy `/stat/sysinfo`** (always present with classic auth).

Future capability bits (e.g. classic-vs-zone firewall mode) are detected lazily by the resources that
need them — counts across both surfaces — not precomputed here. Resources surface capability errors at
**plan time** via `ModifyPlan`/`ValidateConfig` (a typed `CapabilityError` → diagnostic) — never a raw
4xx mid-apply. Classic vs. zone firewall is detected this way and the two are never mixed.

## 6. Safety / footgun prevention

1. **No silent re-creates** — immutable fields use `RequiresReplace()`.
2. **Danger isolated + guarded** — `unifi_network_device` destroy **reverts config and keeps the device
   adopted**; forgetting requires `forget_on_destroy = true` (guard with `prevent_destroy`).
   `attr_no_delete` objects (default network/WLAN group/user group) are import/update-only; destroy =
   no-op state removal, not a hard error.
3. **Capability gating** turns 4xx into plan-time diagnostics (§5).
4. **Validators mirror the controller** — e.g. firewall `rule_index` allows the full range
   (migrated classic rules land in a high band like 20000+) with a plan-time duplicate-index guard.
5. **Read reconciles, never blind-overwrites** — refresh Computed, reconcile configured only when
   canonically different; remove-on-404. `UseStateForUnknown()` only on post-create-immutable values.
   Telemetry is **Computed-only** (refreshed on read, never part of the plan). This makes "a live
   instance imports to an empty plan" hold.
6. **429 backoff** (honor `Retry-After`); **secrets `Sensitive`**; **site explicit in the plan**.
