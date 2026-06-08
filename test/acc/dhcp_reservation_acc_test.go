//go:build acc

package acc

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

const tfaccReservationMAC = "02:00:00:aa:bb:01"

func TestAccDHCPReservationResource(t *testing.T) {
	name := acctest.NamePrefix + "resv"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckReservationCleared,
		Steps: []resource.TestStep{
			{
				Config: testAccReservationConfig(name, "192.0.2.50"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_dhcp_reservation.test", "mac", tfaccReservationMAC),
					resource.TestCheckResourceAttr("unifi_network_dhcp_reservation.test", "ip_address", "192.0.2.50"),
					resource.TestCheckResourceAttrSet("unifi_network_dhcp_reservation.test", "id"),
				),
			},
			{
				Config: testAccReservationConfig(name, "192.0.2.51"),
				Check:  resource.TestCheckResourceAttr("unifi_network_dhcp_reservation.test", "ip_address", "192.0.2.51"),
			},
			{
				ResourceName:      "unifi_network_dhcp_reservation.test",
				ImportState:       true,
				ImportStateId:     tfaccReservationMAC,
				ImportStateVerify: true,
			},
			{
				Config:           testAccReservationConfig(name, "192.0.2.51"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccReservationConfig(name, ip string) string {
	return fmt.Sprintf(`
resource "unifi_network_network" "resvnet" {
  name         = "%[1]s-net"
  purpose      = "corporate"
  vlan_id      = 3993
  subnet       = "192.0.2.1/24"
  dhcp_enabled = true
  dhcp_start   = "192.0.2.100"
  dhcp_stop    = "192.0.2.200"
}

resource "unifi_network_dhcp_reservation" "test" {
  mac        = %[2]q
  ip_address = %[3]q
  network_id = unifi_network_network.resvnet.id
  name       = %[1]q
}
`, name, tfaccReservationMAC, ip)
}

// Destroy clears the reservation but leaves the client record (safe by default),
// so "destroyed" means the fixed IP is gone, not that the client is.
func testAccCheckReservationCleared(*terraform.State) error {
	c, err := acctest.Client()
	if err != nil {
		return err
	}
	u, err := c.Network().Clients().GetByMAC(context.Background(), tfaccReservationMAC)
	if acctest.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.UseFixedip != nil && *u.UseFixedip {
		return fmt.Errorf("reservation for %s still has a fixed IP after destroy", tfaccReservationMAC)
	}
	return nil
}

func init() {
	acctest.RegisterSweeper("unifi_network_dhcp_reservation",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			u, err := c.Network().Clients().GetByMAC(ctx, tfaccReservationMAC)
			if acctest.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			// Surface it for sweeping only while it still holds a fixed IP.
			if u.UseFixedip == nil || !*u.UseFixedip {
				return nil, nil
			}
			return []acctest.SweepItem{{ID: acctest.Deref(u.UnderscoreId), Name: acctest.NamePrefix + "resv"}}, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			off := false
			_, err := c.Network().Clients().Users().Update(ctx, id, &wire.User{UnderscoreId: &id, UseFixedip: &off})
			return err
		})
}
