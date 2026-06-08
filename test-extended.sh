#!/usr/bin/env bash
#
# Tier 2 — extended acceptance tests (package test/acc, build tag `acc`). The Go
# harness (TestMain) owns the stack lifecycle — booting the Docker controller +
# Mongo + UniFi-OS bridge, waiting for readiness, seeding a local admin, and
# exporting the connection env. This script only triggers it. The concrete
# behaviour/interface coverage lives in test/acc (create → update → import →
# destroy against a real controller).
#
# Run it before opening a PR and paste the printed summary into the PR
# description. Needs Docker. See docs/testing.md.
#
#   ./test-extended.sh                  boot (if needed) + run every acceptance test
#   ./test-extended.sh -run TestAccFoo  forward extra `go test` flags (one suite)
#   ./test-extended.sh up               boot + seed the stack, run no tests
#   ./test-extended.sh down             tear the stack down
#   ./test-extended.sh --fresh [...]    tear down, then boot fresh + run
#
set -euo pipefail
cd "$(dirname "$0")"

COMPOSE=(docker compose -p tfunifi -f test/tier2/compose.yaml)
GOTEST=(env UNIFI_T2_MANAGE=1 TF_ACC=1 go test -tags acc ./test/acc/)

case "${1:-}" in
  up)      exec "${GOTEST[@]}" -run '^$' ;;
  down)    exec "${COMPOSE[@]}" down -v ;;
  --fresh) shift; "${COMPOSE[@]}" down -v ;;
esac

# The `acc` build tag selects the acceptance package — no name-prefix filter.
# -p 1 keeps suites serial against the single controller.
log="$(mktemp)"
set +e
"${GOTEST[@]}" -p 1 -count=1 -v -timeout 60m "$@" 2>&1 | tee "$log"
rc=${PIPESTATUS[0]}
set -e

echo
echo "### Tier-2 acceptance summary (\`./test-extended.sh\`)"
echo
printf '%s passed, %s failed, %s skipped\n\n' \
  "$(grep -c '^--- PASS' "$log" || true)" \
  "$(grep -c '^--- FAIL' "$log" || true)" \
  "$(grep -c '^--- SKIP' "$log" || true)"
grep -E '^--- (PASS|FAIL|SKIP)' "$log" | sed 's/^--- /    /' || true
rm -f "$log"
exit "$rc"
