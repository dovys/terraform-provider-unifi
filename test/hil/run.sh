#!/usr/bin/env bash
# Tier-3 HIL — Rung B: import the operator's LIVE console with our provider and
# assert a clean plan (the truest regression; docs/testing.md).
#
# READ-ONLY on the controller: import-discovery and `tofu plan` issue only GETs
# (plus the provider's session-login POST). This script NEVER runs `apply` or
# `destroy` — there is no such codepath. Run from the Mac (the mgmt VLAN is
# workstation-only).
#
# Usage:  ./run.sh
# Config: put your console address + local-admin credentials in test/hil/secrets.env
#         (gitignored — copy secrets.env.example). Or export UNIFI_* yourself.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

# Operator-specific config (live console address, local-admin credentials) lives
# in test/hil/secrets.env — gitignored, so the maintainer's network topology and
# password-manager layout never land in this (eventually public) repo.
if [ -f "$here/secrets.env" ]; then
  # shellcheck source=/dev/null
  . "$here/secrets.env"
fi
: "${UNIFI_HOST:?set UNIFI_HOST — see test/hil/secrets.env.example}"
: "${UNIFI_USERNAME:?set UNIFI_USERNAME (a local UniFi OS admin) — see test/hil/secrets.env.example}"
: "${UNIFI_PASSWORD:?set UNIFI_PASSWORD — see test/hil/secrets.env.example}"
export UNIFI_HOST UNIFI_USERNAME UNIFI_PASSWORD
export UNIFI_ALLOW_INSECURE="${UNIFI_ALLOW_INSECURE:-true}"

# 1) Build + install the provider; point dev_overrides at it (no registry/init).
( cd "$repo" && go install ./... )
bin="$(go env GOBIN)"; [ -z "$bin" ] && bin="$(go env GOPATH)/bin"
cat > "$here/dev.tfrc" <<EOF
provider_installation {
  dev_overrides { "dovys/unifi" = "$bin" }
  direct {}
}
EOF
export TF_CLI_CONFIG_FILE="$here/dev.tfrc"

# 2) Discover live object IDs -> import blocks (gitignored).
( cd "$repo" && HIL_IMPORTS_OUT="$here/imports.tf" \
    go test -tags hil ./internal/provider -run TestHILGenerateImports -count=1 -v )

# 3) Plan with import blocks + config generation. Read-only; never applied.
cd "$here"
rm -f generated.tf plan.out
set +e
tofu plan -generate-config-out=generated.tf -input=false -no-color 2>&1 | tee plan.out
rc=${PIPESTATUS[0]}
set -e

echo
echo "── Rung B result ──────────────────────────────────────────────"
if [ "$rc" -ne 0 ]; then
  echo "❌ tofu plan errored (exit $rc) — inspect plan.out / generated.tf."
  exit "$rc"
elif grep -qE 'Plan: [0-9]+ to import, 0 to add, 0 to change, 0 to destroy' plan.out; then
  echo "✅ CLEAN: every imported live object round-trips with no drift."
else
  echo "⚠️  Plan shows drift (add/change/destroy != 0). Each diff is a HIL finding —"
  echo "    a field the provider's Read/normalize does not round-trip against live."
  echo "    Inspect plan.out (the diff) and generated.tf (what the provider read)."
fi
