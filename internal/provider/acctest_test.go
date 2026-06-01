package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

// accNamePrefix is the mandatory prefix for every object an acceptance test
// creates, so sweepers can find and delete leaked objects without touching
// anything an operator made by hand (testing.md, Tier 2).
const accNamePrefix = "tfacc-"

// testAccProtoV6ProviderFactories wires the in-process provider for acceptance
// tests under the "unifi" name.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"unifi": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck fails fast (before any plan) if the controller env isn't set.
// Tier-2 tests need a real (Docker/sim) controller; without it they can't run.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, env := range []string{"UNIFI_HOST"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests (see docs/testing.md)", env)
		}
	}
	// A local admin (username+password) is required.
	if os.Getenv("UNIFI_USERNAME") == "" || os.Getenv("UNIFI_PASSWORD") == "" {
		t.Fatal("set UNIFI_USERNAME and UNIFI_PASSWORD (a local UniFi OS admin) for acceptance tests")
	}
}

// sweepClient builds a client from the acceptance env for sweepers.
func sweepClient() (unifi.Client, error) {
	host := os.Getenv("UNIFI_HOST")
	if host == "" {
		return nil, fmt.Errorf("UNIFI_HOST not set")
	}
	return unifi.NewClient(context.Background(), unifi.Config{
		Host:          host,
		Username:      os.Getenv("UNIFI_USERNAME"),
		Password:      os.Getenv("UNIFI_PASSWORD"),
		Site:          envOr("UNIFI_SITE", "default"),
		AllowInsecure: strings.EqualFold(os.Getenv("UNIFI_INSECURE"), "true") || os.Getenv("UNIFI_INSECURE") == "1",
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestMain(m *testing.M) {
	resource.TestMain(m)
}
