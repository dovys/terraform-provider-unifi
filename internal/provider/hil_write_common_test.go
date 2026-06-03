//go:build hil

// Write-verification HIL tier (bucket 2) — see docs/testing.md §"Write verification".
//
// These tests CREATE objects on the operator's LIVE console, verify the write
// path against real firmware, then destroy them. They are safe by construction:
// every object is additive (never mutates an existing object), inert (zero
// effect on live traffic/RF/clients while it exists), and guaranteed torn down
// (resource.Test destroys after the steps; the tfacc- sweeper is the backstop).
//
// They never run by accident: the //go:build hil tag keeps them out of `make
// test` and Tier-2 CI, and hilWriteGate refuses to run without an explicit
// opt-in plus a pinned target host. Drive them through test/hil/write.sh, which
// adds an interactive confirmation and the pre/post sweep.
package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

// tfaccHILPrefix names every object the write tier creates on the live console.
// It extends the Tier-2 accNamePrefix ("tfacc-"), so the existing "tfacc-"
// sweepers already match anything this tier leaks — tfaccHILPrefix just marks
// it as live-console write-test debris specifically.
const tfaccHILPrefix = accNamePrefix + "hil-" // "tfacc-hil-"

// Documentation prefixes (RFC 5737 / RFC 3849). Nothing on a real network
// routes to these, so a static/traffic route or port-forward target pointed at
// one carries no live traffic — the inertness lever for W1 routing resources.
const (
	blackholeV4   = "192.0.2.0/24"    // TEST-NET-1
	blackholeV4b  = "198.51.100.0/24" // TEST-NET-2
	blackholeV4IP = "192.0.2.1"       // a host in TEST-NET-1 (route next-hop / forward target)
	blackholeV6   = "2001:db8::/32"   // RFC 3849 documentation range
)

// hilTestMAC returns a stable, locally-administered unicast MAC for a write
// test (the 0x02 bit means "locally administered", so it can't collide with a
// real vendor OUI). It is asserted absent from active clients before use
// (requireMACNotActive) so a fixed-IP reservation we create binds to nothing.
func hilTestMAC(suffix byte) string {
	return fmt.Sprintf("02:00:5e:de:ad:%02x", suffix)
}

// hilWriteGate is the in-code backstop for every write test. It refuses to run
// unless the operator has opted in AND pinned the exact target host — so a
// stray UNIFI_HOST (e.g. a production console) can never be written to even if
// UNIFI_HIL_ALLOW_WRITES is left set. The human-facing "type 'I understand'"
// confirmation and the pre/post sweep live in test/hil/write.sh.
func hilWriteGate(t *testing.T, tier string) {
	t.Helper()
	if os.Getenv("UNIFI_HIL_ALLOW_WRITES") != "1" {
		t.Skip("write-verification HIL tier is opt-in: set UNIFI_HIL_ALLOW_WRITES=1 and run via test/hil/write.sh")
	}
	testAccPreCheck(t) // UNIFI_HOST + local-admin UNIFI_USERNAME/UNIFI_PASSWORD
	host := os.Getenv("UNIFI_HOST")
	if pinned := os.Getenv("UNIFI_HIL_WRITE_HOST"); pinned == "" || pinned != host {
		t.Fatalf("host allowlist: set UNIFI_HIL_WRITE_HOST to exactly the console you intend to write to "+
			"(UNIFI_HOST=%q, UNIFI_HIL_WRITE_HOST=%q)", host, pinned)
	}
	t.Logf("HIL WRITE [%s] → %s — additive + inert + tfacc-hil-; auto-destroy + sweep backstop", tier, host)
}

// assertInert fails before any apply if a rendered config isn't inert by
// construction: it must name its objects with tfaccHILPrefix and must contain
// every token the resource relies on to stay inert (e.g. "enabled = false", a
// blackhole prefix). This makes "inert" a literal precondition a careless future
// edit can't slip past — not a convention (docs/testing.md rail 2).
func assertInert(t *testing.T, config string, mustContain ...string) {
	t.Helper()
	if !strings.Contains(config, tfaccHILPrefix) {
		t.Fatalf("inertness: config must name objects with %q:\n%s", tfaccHILPrefix, config)
	}
	for _, tok := range mustContain {
		if !strings.Contains(config, tok) {
			t.Fatalf("inertness: config must contain %q to stay inert:\n%s", tok, config)
		}
	}
}

// hilWriteTest describes one resource's write-verification lifecycle. runHILWrite
// turns it into the standard create → update-one-field → import(verify) →
// second-empty-plan sequence, with destroy + CheckDestroy handled by the
// framework after the steps.
type hilWriteTest struct {
	tier         string // "W0" | "W1" | "W2" (logged; all share one gate)
	resourceAddr string // e.g. "unifi_network_firewall_group.test"

	createConfig string
	createChecks []resource.TestCheckFunc
	updateConfig string // identical but one field changed (still inert)
	updateChecks []resource.TestCheckFunc

	// inertTokens are asserted present in BOTH configs before anything runs
	// (assertInert). The tfacc-hil- prefix is always checked implicitly.
	inertTokens []string

	// preApply is an optional live-state precondition run before the create
	// step (e.g. assert a test MAC isn't an active client). May be nil.
	preApply func(t *testing.T)

	checkDestroy resource.TestCheckFunc
	// importIgnore lists attributes ImportStateVerify must skip — write-only
	// secrets (x_secret, x_password) that the controller never returns.
	importIgnore []string
}

// registerHILSweeper wires a sweeper for the write tier. It only ever deletes
// objects whose natural-key carries tfaccHILPrefix (the sweep callbacks enforce
// that), so it can never touch an operator-made object. Registered from the
// hil-tagged files' init(), so it exists only under `-tags hil`.
func registerHILSweeper(resourceType string, sweep func(ctx context.Context, c unifi.Client) error) {
	resource.AddTestSweepers(resourceType, &resource.Sweeper{
		Name: resourceType,
		F: func(string) error {
			c, err := sweepClient()
			if err != nil {
				return err
			}
			return sweep(context.Background(), c)
		},
	})
}

// checkDestroyByGet asserts every state resource of resourceType is gone via a
// Get-by-id that must return not-found (legacy + Protect collections that expose
// Get). The framework destroys after the steps; this verifies it took.
func checkDestroyByGet(resourceType string, get func(ctx context.Context, c unifi.Client, id string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := sweepClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			err := get(ctx, c, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("%s %s still exists after destroy", resourceType, rs.Primary.ID)
			}
			if !isNotFoundErr(err) {
				return fmt.Errorf("checking destroy of %s %s: %w", resourceType, rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// checkDestroyByList asserts every state resource of resourceType is gone for the
// v2 collections that have no Get — listIDs returns the live collection's _ids.
func checkDestroyByList(resourceType string, listIDs func(ctx context.Context, c unifi.Client) ([]string, error)) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := sweepClient()
		if err != nil {
			return err
		}
		ids, err := listIDs(context.Background(), c)
		if err != nil {
			return err
		}
		live := make(map[string]bool, len(ids))
		for _, id := range ids {
			live[id] = true
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			if live[rs.Primary.ID] {
				return fmt.Errorf("%s %s still exists after destroy", resourceType, rs.Primary.ID)
			}
		}
		return nil
	}
}

func runHILWrite(t *testing.T, tc hilWriteTest) {
	t.Helper()
	hilWriteGate(t, tc.tier)
	for _, cfg := range []string{tc.createConfig, tc.updateConfig} {
		assertInert(t, cfg, tc.inertTokens...)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             tc.checkDestroy,
		Steps: []resource.TestStep{
			{ // create: verify the write path + field round-trip against live firmware
				PreConfig: func() {
					if tc.preApply != nil {
						tc.preApply(t)
					}
				},
				Config: tc.createConfig,
				Check:  resource.ComposeAggregateTestCheckFunc(tc.createChecks...),
			},
			{ // update one field: exercise the update/merge path
				Config: tc.updateConfig,
				Check:  resource.ComposeAggregateTestCheckFunc(tc.updateChecks...),
			},
			{ // import the live object: must match state exactly
				ResourceName:            tc.resourceAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: tc.importIgnore,
			},
			{ // idempotency: a second plan over the same config must be empty
				Config: tc.updateConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
