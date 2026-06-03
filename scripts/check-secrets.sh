#!/usr/bin/env bash
# secret-hygiene gate. Fail if maintainer-private config leaked into the
# (eventually public) repo: the live console's address/supernet, the
# local-admin / 1Password item name, or a committed credential fetch
# (op / keychain / a --vault reference).
#
# HIL operator config belongs in the GITIGNORED test/hil/secrets.env, never in a
# tracked file — see test/hil/secrets.env.example and docs/testing.md. Run by the
# `secret-hygiene` CI job and by `make check`.
#
# Scans tracked files only. Excludes this script (it necessarily names the
# patterns) and *.example templates (the one sanctioned place to show the
# credential-fetch pattern, with placeholders).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Forbidden tokens. The IP patterns match a full dotted quad in a private range,
# leading-anchored so a non-first octet (e.g. the 100 in the RFC-5737
# 198.51.100.x test range) and version strings (10.15.7) don't false-match. Real
# topology must use the RFC-5737/3849 documentation ranges (192.0.2.x,
# 198.51.100.x, 203.0.113.x, 2001:db8::/32), which are allowed:
#   10.a.b.c              any RFC-1918 10/8 address (the home VLANs live here)
#   100.a.b.c             100/8 (CGNAT / overlay-network range)
#   192.168.a.b           RFC-1918 192.168/16
#   network-admin  the local-admin account / password-manager item name
#                         (also matches the item `unifi-network-admin`)
#   find-generic-password a macOS Keychain credential fetch
#   op item get           a 1Password credential fetch
#   --vault               a 1Password vault reference
ipv4='(^|[^0-9.])(10|100)\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|(^|[^0-9.])192\.168\.[0-9]{1,3}\.[0-9]{1,3}'
creds='network-admin|find-generic-password|op item get|--vault'
patterns="${ipv4}|${creds}"

# git grep exits 0=match, 1=no-match, >1=error. Distinguish them explicitly: an
# error (bad pattern, broken repo) must FAIL loudly, never be mistaken for clean.
set +e
hits="$(git grep -nE "$patterns" -- \
  ':(exclude)scripts/check-secrets.sh' \
  ':(exclude)*.example')"
rc=$?
set -e

if [ "$rc" -gt 1 ]; then
  echo "::error::secret-hygiene: git grep failed (exit $rc) — pattern or repo error, not a clean result."
  exit "$rc"
elif [ "$rc" -eq 0 ]; then
  echo "::error::secret-hygiene: maintainer-private config found in tracked files."
  echo "Move the console address / credentials into the gitignored test/hil/secrets.env"
  echo "(template: test/hil/secrets.env.example; see docs/testing.md). Offending lines:"
  echo "$hits"
  exit 1
fi

echo "secret-hygiene: clean — no private console/credential config in tracked files."
