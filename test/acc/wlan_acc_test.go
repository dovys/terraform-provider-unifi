//go:build acc

package acc

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

func init() {
	acctest.RegisterSweeper("unifi_network_wlan",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ws, err := c.Network().WiFi().WLANs().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(ws))
			for i := range ws {
				out[i] = acctest.SweepItem{ID: acctest.Deref(ws[i].UnderscoreId), Name: acctest.Deref(ws[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().WiFi().WLANs().Delete(ctx, id)
		})
}

func TestAccWLANResource(t *testing.T) {
	name := acctest.NamePrefix + "wlan"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_wlan", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().WiFi().WLANs().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccWLANConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_wlan.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_wlan.test", "security", "wpapsk"),
					resource.TestCheckResourceAttr("unifi_network_wlan.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("unifi_network_wlan.test", "id"),
				),
			},
			{
				Config: testAccWLANConfig(name, false),
				Check:  resource.TestCheckResourceAttr("unifi_network_wlan.test", "enabled", "false"),
			},
			{
				ResourceName:            "unifi_network_wlan.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"passphrase"}, // write-preserving secret
			},
			{
				Config:           testAccWLANConfig(name, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccWLANConfig(name string, enabled bool) string {
	return fmt.Sprintf(`
resource "unifi_network_network" "wlannet" {
  name    = "%[1]s-net"
  purpose = "corporate"
  vlan_id = 3992
  subnet  = "203.0.113.1/24"
}

resource "unifi_network_ap_group" "wlan" {
  name = "%[1]s-apg"
}

resource "unifi_network_wlan" "test" {
  name         = %[1]q
  network_id   = unifi_network_network.wlannet.id
  security     = "wpapsk"
  passphrase   = "tfacc-wlan-pw-123"
  enabled      = %[2]t
  bands        = ["2g", "5g"]
  ap_group_ids = [unifi_network_ap_group.wlan.id]
}
`, name, enabled)
}
