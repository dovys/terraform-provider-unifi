// Package models holds typed request/response structs for the UniFi API
// surfaces. Structs are intentionally minimal — we model the fields the
// provider owns and carry everything else through as raw JSON where a
// GET-merge-PUT write requires it (development.md, recipe step 2).
package models

// LegacyEnvelope wraps every legacy REST/stat response:
// {"meta":{"rc":"ok"},"data":[...]}.
type LegacyEnvelope[T any] struct {
	Meta LegacyMeta `json:"meta"`
	Data []T        `json:"data"`
}

// LegacyMeta is the status block on a legacy response. RC is "ok" on success;
// on failure it is "error" and Msg carries the controller's reason.
type LegacyMeta struct {
	RC  string `json:"rc"`
	Msg string `json:"msg,omitempty"`
}

// OK reports whether the legacy call succeeded.
func (m LegacyMeta) OK() bool { return m.RC == "ok" }

// IntegrationPage is one page of an Integration v1 list response:
// {"offset":0,"limit":25,"count":1,"totalCount":1,"data":[...]}.
type IntegrationPage[T any] struct {
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
	Count      int `json:"count"`
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}
