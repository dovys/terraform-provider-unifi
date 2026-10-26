// Package wire holds the generated Go types for the UniFi Protect private API:
// structs and enum constants, generated from the maintainer's version-pinned
// API spec (not public).
//
// Types only. The ergonomic, hand-written client (groups, GET-merge-PATCH, the
// capability gating) lives in the parent `protect` package and picks the types
// it needs from here. The generated operation layer is intentionally not
// produced.
//
// The output is unversioned on purpose (see the Network wire package): the
// provider tracks the latest modeled controller; on a firmware/spec bump,
// re-point the path below at the new version and regenerate.
//
// The Protect spec is OpenAPI 3.1, which the 3.0-oriented generator rejects on
// two idioms (numeric exclusiveMinimum/Maximum; `oneOf:[ref,{type:null}]`
// nullables); its 378 schemas also include hundreds of runtime/event types the
// provider never models, and inline enums/objects whose lifted names collide.
// So scripts/wire-gen.sh first runs a deterministic projection shipped with the
// spec: downgrade 3.1->3.0, closure-trim to the modeled roots, hoist inline
// enums/objects to uniquely-named components — then codegens from that.
//
// The roots are listed explicitly (not left to the projector's defaults): they
// are exactly the resources this provider models, and an omission silently drops
// a resource's types on regen instead of erroring.
//
//go:generate bash ../../../../scripts/wire-gen.sh $UNIFI_SPECS/protect/v7.1.69/openapi.yaml $UNIFI_SPECS/tools/protect_codegen_spec.py camera liveview nvr light sensor chime viewer bridge speaker nvrUpdate chimeUpdate talkbackSettingsUpdate automation
package wire
