package models

// Sysinfo is the subset of the object returned by legacy GET /stat/sysinfo that
// the provider needs. The Network application version lives here and is present
// whenever the classic (cookie) session is authenticated — unlike the
// Integration application-info endpoint, which 404s on older firmware
// (architecture.md §5).
type Sysinfo struct {
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
	Name    string `json:"name,omitempty"`
}
