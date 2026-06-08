#!/usr/bin/env bash
#
# Tier 1 — unit tests only. Fast, no controller, no behaviour reproduction:
# serialization round-trips, validators, plan modifiers, capability logic. This
# is exactly what CI runs; concrete behaviour/interface coverage lives in Tier 2
# (./test-extended.sh). See docs/testing.md.
#
set -euo pipefail
cd "$(dirname "$0")"
exec go test ./... -timeout 120s "$@"
