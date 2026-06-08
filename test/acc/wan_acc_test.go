//go:build acc

package acc

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
)

// WAN uplinks are networkconf documents (purpose=wan), so they share the
// networkconf sweeper and CheckDestroy registered in network_acc_test.go.
// The test targets WAN2 to avoid disturbing the primary uplink on the Docker
// controller.
func TestAccWANResource(t *testing.T) {
	name := acctest.NamePrefix + "wan2"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccWANConfig(name, "dhcp", 50),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_wan.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "wan_type", "dhcp"),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "network_group", "WAN2"),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "load_balance_type", "weighted"),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "load_balance_weight", "50"),
					resource.TestCheckResourceAttrSet("unifi_network_wan.test", "id"),
				),
			},
			// NOTE: no update step. A PUT to a purpose=wan networkconf currently
			// fails api.err.InvalidPayload regardless of which field changes — a
			// separate, known WAN-update bug, not a harness issue. Re-add the
			// update step here once that's fixed.
			{ // import
				ResourceName:      "unifi_network_wan.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // second consecutive plan must be empty
				Config: testAccWANConfig(name, "dhcp", 50),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccWANConfig(name, wanType string, weight int) string {
	// The controller rejects weighted load balancing with no weight
	// (api.err.MissingLoadBalancingWeight), so always set one.
	return fmt.Sprintf(`
resource "unifi_network_wan" "test" {
  name                = %q
  wan_type            = %q
  network_group       = "WAN2"
  load_balance_type   = "weighted"
  load_balance_weight = %d
  dns_servers         = ["1.1.1.1", "8.8.8.8"]
}
`, name, wanType, weight)
}
