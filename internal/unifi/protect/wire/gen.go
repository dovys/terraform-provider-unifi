// Package wire holds the generated Go types for the UniFi Protect private API:
// structs and enum constants, projected from the version-anchored OpenAPI spec
// in the sibling unifi-specs repository (the spec Protect itself ships, derived
// not decompiled — see ../../../../../unifi-specs/protect/v7.1.69/SOURCE.md).
//
// Types only. The ergonomic, hand-written client (groups, GET-merge-PATCH, the
// capability gating) lives in the parent `protect` package and picks the types
// it needs from here. The generated operation layer is intentionally not
// produced.
//
// The output is unversioned on purpose (see the Network wire package): the
// provider tracks the latest modeled controller; on a firmware/spec bump,
// re-point the path below at the new version's openapi.yaml and regenerate.
//
// Protect ships OpenAPI 3.1, which the 3.0-oriented generator rejects on two
// idioms (numeric exclusiveMinimum/Maximum; `oneOf:[ref,{type:null}]`
// nullables); its 378 schemas also include hundreds of runtime/event types the
// provider never models, and inline enums/objects whose lifted names collide.
// The :generate step runs a deterministic projection
// (unifi-specs/tools/protect_codegen_spec.py): downgrade 3.1->3.0, closure-trim
// to the modeled roots, hoist inline enums/objects to uniquely-named
// components, into a temp file — then codegens from that. The committed spec
// stays canonical 3.1; only this focused projection feeds codegen.
//
// The roots are listed explicitly (not left to the projector's defaults): they
// are exactly the resources this provider models, and an omission silently drops
// a resource's types on regen instead of erroring.
//
//go:generate sh -c "uv run ../../../../../unifi-specs/tools/protect_codegen_spec.py ../../../../../unifi-specs/protect/v7.1.69/openapi.yaml /tmp/protect-codegen.yaml camera liveview nvr light sensor chime viewer bridge speaker nvrUpdate chimeUpdate talkbackSettingsUpdate automation && go tool oapi-codegen -config oapi-codegen.yaml /tmp/protect-codegen.yaml"
package wire
