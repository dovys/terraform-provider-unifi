package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// WAN uplinks are networkconf documents (purpose=wan), so they share the
// networkconf sweeper and CheckDestroy registered in network_resource_acc_test.go.
// The test targets WAN2 to avoid disturbing the primary uplink on a sim/Docker
// controller.
func TestAccWANResource(t *testing.T) {
	name := accNamePrefix + "wan2"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccWANConfig(name, "dhcp", "weighted"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_wan.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "wan_type", "dhcp"),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "network_group", "WAN2"),
					resource.TestCheckResourceAttr("unifi_network_wan.test", "load_balance_type", "weighted"),
					resource.TestCheckResourceAttrSet("unifi_network_wan.test", "id"),
				),
			},
			{ // update: switch to failover-only
				Config: testAccWANConfig(name, "dhcp", "failover-only"),
				Check:  resource.TestCheckResourceAttr("unifi_network_wan.test", "load_balance_type", "failover-only"),
			},
			{ // import
				ResourceName:      "unifi_network_wan.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // second consecutive plan must be empty
				Config: testAccWANConfig(name, "dhcp", "failover-only"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccWANConfig(name, wanType, lb string) string {
	return fmt.Sprintf(`
resource "unifi_network_wan" "test" {
  name              = %q
  wan_type          = %q
  network_group     = "WAN2"
  load_balance_type = %q
  dns_servers       = ["1.1.1.1", "8.8.8.8"]
}
`, name, wanType, lb)
}
