package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

func init() {
	// One sweeper covers every networkconf (LAN/VLAN/guest and WAN) — they share
	// the list/delete surface. Deletes only objects carrying the tfacc- prefix.
	resource.AddTestSweepers("unifi_network_network", &resource.Sweeper{
		Name: "unifi_network_network",
		F: func(_ string) error {
			c, err := sweepClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			nets, err := c.ListNetworks(ctx)
			if err != nil {
				return err
			}
			for i := range nets {
				if !strings.HasPrefix(nets[i].Name, accNamePrefix) {
					continue
				}
				if err := c.DeleteNetwork(ctx, nets[i].ID); err != nil {
					return fmt.Errorf("sweep network %s (%s): %w", nets[i].Name, nets[i].ID, err)
				}
			}
			return nil
		},
	})
}

func TestAccNetworkResource(t *testing.T) {
	name := accNamePrefix + "net"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccNetworkConfig(name, 3990, "192.0.2.1/24", `["1.1.1.1"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_network.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_network.test", "vlan_id", "3990"),
					resource.TestCheckResourceAttr("unifi_network_network.test", "purpose", "corporate"),
					resource.TestCheckResourceAttrSet("unifi_network_network.test", "id"),
					resource.TestCheckResourceAttrSet("unifi_network_network.test", "network_group"),
				),
			},
			{ // update: rename + change DHCP DNS
				Config: testAccNetworkConfig(name+"2", 3990, "192.0.2.1/24", `["1.1.1.1", "9.9.9.9"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_network.test", "name", name+"2"),
					resource.TestCheckResourceAttr("unifi_network_network.test", "dhcp_dns.#", "2"),
				),
			},
			{ // import
				ResourceName:      "unifi_network_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // second consecutive plan must be empty (idempotency)
				Config: testAccNetworkConfig(name+"2", 3990, "192.0.2.1/24", `["1.1.1.1", "9.9.9.9"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccNetworkConfig(name string, vlan int, subnet, dhcpDNS string) string {
	return fmt.Sprintf(`
resource "unifi_network_network" "test" {
  name    = %q
  purpose = "corporate"
  vlan_id = %d
  subnet  = %q

  dhcp_enabled = true
  dhcp_start   = "192.0.2.100"
  dhcp_stop    = "192.0.2.254"
  dhcp_dns     = %s
}
`, name, vlan, subnet, dhcpDNS)
}

// testAccCheckNetworkDestroy asserts every test-managed network is gone.
func testAccCheckNetworkDestroy(s *terraform.State) error {
	c, err := sweepClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "unifi_network_network" && rs.Type != "unifi_network_wan" {
			continue
		}
		_, err := c.GetNetwork(ctx, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("network %s still exists after destroy", rs.Primary.ID)
		}
		if !errors.Is(err, unifi.ErrNotFound) {
			return fmt.Errorf("checking destroy of %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}
