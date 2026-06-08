//go:build acc

package acc

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
)

// unifi_network_threat_management is the `ips` settings singleton; like
// unifi_network_settings it always exists, so destroy is a no-op state removal
// (no CheckDestroy, no sweeper, import takes a hand-written config rather than
// ImportStateVerify — see testing.md).
//
// The Tier-2 controller has no gateway, so it CANNOT enable IDS/IPS: it forces
// ips_mode -> "disabled" and clears enabled_categories (verified directly against
// the stack). This lifecycle therefore exercises the CRUD plumbing over the
// round-trippable inert fields (mode "disabled" + the memory_optimized flag); the
// enabled (non-"disabled") path is gateway + CyberSecure-subscription gated and is
// covered by the subscription-refusal step below plus the unit tests. Enabling on
// real licensed hardware was not verifiable — see TODO.md.
func TestAccThreatManagementResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create — engine off, one inert flag set
				Config: testAccThreatManagementConfig("disabled", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_threat_management.test", "mode", "disabled"),
					resource.TestCheckResourceAttr("unifi_network_threat_management.test", "memory_optimized", "true"),
					resource.TestCheckResourceAttrSet("unifi_network_threat_management.test", "id"),
				),
			},
			{ // update the flag — round-trips even on a gateway-less controller
				Config: testAccThreatManagementConfig("disabled", false),
				Check:  resource.TestCheckResourceAttr("unifi_network_threat_management.test", "memory_optimized", "false"),
			},
			{ // second consecutive plan must be empty
				Config:           testAccThreatManagementConfig("disabled", false),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{ // subscription check: enabling without an active CyberSecure subscription
				// must fail with a clear diagnostic, never loop on drift. The gateway-less
				// Tier-2 controller is exactly that case — it lists the IPS feature yet
				// silently keeps the engine off — so the resource's apply-time verification
				// fires here.
				Config:      testAccThreatManagementEnableConfig("ids"),
				ExpectError: regexp.MustCompile(`Threat Management could not be enabled`),
			},
		},
	})
}

func testAccThreatManagementConfig(mode string, memoryOptimized bool) string {
	return fmt.Sprintf(`
resource "unifi_network_threat_management" "test" {
  mode             = %q
  memory_optimized = %t
}
`, mode, memoryOptimized)
}

func testAccThreatManagementEnableConfig(mode string) string {
	return fmt.Sprintf(`
resource "unifi_network_threat_management" "test" {
  mode               = %q
  enabled_categories = ["emerging-malware"]
}
`, mode)
}
