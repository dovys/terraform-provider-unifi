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
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// unifi_network_mdns is a site-level singleton (the mDNS reflector scope lives in
// the global network-config document). Destroy is a no-op state removal — the
// document always exists — so CheckDestroy resets the scope to "none" rather than
// asserting the object is gone. The lifecycle exercises none → all → some (the
// last selecting a freshly created network) → import → empty plan.
func TestAccMdnsResource(t *testing.T) {
	netName := acctest.NamePrefix + "mdns-net"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccResetMdns,
		Steps: []resource.TestStep{
			{
				Config: testAccMdnsConfigMode("none"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_mdns.test", "mode", "none"),
					resource.TestCheckResourceAttr("unifi_network_mdns.test", "id", "mdns"),
					resource.TestCheckNoResourceAttr("unifi_network_mdns.test", "network_ids.#"),
				),
			},
			{
				Config: testAccMdnsConfigMode("all"),
				Check:  resource.TestCheckResourceAttr("unifi_network_mdns.test", "mode", "all"),
			},
			{
				Config: testAccMdnsConfigSome(netName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_mdns.test", "mode", "some"),
					resource.TestCheckResourceAttr("unifi_network_mdns.test", "network_ids.#", "1"),
				),
			},
			{
				ResourceName:      "unifi_network_mdns.test",
				ImportState:       true,
				ImportStateId:     "mdns",
				ImportStateVerify: true,
			},
			{
				Config:           testAccMdnsConfigSome(netName),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccMdnsConfigMode(mode string) string {
	return fmt.Sprintf(`
resource "unifi_network_mdns" "test" {
  mode = %q
}
`, mode)
}

func testAccMdnsConfigSome(netName string) string {
	return fmt.Sprintf(`
resource "unifi_network_network" "mdnsnet" {
  name    = %[1]q
  purpose = "corporate"
  vlan_id = 3994
  subnet  = "192.0.2.1/24"
}

resource "unifi_network_mdns" "test" {
  mode        = "some"
  network_ids = [unifi_network_network.mdnsnet.id]
}
`, netName)
}

// testAccResetMdns returns the reflector scope to "none" after the test, so the
// shared throwaway controller doesn't carry the test's selection forward.
func testAccResetMdns(_ *terraform.State) error {
	c, err := acctest.Client()
	if err != nil {
		return err
	}
	ctx := context.Background()
	doc, err := c.Network().GlobalNetwork().Get(ctx)
	if err != nil {
		if acctest.IsNotFound(err) {
			return nil
		}
		return err
	}
	doc.MdnsEnabledFor = wire.GlobalIgmpSnoopingForNone
	doc.MdnsEnabledForNetworkIds = []string{}
	if _, err := c.Network().GlobalNetwork().Set(ctx, doc); err != nil {
		return fmt.Errorf("reset mDNS scope: %w", err)
	}
	return nil
}
