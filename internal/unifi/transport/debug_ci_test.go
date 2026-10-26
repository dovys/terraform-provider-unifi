package transport

import (
	"fmt"
	"strings"
	"testing"
)

// TestRedactJSON proves the wire-debug dump masks secret-bearing fields at any
// depth (the login body, write-only passphrases the controller echoes) while
// leaving ordinary fields readable for debugging.
func TestRedactJSON(t *testing.T) {
	in := []byte(`{
		"username": "network-admin",
		"password": "hunter2",
		"x_passphrase": "wifi-secret",
		"name": "Internet 2",
		"key": "host.example.com",
		"x_ipsec_pre_shared_key": "ipsec-secret",
		"x_ca_key": "ca-private-material",
		"wireguard_client_preshared_key": "wg-secret",
		"nested": {"wpa_psk": "psk-secret", "vlan": 99},
		"list": [{"auth_token": "abc"}, {"keep": "ok"}]
	}`)
	out := redactJSON(in)

	for _, leaked := range []string{"hunter2", "wifi-secret", "psk-secret", "abc", "ipsec-secret", "ca-private-material", "wg-secret"} {
		if strings.Contains(out, leaked) {
			t.Errorf("secret %q leaked into debug output:\n%s", leaked, out)
		}
	}
	// "key" here is a static-DNS hostname, not key material — must survive.
	for _, kept := range []string{"network-admin", "Internet 2", "99", "ok", "host.example.com"} {
		if !strings.Contains(out, kept) {
			t.Errorf("non-secret %q missing from debug output:\n%s", kept, out)
		}
	}
	if strings.Count(out, "***REDACTED***") != 7 {
		t.Errorf("want 7 redactions (password, x_passphrase, wpa_psk, auth_token, ipsec/ca/wireguard keys), got:\n%s", out)
	}
}

// TestRedactJSONConsoleCertificate proves the console-certificate surface's
// secrets are masked: the camelCase DNS-provider credentials (by field name) and
// the BYO-upload private key (by value — it rides under the bare field "key",
// which is not a secret name). The public certificate and non-secret discriminators
// stay readable.
func TestRedactJSONConsoleCertificate(t *testing.T) {
	in := []byte(`{
		"name": "console",
		"domain": "console.example.com",
		"dnsProvider": {
			"provider": "ovh",
			"apiToken": "cf-token",
			"apiKey": "dynu-key",
			"apiSecret": "godaddy-secret",
			"secretKey": "porkbun-secret",
			"accessKeyId": "AKIAEXAMPLE",
			"secretAccessKey": "aws-secret",
			"applicationKey": "ovh-app-key",
			"applicationSecret": "ovh-app-secret",
			"consumerKey": "ovh-consumer",
			"region": "eu-west-1",
			"endpoint": "ovh-eu"
		},
		"cert": "-----BEGIN CERTIFICATE-----\nMIIPUBLIC\n-----END CERTIFICATE-----",
		"key": "-----BEGIN PRIVATE KEY-----\nMIISECRET\n-----END PRIVATE KEY-----"
	}`)
	out := redactJSON(in)

	for _, leaked := range []string{
		"cf-token", "dynu-key", "godaddy-secret", "porkbun-secret", "AKIAEXAMPLE",
		"aws-secret", "ovh-app-key", "ovh-app-secret", "ovh-consumer", "MIISECRET",
	} {
		if strings.Contains(out, leaked) {
			t.Errorf("console-cert secret %q leaked into debug output:\n%s", leaked, out)
		}
	}
	// Non-secret discriminators and the public certificate must survive.
	for _, kept := range []string{"console", "console.example.com", "ovh", "eu-west-1", "ovh-eu", "MIIPUBLIC"} {
		if !strings.Contains(out, kept) {
			t.Errorf("non-secret %q missing from debug output:\n%s", kept, out)
		}
	}
}

// TestRedactJSONNonJSON: a body that fails to parse can't be field-redacted, so
// none of its content may reach the log — only its length and sniffed type.
func TestRedactJSONNonJSON(t *testing.T) {
	cases := map[string]struct {
		body     string
		wantType string
	}{
		"form":      {"username=admin&password=hunter2", "text/plain"},
		"html":      {"<html><body>token=hunter2</body></html>", "text/html"},
		"truncated": {`{"password":"hunter2"`, "text/plain"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := redactJSON([]byte(tc.body))
			if strings.Contains(got, "hunter2") || strings.Contains(got, "admin") {
				t.Errorf("non-JSON body content leaked: %q", got)
			}
			want := fmt.Sprintf("%d bytes", len(tc.body))
			if !strings.Contains(got, want) || !strings.Contains(got, tc.wantType) {
				t.Errorf("redactJSON = %q, want length %q and type %q", got, want, tc.wantType)
			}
		})
	}
}
