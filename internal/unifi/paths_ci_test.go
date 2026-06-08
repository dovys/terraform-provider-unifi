package unifi

import "testing"

func TestPathBuilders(t *testing.T) {
	if got := legacyPath("default", "rest/networkconf"); got != "/proxy/network/api/s/default/rest/networkconf" {
		t.Errorf("legacyPath = %q", got)
	}
	if got := legacyPath("default", "/stat/sysinfo"); got != "/proxy/network/api/s/default/stat/sysinfo" {
		t.Errorf("legacyPath (leading slash) = %q", got)
	}
}
