package unifi

import "testing"

func TestPathBuilders(t *testing.T) {
	if got := legacyPath("default", "rest/networkconf"); got != "/proxy/network/api/s/default/rest/networkconf" {
		t.Errorf("legacyPath = %q", got)
	}
	if got := legacyPath("default", "/stat/sysinfo"); got != "/proxy/network/api/s/default/stat/sysinfo" {
		t.Errorf("legacyPath (leading slash) = %q", got)
	}
	if got := v2Path("default", "static-dns"); got != "/proxy/network/v2/api/site/default/static-dns" {
		t.Errorf("v2Path = %q", got)
	}
}
