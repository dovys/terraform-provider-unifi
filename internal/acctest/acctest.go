// Package acctest holds the shared harness for the provider's black-box test
// packages — Tier-2 acceptance (test/acc) and Tier-3 HIL (test/hil). Both drive
// the provider through its public Terraform protocol, so the helpers here (the
// in-process provider factory, the env precheck, an env-built client, and the
// sweep/CheckDestroy plumbing) need nothing unexported from the provider.
//
// It is imported only by those tagged test packages, so it never links into the
// provider binary despite importing testing.
package acctest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dovys/terraform-provider-unifi/internal/provider"
	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

// NamePrefix is the mandatory prefix for every object an acceptance/HIL test
// creates, so sweepers find and delete leaked objects without touching anything
// an operator made by hand (testing.md).
const NamePrefix = "tfacc-"

// ProtoV6ProviderFactories wires the in-process provider under the "unifi" name.
var ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"unifi": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// PreCheck fails fast (before any plan) if the controller env isn't set. Tier-2
// runs against the Docker controller; a local admin (username+password) is
// required.
func PreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("UNIFI_HOST") == "" {
		t.Fatal("UNIFI_HOST must be set for acceptance tests (see docs/testing.md)")
	}
	if os.Getenv("UNIFI_USERNAME") == "" || os.Getenv("UNIFI_PASSWORD") == "" {
		t.Fatal("set UNIFI_USERNAME and UNIFI_PASSWORD (a local UniFi OS admin) for acceptance tests")
	}
}

// Client builds a client from the acceptance env, for sweepers and CheckDestroy.
func Client() (unifi.Client, error) {
	return unifi.NewClient(context.Background(), unifi.Config{
		Host:          os.Getenv("UNIFI_HOST"),
		Username:      os.Getenv("UNIFI_USERNAME"),
		Password:      os.Getenv("UNIFI_PASSWORD"),
		Site:          EnvOr("UNIFI_SITE", "default"),
		AllowInsecure: strings.EqualFold(os.Getenv("UNIFI_ALLOW_INSECURE"), "true") || os.Getenv("UNIFI_ALLOW_INSECURE") == "1",
	})
}

// Deref returns the pointed-to string, or "" for a nil pointer.
func Deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// EnvOr returns the value of env var key, or def if it is unset/empty.
func EnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
