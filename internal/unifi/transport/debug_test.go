package transport

import (
	"strings"
	"testing"
)

// TestRedactJSON proves the wire-debug dump masks secret-bearing fields at any
// depth (the login body, write-only passphrases the controller echoes) while
// leaving ordinary fields readable for reverse-engineering.
func TestRedactJSON(t *testing.T) {
	in := []byte(`{
		"username": "network-admin",
		"password": "hunter2",
		"x_passphrase": "wifi-secret",
		"name": "Internet 2",
		"nested": {"wpa_psk": "psk-secret", "vlan": 99},
		"list": [{"auth_token": "abc"}, {"keep": "ok"}]
	}`)
	out := redactJSON(in)

	for _, leaked := range []string{"hunter2", "wifi-secret", "psk-secret", "abc"} {
		if strings.Contains(out, leaked) {
			t.Errorf("secret %q leaked into debug output:\n%s", leaked, out)
		}
	}
	for _, kept := range []string{"network-admin", "Internet 2", "99", "ok"} {
		if !strings.Contains(out, kept) {
			t.Errorf("non-secret %q missing from debug output:\n%s", kept, out)
		}
	}
	if strings.Count(out, "***REDACTED***") != 4 {
		t.Errorf("want 4 redactions (password, x_passphrase, wpa_psk, auth_token), got:\n%s", out)
	}
}

func TestRedactJSONNonJSON(t *testing.T) {
	if got := redactJSON([]byte("not json")); got != "not json" {
		t.Errorf("non-JSON body should pass through, got %q", got)
	}
}
