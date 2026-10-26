#!/usr/bin/env -S uv run python
"""Redact a raw UniFi API capture so only its *shape* survives.

Studying the private UniFi APIs means pasting captured JSON around. Raw
captures are real network data: secrets, PSKs, tokens, MACs, IPs, emails. We
only ever need the **structure, field names, and enum values** — never the
values themselves. This walks a JSON
document and:

  * replaces the *value* of any key whose name looks like a secret
    (password / psk / token / apikey / cookie / ...) with "<REDACTED>";
  * masks value *patterns* anywhere they appear regardless of key — MAC
    addresses -> "<MAC>", IPv4/IPv6 -> "<IP>"/"<IPV6>", emails -> "<EMAIL>".

It deliberately does NOT touch short enum-like strings ("default", "person",
"fullHd", ...) or 24-hex object ids — those reveal the contract we want to
model and carry no secret. When in doubt about a field, redact upstream by
hand; this is a floor, not a guarantee.

Usage:
    uv run scripts/redact-capture.py raw.json > redacted.json
    some-curl | uv run scripts/redact-capture.py > redacted.json
"""

from __future__ import annotations

import ipaddress
import json
import re
import sys

# Substrings (matched case-insensitively against the key name) whose *values*
# are always secret. Curated to avoid clobbering enum-bearing fields: e.g. "psk"
# already covers wpa_psk/x_psk, so "wpa" is omitted to keep the wpa_mode enum
# visible. UniFi's "owned secret" convention prefixes such keys with "x_", which
# is matched as a *prefix* (below) so it doesn't catch innocuous keys like
# "max_clients".
SECRET_KEY_SUBSTRINGS = (
    "password",
    "passphrase",
    "psk",
    "secret",
    "token",
    "apikey",
    "api_key",
    "privatekey",
    "private_key",
    "authorization",
    "cookie",
    "credential",
    "jwt",
    "hashed",
    "pwd",
    "sshkey",
    "ssh_key",
    "certificate",
)

REDACTED = "<REDACTED>"

# Scalar value-pattern maskers applied to every remaining string, in order.
_MAC = re.compile(r"\b(?:[0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b")
_IPV4 = re.compile(r"\b\d{1,3}(?:\.\d{1,3}){3}\b")
_EMAIL = re.compile(r"\b[\w.+-]+@[\w-]+\.[\w.-]+\b")
# IPv6 is matched by extracting hex:colon candidate tokens and *validating* them
# with the stdlib — far more robust than a regex against the "::" compressed
# form, and it won't false-positive on object ids (no colons) or already-masked
# MACs (6 groups → invalid v6). Runs after MAC so masked MACs can't re-match.
_IPV6_CANDIDATE = re.compile(r"[0-9A-Fa-f:]{2,}(?:/\d{1,3})?")


def _is_secret_key(key: str) -> bool:
    low = key.lower()
    if low.startswith("x_"):
        return True
    return any(sub in low for sub in SECRET_KEY_SUBSTRINGS)


def _mask_ipv6(s: str) -> str:
    def repl(m: re.Match[str]) -> str:
        tok = m.group(0)
        addr = tok.split("/", 1)[0]
        try:
            ip = ipaddress.ip_address(addr)
        except ValueError:
            try:
                ip = ipaddress.ip_network(tok, strict=False).network_address
            except ValueError:
                return tok
        return "<IPV6>" if ip.version == 6 else tok

    return _IPV6_CANDIDATE.sub(repl, s)


def _mask_value_patterns(s: str) -> str:
    s = _EMAIL.sub("<EMAIL>", s)
    s = _MAC.sub("<MAC>", s)
    s = _IPV4.sub("<IP>", s)
    s = _mask_ipv6(s)
    return s


def redact(node: object, parent_key: str | None = None) -> object:
    if isinstance(node, dict):
        return {
            k: (REDACTED if _is_secret_key(k) else redact(v, k))
            for k, v in node.items()
        }
    if isinstance(node, list):
        return [redact(item, parent_key) for item in node]
    if isinstance(node, str):
        return _mask_value_patterns(node)
    return node


def main() -> int:
    raw = (
        open(sys.argv[1], encoding="utf-8").read()
        if len(sys.argv) > 1
        else sys.stdin.read()
    )
    try:
        doc = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"redact-capture: input is not valid JSON: {e}", file=sys.stderr)
        return 1
    json.dump(redact(doc), sys.stdout, indent=2, sort_keys=False)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
