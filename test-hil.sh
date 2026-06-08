#!/usr/bin/env bash
#
# Tier 3 — hardware-in-the-loop, against the operator's LIVE console. Use
# SPARINGLY: it touches real gear and is not a CI gate. Delegates to the rungs in
# test/hil/ (config + credentials come from test/hil/secrets.env, gitignored).
# See docs/testing.md and test/hil/README.md.
#
#   ./test-hil.sh         read-only: import the live console, assert a clean plan
#   ./test-hil.sh write   additive write verification (W0–W2), triple-gated
#
set -euo pipefail
cd "$(dirname "$0")"
case "${1:-read}" in
  read)  exec test/hil/run.sh ;;
  write) shift; exec test/hil/write.sh "$@" ;;
  *)     echo "usage: ./test-hil.sh [read|write]" >&2; exit 2 ;;
esac
