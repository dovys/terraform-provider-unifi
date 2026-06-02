// Package wire holds the generated Go types for the UniFi Access private API:
// structs and enum constants, projected from the version-anchored OpenAPI spec
// in the sibling unifi-specs repository (reverse-engineered from the Access Go
// binary — see ../../../../../unifi-specs/access/v4.2.27/SOURCE.md).
//
// Types only. The ergonomic, hand-written client (groups, the {code,msg,data}
// envelope, CRUD) lives in the parent `access` package and picks the types it
// needs from here. The generated operation layer is intentionally not produced.
//
// The output is unversioned on purpose (see the Network wire package): the
// provider tracks the latest modeled controller; on a firmware/spec bump,
// re-point the path below at the new version's openapi.yaml and regenerate.
//
// The shipped spec has 2018 components (most are runtime/event types the
// provider never models). The :generate step runs a deterministic projection
// (unifi-specs/tools/access_codegen_spec.py): closure-trim to the modeled
// roots + hoist inline enums/objects to uniquely-named components, into a temp
// file — then codegens from that. The committed spec stays canonical; only this
// focused projection feeds codegen.
//
//go:generate sh -c "uv run ../../../../../unifi-specs/tools/access_codegen_spec.py ../../../../../unifi-specs/access/v4.2.27/openapi.yaml /tmp/access-codegen.yaml && go tool oapi-codegen -config oapi-codegen.yaml /tmp/access-codegen.yaml"
package wire
