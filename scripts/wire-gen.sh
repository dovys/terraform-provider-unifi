#!/usr/bin/env bash
# Codegen for one internal/unifi/<app>/wire package; run from that package dir
# by its go:generate directive (see gen.go there).
#
#   wire-gen.sh <spec> [<projector.py> <root>...]
#
# Optionally projects the spec through a uv-run projector, strips the vendor's
# description/summary prose (strip-spec-prose.go), then runs oapi-codegen with
# the package's oapi-codegen.yaml. Intermediates live in a per-run temp dir.
# Not an oapi-codegen overlay: `$..description` would also drop properties
# named "description"; the stripper removes scalar prose only.
set -euo pipefail

if [ ! -f "$1" ]; then
  echo "wire-gen: spec not found: $1 (is UNIFI_SPECS set? see 'make generate')" >&2
  exit 1
fi
spec="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
shift

scripts="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

src="$spec"
if [ "$#" -gt 0 ]; then
  projector="$1"
  shift
  uv run "$projector" "$spec" "$tmp/projected.yaml" "$@"
  src="$tmp/projected.yaml"
fi

# -C: resolve the stripper's yaml.v3 import against this module, whatever the cwd.
go -C "$scripts" run strip-spec-prose.go "$src" "$tmp/codegen.yaml"
go tool oapi-codegen -config oapi-codegen.yaml "$tmp/codegen.yaml"
