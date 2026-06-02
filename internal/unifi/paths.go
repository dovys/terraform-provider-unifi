package unifi

import (
	"fmt"
	"strings"
)

// legacyPath builds a console-level legacy API path. The proxy prefix lives here
// in one place; the site argument is the slug (the legacy/v2 name, e.g.
// "default"). Network-app paths are built inside the network substruct client.
func legacyPath(site, suffix string) string {
	return fmt.Sprintf("/proxy/network/api/s/%s/%s", site, strings.TrimPrefix(suffix, "/"))
}
