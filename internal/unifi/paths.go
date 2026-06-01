package unifi

import (
	"fmt"
	"strings"
)

// Surface path builders. The proxy prefixes live here in one place. Both
// surfaces run over the same classic (cookie+CSRF) session; the site argument is
// the slug (the legacy/v2 name, e.g. "default").

func legacyPath(site, suffix string) string {
	return fmt.Sprintf("/proxy/network/api/s/%s/%s", site, strings.TrimPrefix(suffix, "/"))
}

func v2Path(site, suffix string) string {
	return fmt.Sprintf("/proxy/network/v2/api/site/%s/%s", site, strings.TrimPrefix(suffix, "/"))
}
