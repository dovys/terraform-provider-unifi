package unifi

import (
	"fmt"
	"strings"
)

// Surface path builders. The proxy prefixes live here in one place; router.go
// decides which surface an operation uses, and these build the URL for it. The
// site argument is the slug (legacy/v2 name, e.g. "default") for legacy/v2;
// Integration paths that need a site take the UUID in the suffix.

func legacyPath(site, suffix string) string {
	return fmt.Sprintf("/proxy/network/api/s/%s/%s", site, strings.TrimPrefix(suffix, "/"))
}

func v2Path(site, suffix string) string {
	return fmt.Sprintf("/proxy/network/v2/api/site/%s/%s", site, strings.TrimPrefix(suffix, "/"))
}

func integrationPath(suffix string) string {
	return "/proxy/network/integration/v1/" + strings.TrimPrefix(suffix, "/")
}
