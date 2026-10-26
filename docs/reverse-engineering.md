# API discovery

The provider talks to the private APIs the UniFi web UI uses, and most of them have no public documentation. This page covers how to work out one of these surfaces without copying vendor code: watch what the UI and the provider send, read your own console, and use the OpenAPI documents UniFi ships.

## Surfaces

The surfaces, their base paths, and their write contracts are in [architecture.md](architecture.md#1-api-surfaces). UniFi's official APIs, the Integration API (`X-API-KEY`) and the documented parts of the Network API, miss much of the feature set, so the provider models the private APIs instead and leaves the Integration API out of scope.

## Clean-room rules

Model the interface you can observe. Never model a vendor's implementation.

| Allowed | Not allowed |
|---|---|
| Paths, verbs, query and body field names, enum values, validation rules (required, min/max, formats) | Copying vendor code, logic, comments, or naming |
| Request and response shapes seen on the wire | Lifting a vendor data model verbatim |
| Observed behavior: merge or replace, defaults, computed fields, side effects | Reproducing proprietary algorithms |

Also never copy code from other Terraform providers or UniFi client libraries.

## Methods

### Vendor OpenAPI documents

- Protect ships a full OpenAPI document for its private API. Start there.
- Access ships only an Integration API document. It describes a different surface, so use it only as a hint for field names.
- The console OS surface has none.

These are interface documents. They list fields and enums, not how the controller behaves on write, and a fact found only in one stays unconfirmed until you observe it.

### Browser devtools

This is the fastest way to see what a UI feature sends.

1. Open the UniFi web UI on your console and open the browser's devtools Network tab.
2. Filter on `/proxy/` (or `/api/` for console OS settings).
3. Do the action in the UI.
4. Record the method, URL, request body, and response body.

You can export the session as a HAR file, but a HAR carries your session cookie and CSRF token. Extract the bodies you need and run them through [`scripts/redact-capture.py`](#redacting-captures); never share the HAR itself.

### Read-only GETs

Read your own console from your workstation over the cookie session. Keep credentials in environment variables and the cookie in a jar file, so neither ends up in shell history or a paste:

```sh
curl -sk -c jar.txt -H 'Content-Type: application/json' \
  -d "{\"username\":\"$UNIFI_USERNAME\",\"password\":\"$UNIFI_PASSWORD\"}" \
  "https://$UNIFI_HOST/api/auth/login" -o /dev/null

mkdir -p re-captures
curl -sk -b jar.txt "https://$UNIFI_HOST/proxy/protect/api/bootstrap" \
  | uv run scripts/redact-capture.py > re-captures/protect-bootstrap.json
rm jar.txt
```

GETs need no CSRF token. Useful starting points are `/proxy/protect/api/bootstrap`, `/proxy/protect/api/cameras`, and `/proxy/protect/api/nvr`.

### The provider's wire log

Run Terraform with `TF_LOG_PROVIDER=debug` and grep for `wire HTTP` to see every request and response the provider sends ([Wire logs](development.md#wire-logs)). Use it to see which fields a write accepts: `api.err.InvalidValue` and `api.err.InvalidPayload` mean the surface rejected a field. It masks secret-named fields but not MACs, IPs, or names, so redact it like any other capture.

### Write behavior

GET an object, send a sparse write, then GET it again. The diff tells you whether the surface merges or replaces, which fields it defaults, and which it computes. Do this only on a disposable object you created (name it with the `tfacc-` prefix), never on a production network. Follow the test tiers in [testing.md](testing.md).

If you can't confirm a behavior by observation, mark it provisional in the issue or PR, or list it in [TODO.md](../TODO.md) as needing [hardware-in-the-loop (HIL)](testing.md#tier-3-hardware-in-the-loop) verification. Don't ship it as certain.

## Record shapes, not values

A useful capture records:

- field names, types, and nullability;
- enum values;
- which fields round-trip on write and which the controller ignores or rejects;
- merge or replace semantics (the legacy `/rest` PUT merges, so the provider sends only the fields it owns);
- defaults and computed fields.

It never records real values from your network.

## Redacting captures

[`scripts/redact-capture.py`](../scripts/redact-capture.py) reads JSON from a file or stdin:

```sh
uv run scripts/redact-capture.py raw.json > re-captures/redacted.json
```

It replaces the values of secret-named keys (password, psk, token, cookie, any `x_` key, and so on) with `"<REDACTED>"`, and masks MACs, IPv4, IPv6, and emails anywhere they appear. It keeps field names, enums, and structure.

It does not touch names, hostnames, SSIDs, site names, descriptions, 24-hex object ids, or other free text. Redact those by hand and read the output before you share it.

Keep raw and redacted captures in `re-captures/`, which is gitignored. Never commit a capture. Record each capture's provenance next to it: the app and firmware version, the UI action or request, and the method (devtools, wire log, GET, or vendor OpenAPI). Test fixtures use fictional values: IPs from 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24, or 2001:db8::/32, and MACs from 00:00:5e:00:53:00/24.

## From capture to code

To contribute a surface, open an issue or PR with the redacted request and response, the UniFi app and firmware version, and the UI action. See [CONTRIBUTING.md](../CONTRIBUTING.md).

The maintainer then regenerates the wire types ([Wire types](development.md#wire-types)), and the resource follows the recipe in [development.md](development.md), and the surface is documented in the matching `docs/apps/*.md`. Test it with unit round-trips against fictional-value fixtures (`*_ci_test.go`, `make test`), add a HIL test where hardware is involved (`make testhil`), and confirm that importing objects from your own console reaches a clean plan. See [testing.md](testing.md).

## Open surfaces

Deferred surfaces are tracked in [roadmap.md](roadmap.md), and open capture work in [TODO.md](../TODO.md).
