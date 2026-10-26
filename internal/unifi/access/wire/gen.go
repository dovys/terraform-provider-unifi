// Package wire holds the generated Go types for the UniFi Access private API:
// structs and enum constants, generated from the maintainer's version-pinned
// API spec (not public).
//
// Types only. The ergonomic, hand-written client (groups, the {code,msg,data}
// envelope, CRUD) lives in the parent `access` package and picks the types it
// needs from here. The generated operation layer is intentionally not produced.
//
// The output is unversioned on purpose (see the Network wire package): the
// provider tracks the latest modeled controller; on a firmware/spec bump,
// re-point the path below at the new version and regenerate.
//
// The spec has 2018 components (most are runtime/event types the provider never
// models), so scripts/wire-gen.sh first runs a deterministic projection shipped
// with the spec: closure-trim to the modeled roots + hoist inline enums/objects
// to uniquely-named components — then codegens from that.
//
//go:generate bash ../../../../scripts/wire-gen.sh $UNIFI_SPECS/access/v4.2.27/openapi.yaml $UNIFI_SPECS/tools/access_codegen_spec.py
package wire
