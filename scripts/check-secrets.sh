#!/usr/bin/env bash
# secret-hygiene gate. Fail if private operator config leaked into a tracked
# file: a private-range address or a committed credential fetch (op / keychain /
# a --vault reference).
#
# HIL operator config belongs in the GITIGNORED test/hil/secrets.env, never in a
# tracked file — see test/hil/secrets.env.example and docs/testing.md. Run by the
# `secret-hygiene` CI job and by `make check`.
#
# Optional: scripts/secret-denylist.local (gitignored via *.local) lists your own
# identifiers (hostnames, account names, MACs), one fixed string per line (`#`
# comments and blank lines ignored); each is also forbidden, case-insensitively.
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
#   10.a.b.c              any RFC-1918 10/8 address
#   100.a.b.c             100/8 (CGNAT / overlay-network range)
#   192.168.a.b           RFC-1918 192.168/16 — forbidden EXCEPT under examples/,
#                         where a fictional home config reads as 192.168.x
#   find-generic-password a macOS Keychain credential fetch
#   op item get           a 1Password credential fetch
#   --vault               a 1Password vault reference
# 10/8, 100/8 and the credential tokens are forbidden in EVERY tracked file
# (examples included); 192.168/16 is split out so it can be allowed under examples/.
ip_real='(^|[^0-9.])(10|100)\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'
ip_192='(^|[^0-9.])192\.168\.[0-9]{1,3}\.[0-9]{1,3}'
creds='find-generic-password|op item get|--vault'
denylist='scripts/secret-denylist.local'

# git grep exits 0=match, 1=no-match, >1=error. Distinguish them explicitly: an
# error (bad pattern, broken repo) must FAIL loudly, never be mistaken for clean.
set +e
# 10/8, 100/8 and the credential tokens — every tracked file (examples included).
hits_real="$(git grep -nE "${ip_real}|${creds}" -- \
  ':(exclude)scripts/check-secrets.sh' \
  ':(exclude)*.example')"
rc1=$?
# 192.168/16 — every tracked file EXCEPT under examples/.
hits_192="$(git grep -nE "$ip_192" -- \
  ':(exclude)scripts/check-secrets.sh' \
  ':(exclude)*.example' \
  ':(exclude)examples/')"
rc2=$?
# Local denylist — every tracked file. Comment/blank lines are dropped first: an
# empty fixed-string pattern would match every line.
hits_local=""
rc3=0
if [ -f "$denylist" ]; then
  patterns="$(mktemp)" || exit 2
  # Strip CR/trailing blanks: they'd stay in the -F pattern and never match.
  sed -e 's/[[:space:]]*$//' "$denylist" | grep -vE '^[[:space:]]*(#|$)' >"$patterns"
  if [ -s "$patterns" ]; then
    hits_local="$(git grep -n -i -F -f "$patterns" -- \
      ':(exclude)scripts/check-secrets.sh')"
    rc3=$?
  fi
  rm -f "$patterns"
fi
set -e

if [ "$rc1" -gt 1 ] || [ "$rc2" -gt 1 ] || [ "$rc3" -gt 1 ]; then
  echo "::error::secret-hygiene: git grep failed (exit $rc1/$rc2/$rc3) — pattern or repo error, not a clean result."
  exit 2
fi

hits="$(printf '%s\n%s\n%s' "$hits_real" "$hits_192" "$hits_local" | grep -vE '^$' || true)"
if [ -n "$hits" ]; then
  echo "::error::secret-hygiene: private operator config found in tracked files."
  echo "Move the console address / credentials into the gitignored test/hil/secrets.env"
  echo "(template: test/hil/secrets.env.example; see docs/testing.md). Offending lines:"
  echo "$hits"
  exit 1
fi

echo "secret-hygiene: clean — no private console/credential config in tracked files."
