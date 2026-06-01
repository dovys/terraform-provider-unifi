package models

// ApplicationInfo is the Integration v1 `/application-info` response. Present
// on Network 9.3+ (404s below). Used by the data source and as a version
// fallback when only the Integration API key is configured.
type ApplicationInfo struct {
	ApplicationVersion string `json:"applicationVersion"`
}
