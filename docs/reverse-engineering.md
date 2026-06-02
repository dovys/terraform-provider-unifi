# Reverse-engineering the UniFi private APIs

**Why this exists.** Official surfaces (the Protect Integration API, the bits of the Network API
Ubiquiti documents) plus the hardware we can physically obtain will never cover the full UniFi feature
surface. To reach genuine **feature-completeness**, we reverse-engineer the **private APIs the web UI
itself uses** — `/proxy/network/...` (already the basis of the Network app) and `/proxy/protect/api/...`
(the Protect pivot) — to learn their real endpoints, data structures, and behavior. This is the same
posture the Network app already takes toward legacy REST, made explicit and systematic.

## Clean-room boundary (read first)

The provider is **clean-room** ([invariants in CLAUDE.md], [[../docs/README.md]]): we model the
**observable interface and behavior**, never a vendor's (or another provider's) implementation. RE does
**not** relax this — it sharpens where the line is:

| Allowed (interface facts) | Not allowed (implementation) |
|---|---|
| Endpoint paths, HTTP verbs, query/body **field names**, **enum values**, validation **rules** (min/max, required, formats) | Copying source code, logic, comments, structure, or variable naming |
| Request/response **shapes** observed on the wire | Lifting a vendor's data model verbatim because it's "easier" |
| Documented/observed **behavior** (merge-vs-replace, defaults, side effects) | Reproducing proprietary algorithms |

**The rule:** on-box source/config is used **only to discover interface facts** (what endpoints exist,
what fields/enums they take). **Every discovered fact is then confirmed by observing the live API**, and
the provider is modeled **from that observed contract** — not from the source. If a fact can't be
confirmed by observation, it's marked provisional/HIL, never shipped as certain. When in doubt, prefer
observation over reading source.

**Captures are real network data → never committed.** Redact secrets, passwords/PSKs, API keys, MACs,
public IPs, and PII before anything leaves the box. Pipe every capture through
[`scripts/redact-capture.py`](../scripts/redact-capture.py) (stdlib-only, `uv run`): it nulls
secret-named values and masks MAC/IP/email patterns while **keeping field names, enum values, and
structure** — exactly the shape we model from. Stage raw dumps under the gitignored `re-captures/`
directory (same rule as `unifi-inventory.json`); the repo holds *modeled types + tests against redacted
fixtures*, never raw dumps. The redactor is a floor, not a guarantee — eyeball the output before sharing.

## Access & method order

The user has **SSH** into the console / UniFi OS Server. Method order:

1. **Source discovery (on-box, facts only).** Locate each app's HTTP route definitions, request
   validators, and stored schemas to **enumerate** the endpoint surface (paths, verbs, params, enums,
   constraints). Read for *what the interface is*, never to copy *how it's implemented*.
2. **Live-API observation (the source of truth).** From the console, authenticate to the local API and
   call the endpoints discovered in (1), capturing request/response **shapes** and **behavior** (do a
   GET, mutate, GET again to learn merge-vs-replace, defaults, computed fields). This is what we model
   from.
3. **Web-UI session recording (fallback / cross-check).** When source is obfuscated/compiled or a flow
   is unclear, record the browser's traffic to the private API (HAR / devtools) to see exactly what the
   UI sends and gets back. Co-equal observation method; promote it to primary for any surface where (1)
   is a dead end.

## Targets & priority

1. **Protect private API** (`/proxy/protect/api/...`) — unblocks the Protect pivot to private REST
   (one cookie session, no `api_key` — see [apps/protect.md](apps/protect.md)). Capture the bootstrap
   document + per-device-type shapes (camera, light, sensor, chime, viewer), the NVR doc, liveviews,
   and the **write verbs** (PATCH/PUT/POST) + their accepted fields. Establishes the typed shapes the
   Protect resources need.
2. **Network long-tail** deferred for lack of a verifiable surface:
   - **settings** — the ~39 `setting/<key>` singletons to type ([TODO.md]).
   - **zone-firewall** — `firewall/zone` + `firewall-policies` and the classic-vs-zone capability gate
     ([[zone-firewall-deferred]]).
   - **VPN / BGP** — the `networkconf` VPN purposes vs. the `openvpn`/`ipsec`/`teleport`/
     `magic_site_to_site_vpn` settings, and however BGP is actually stored ([[phase4-vpn-bgp-deferred]]).

## Workflow (per surface)

1. **Discover** (SSH, source) → an endpoint/field/enum inventory for the target.
2. **Observe** (SSH curl of the local API, and/or web-UI recording) → real request/response shapes +
   behavior, redacted.
3. **Stage** the redacted capture as a gitignored fixture; note provenance (discovered vs. observed).
4. **Model** typed structs + validators in `internal/unifi` + the resource, **from the observed
   contract**. Mark anything unconfirmed as provisional/HIL.
5. **Test** unit round-trips against the redacted fixtures; HIL where hardware allows; live-import
   against the operator's console for read-confirmable resources.
6. **Document** the surface in the relevant `docs/apps/*.md` + the per-resource recipe.

## Kickoff (Protect private API)

First capture, to rework the Protect base onto the private REST surface. Run over SSH on the console;
**redact** before sharing. (Exact service names/paths vary by firmware — step 1 is to find them.)

```sh
# 1) Discover: locate the Protect service + its API routes (facts only — enumerate, don't copy code).
#    Find the running Protect process, its install dir, and where it defines HTTP routes/validators.
ps aux | grep -i protect
#   then explore the install dir's route/handler/schema files to list endpoints, verbs, fields, enums.

# 2) Observe: from the console, log in to the local API and capture the private Protect shapes.
#    (Use your local admin creds; keep the cookie out of anything you paste.)
#    GET /proxy/protect/api/bootstrap        -> the big doc (nvr, cameras[], lights[], ... + version)
#    GET /proxy/protect/api/cameras          -> camera list (private shape, cf. the Integration shape)
#    GET /proxy/protect/api/nvr              -> NVR/console settings

# 3) Redact + stage: run each response through the redactor, into the gitignored re-captures/ dir.
#    (Masks secrets/MAC/IP/email; keeps field names, enums, structure.)
mkdir -p re-captures
uv run scripts/redact-capture.py raw-bootstrap.json > re-captures/protect-bootstrap.json
#    ...or pipe straight from curl:  <curl …> | uv run scripts/redact-capture.py > re-captures/protect-cameras.json
```

Share the **redacted** shapes (types, enums, structure — not values) and I'll model the typed Protect
client + resources from the observed contract. See [[reverse-engineering-workstream]].
