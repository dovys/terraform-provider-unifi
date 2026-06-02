// Package wire holds the generated Go types for the UniFi Network API: structs,
// enum constants, and Valid() guards, projected from the version-anchored
// OpenAPI spec in the sibling unifi-specs repository.
//
// Types only. The ergonomic, hand-written client (groups, CRUD, the v2 write
// contract, capability gating) lives in the parent `network` package and picks
// the types it needs from here. The generated operation layer is intentionally
// not produced: it keys off reverse-engineered operationIds/summaries that are
// noisy, whereas the type surface (field names, enums, validation) is clean.
//
// The output is unversioned on purpose. The provider tracks the latest modeled
// controller; there are no per-version client packages. On a firmware/spec
// bump, re-point the path below at the new version's openapi.yaml and run
// `go generate ./...`; review the diff. (Per project decision: the spec is
// versioned, the client is not — new controllers are expected to be backward
// compatible, and the provider supports the latest OS.)
//
//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../../unifi-specs/network/v10.4.57/openapi.yaml
package wire
