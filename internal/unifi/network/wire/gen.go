// Package wire holds the generated Go types for the UniFi Network API: structs,
// enum constants, and Valid() guards, generated from the maintainer's
// version-pinned API spec (not public).
//
// Types only. The ergonomic, hand-written client (groups, CRUD, the v2 write
// contract, capability gating) lives in the parent `network` package and picks
// the types it needs from here. The generated operation layer is intentionally
// not produced: the spec's operationIds are noisy, whereas the type surface
// (field names, enums, validation) is clean.
//
// The output is unversioned on purpose. The provider tracks the latest modeled
// controller; there are no per-version client packages. On a firmware/spec
// bump, re-point the path below at the new version and run `make generate`;
// review the diff.
//
//go:generate bash ../../../../scripts/wire-gen.sh $UNIFI_SPECS/network/v10.4.57/openapi.yaml
package wire
