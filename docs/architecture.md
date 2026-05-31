# Architecture: API surfaces, client, auth & safety

## 1. Three API surfaces

A UniFi OS console exposes three HTTP surfaces behind one host. Paths below are verbatim from the
live controller (captured in the local network fixture's `meta.api_surfaces`).

| Surface | Base path | Envelope | Auth | Role |
|---------|-----------|----------|------|------|
| **Integration v1** | `/proxy/network/integration/v1/...` | `{offset,limit,count,totalCount,data[]}` | `X-API-KEY` | Official, typed, paginated. Writes are **firmware-gated** (networks/WLANs/zone-firewall/ACL/DNS/vouchers/adoption on 10.1.84+); read-mostly on older firmware. The owner's box runs **Network 10.4.57**, so these writes are live there. |
| **Legacy REST** | `/proxy/network/api/s/<site>/rest/<type>`, `/stat/...`, `/cmd/...` | `{meta:{rc},data:[]}` | cookie + CSRF | Internal API; **widest write surface**. Reverse-engineered, version-unstable. |
| **v2** | `/proxy/network/v2/api/site/<site>/<path>` | bare JSON | cookie + CSRF | Newer: zone firewall, traffic rules/routes, static-DNS, AP groups. |

**Official-where-capable, legacy/v2 fallback.** The router prefers Integration when capability
detection says the firmware supports a write, else legacy/v2 — with no resource-code change. The
owner's box runs Network 10.4.57 (≥ 10.1.84), so Integration writes are available there; older
firmware falls back to legacy/v2 automatically.

**UniFi Protect** (cameras, doorbells, …) is **in scope** under the `unifi_protect_*` namespace — it
reuses this client and the **same console API key** under `/proxy/protect/...` (analogous Integration
+ legacy surfaces; see [protect.md](protect.md)). Out of scope: the cloud **Site Manager API**
(`api.ui.com`, read-only) and the other UniFi OS apps (Access/Talk/Connect/InnerSpace) for now.

## 2. Client

```
internal/unifi/
├── client.go        // Client interface: typed methods per logical operation
├── router.go        // picks backend per operation (policy table + capabilities)
├── transport/
│   ├── integration.go  // X-API-KEY → /integration/v1
│   └── classic.go      // cookie+CSRF → /rest (legacy) AND /v2; handles re-auth
├── models/          // typed request/response structs
└── capabilities.go  // version detection, feature flags, site name↔UUID map
```

Resources call `Client` methods only — never a transport — so the backend split is invisible to
resource code and a future Integration write is a one-line routing change. Client is hand-written
and typed (no generated MPL dependency); if the console serves an OpenAPI spec for `/integration/v1`
we may clean-room codegen that surface.

### Per-resource backend routing

| Resource | Read | Write |
|----------|------|-------|
| networks/VLANs, WAN | integration `GET /networks` → legacy | **legacy** `/rest/networkconf` |
| WLAN, WLAN group | legacy `/rest/wlanconf`, `/rest/wlangroup` | legacy |
| AP group | **v2** `/apgroups` | **v2** |
| device (read) | legacy `/stat/device` → `data.unifi_network_device` | — |
| device adoption | legacy `/stat/device` | integration `POST /devices/adopt` **or** legacy `/cmd/devmgr` |
| device config + ports | legacy `/stat/device` | **legacy** `PUT /rest/device/<id>` (GET-merge-PUT, owned fields) |
| port profile | legacy `/rest/portconf` | legacy |
| firewall group/rule (classic) | legacy `/rest/firewallgroup`,`/rest/firewallrule` | legacy |
| firewall zone/policy, traffic rule/route | **v2** | **v2** |
| static route, port forward, dynamic DNS | legacy `/rest/{routing,portforward,dynamicdns}` | legacy |
| local DNS | **v2** `/static-dns` | **v2** (⚠ legacy `/rest/dnsrecord` 400s) |
| radius profile/account, user group | legacy `/rest/{radiusprofile,account,usergroup}` | legacy |
| DHCP reservation | legacy `/rest/user` (use_fixedip) | **legacy** |
| settings (per key) | legacy `/rest/setting` | legacy |
| sites | integration `GET /sites` | legacy `/cmd/sitemgr` |
| vouchers | integration `/hotspot/vouchers` | integration |

## 3. Authentication

Accepts either or both credential sets; a **local admin account** (not SSO/cloud) is effectively
required today.

| Credential | Enables |
|------------|---------|
| `api_key` (`X-API-KEY`) | Integration v1 — UniFi Network → Settings → Control Plane → Integrations |
| `username` + `password` | Legacy + v2 — a local UniFi OS account |

**Classic session lifecycle (specified, not "transparent"):**
- `POST /api/auth/login` → `TOKEN` cookie (JWT, **~2h expiry**); requests proxy under `/proxy/network/...`.
- CSRF read from the response header (`X-CSRF-Token`/`X-Updated-CSRF-Token`), **falling back to the
  JWT `csrfToken` claim** (some firmwares only surface it there). Mutating requests echo it + cookie.
- **Re-auth + retry once** on 401/403 (or near `exp`) so a long apply can't die when the token lapses.
- **Reject cloud/2FA accounts clearly:** cloud accounts force MFA; 2FA returns HTTP `499`
  (`api.err.Ubic2faTokenRequired`) → emit "use a local admin account", not a raw error.

**Provider schema:** `host`, `api_key` (sensitive), `username`, `password` (sensitive), `site`
(default `"default"`), `allow_insecure`, `timeout`, `max_retries`. Env fallbacks for each
(`UNIFI_HOST`, `UNIFI_API_KEY`, …). Connectivity + capability detection run in `Configure`.

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
- **Version from legacy `/stat/sysinfo`** (always present with classic auth) — not
  `/integration/v1/application-info`, which 404s pre-9.3.
- **Site name↔UUID map** via `GET /integration/v1/sites` (Integration keys sites by UUID, legacy/v2
  by name `default`, legacy objects by Mongo `_id` — three namespaces; match on `internalReference`,
  which is `"default"`, not `name` `"Default"`).
- Feature flags (`IntegrationWritesNetworks`, `HasZoneFirewall`, `StaticDNSOnV2`, …).

Resources surface capability errors at **plan time** via `ModifyPlan`/`ValidateConfig` reading the
cached flags — never a raw 4xx mid-apply. Classic vs. zone firewall is detected (counts across both
surfaces) and the two are never mixed.

## 6. Safety / footgun prevention

1. **No silent re-creates** — immutable fields use `RequiresReplace()`.
2. **Danger isolated + guarded** — only `unifi_network_device_adoption` destroy forgets a device (guard with
   `prevent_destroy`); `unifi_network_device_configuration` destroy reverts its overlay. `attr_no_delete`
   objects (default network/WLAN group/user group) are import/update-only; destroy = no-op state
   removal, not a hard error.
3. **Capability gating** turns 4xx into plan-time diagnostics (§5).
4. **Validators mirror the controller** — e.g. firewall `rule_index` allows the full range
   (migrated classic rules land in a high band like 20000+) with a plan-time duplicate-index guard.
5. **Read reconciles, never blind-overwrites** — refresh Computed, reconcile configured only when
   canonically different; remove-on-404. `UseStateForUnknown()` only on post-create-immutable values.
   Telemetry lives on data sources. This makes "a live instance imports to an empty plan" hold.
6. **429 backoff** (honor `Retry-After`); **secrets `Sensitive`**; **site explicit in the plan**.
