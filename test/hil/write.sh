#!/usr/bin/env bash
# Tier-3 HIL — Write verification (bucket 2). CREATE → verify → DESTROY on the
# operator's LIVE console (docs/testing.md §"Write verification").
#
# UNLIKE run.sh, this WRITES to the controller. Every object is additive, inert
# (disabled rules / RFC-5737 blackhole routes / unreferenced groups), and named
# `tfacc-hil-` so the sweep can always reclaim it. It is a deliberate one-shot,
# NOT a CI gate. Run from the Mac (the mgmt VLAN is workstation-only).
#
# Usage:
#   ./write.sh            # all tiers (W0→W1→W2), interactive confirm
#   ./write.sh W0         # one tier (W0 | W1 | W2)
#   ./write.sh W0 --yes   # skip the interactive confirmation
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

tier="all"; assume_yes="no"
for arg in "$@"; do
  case "$arg" in
    W0|W1|W2|all) tier="$arg" ;;
    --yes|-y)     assume_yes="yes" ;;
    *) echo "unknown arg: $arg (expected W0|W1|W2|all and/or --yes)"; exit 2 ;;
  esac
done
run_re="TestHILWrite"
[ "$tier" != "all" ] && run_re="TestHILWrite_${tier}"

# Operator-specific config (live console address, local-admin credentials) lives
# in test/hil/secrets.env — gitignored, so the maintainer's network topology and
# password-manager layout never land in the repo.
if [ -f "$here/secrets.env" ]; then
  # shellcheck source=/dev/null
  . "$here/secrets.env"
fi
: "${UNIFI_HOST:?set UNIFI_HOST — see test/hil/secrets.env.example}"
: "${UNIFI_USERNAME:?set UNIFI_USERNAME (a local UniFi OS admin) — see test/hil/secrets.env.example}"
: "${UNIFI_PASSWORD:?set UNIFI_PASSWORD — see test/hil/secrets.env.example}"
export UNIFI_HOST UNIFI_USERNAME UNIFI_PASSWORD
export UNIFI_ALLOW_INSECURE="${UNIFI_ALLOW_INSECURE:-true}"

# Opt-in + host pin (the in-code hilWriteGate enforces both; set them here).
export UNIFI_HIL_ALLOW_WRITES=1
export UNIFI_HIL_WRITE_HOST="$UNIFI_HOST"
export TF_ACC=1  # the plugin-testing framework drives real apply/destroy

cat <<EOF

  ┌─ HIL WRITE VERIFICATION ─────────────────────────────────────────┐
  │ This CREATES then DESTROYS objects on a LIVE UniFi console.       │
  │   target : $UNIFI_HOST
  │   tier   : $tier   (W0 data · W1 gateway-inert · W2 AP/switch)
  │   objects: additive + inert + named tfacc-hil- (swept before/after)
  │ W1 triggers a gateway config reload; W2 reprovisions your APs     │
  │ (a brief management-plane blip is possible). Run during low usage.│
  └──────────────────────────────────────────────────────────────────┘
EOF
if [ "$assume_yes" != "yes" ]; then
  read -r -p "  Type 'I understand' to proceed: " reply
  [ "$reply" = "I understand" ] || { echo "  aborted."; exit 1; }
fi

sweep() { # delete any tfacc- leftovers on the live console (pre/post safety net)
  echo "── $1 sweep (tfacc-) ─────────────────────────────────────────"
  ( cd "$repo" && go test -tags hil ./test/hil \
      -sweep=default -timeout 10m 2>&1 ) || echo "  (sweep reported errors — non-fatal; continuing)"
}

sweep "pre-run"

echo "── write tests [$run_re] ─────────────────────────────────────"
set +e
( cd "$repo" && go test -tags hil ./test/hil \
    -run "$run_re" -count=1 -v -timeout 45m 2>&1 | tee "$here/write.out" )
rc=${PIPESTATUS[0]}
set -e

sweep "post-run"  # final net — reclaims anything a failed destroy left behind

echo
echo "── Write-verification result ──────────────────────────────────"
if [ "$rc" -eq 0 ]; then
  echo "✅ PASS: every object created, round-tripped, imported, and destroyed clean."
else
  echo "❌ FAIL (exit $rc) — inspect write.out. The post-run sweep above has"
  echo "   already reclaimed any leftover tfacc-hil- objects."
fi
exit "$rc"
