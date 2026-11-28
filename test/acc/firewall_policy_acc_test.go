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
	acctest.RegisterSweeper("unifi_network_firewall_policy",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ps, err := c.Network().Firewall().Policies().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(ps))
			for i := range ps {
				out[i] = acctest.SweepItem{ID: acctest.Deref(ps[i].UnderscoreId), Name: ps[i].Name}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().Firewall().Policies().Delete(ctx, id)
		})
}

// skipUnlessZoneFirewall skips on a classic-firewall site, which the Docker
// controller is; policies need a zone-based one.
func skipUnlessZoneFirewall(t *testing.T) {
	t.Helper()
	c, err := acctest.Client()
	if err != nil {
		t.Fatal(err)
	}
	zbf, err := c.Network().ZoneBasedFirewall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !zbf {
		t.Skip("site runs the classic firewall; firewall policies need the zone-based firewall")
	}
}

// TestAccFirewallPolicyResource_GatewayZone creates ALLOW and BLOCK policies
// into the Gateway zone, and a BLOCK into the Internal zone, with
// create_allow_respond unset; the controller 400s each create unless
// create_allow_respond is sent false (#8). The second step turns an ALLOW
// holding true into a BLOCK, which the PUT must send as false.
func TestAccFirewallPolicyResource_GatewayZone(t *testing.T) {
	name := acctest.NamePrefix + "fwp-gw"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t); skipUnlessZoneFirewall(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_firewall_policy", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().Firewall().Policies().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccFirewallPolicyGatewayConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.block", "name", name+"-block"),
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.block", "create_allow_respond", "false"),
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.allow", "create_allow_respond", "false"),
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.block_internal", "create_allow_respond", "false"),
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.flip", "create_allow_respond", "true"),
					resource.TestCheckResourceAttrSet("unifi_network_firewall_policy.block", "id"),
				),
			},
			{
				Config: testAccFirewallPolicyGatewayConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.block", "logging", "true"),
					resource.TestCheckResourceAttr("unifi_network_firewall_policy.flip", "create_allow_respond", "false"),
				),
			},
			{ResourceName: "unifi_network_firewall_policy.block", ImportState: true, ImportStateVerify: true},
			{ResourceName: "unifi_network_firewall_policy.allow", ImportState: true, ImportStateVerify: true},
			{ResourceName: "unifi_network_firewall_policy.block_internal", ImportState: true, ImportStateVerify: true},
			{ResourceName: "unifi_network_firewall_policy.flip", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccFirewallPolicyGatewayConfig(name, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

// updated sets logging on block and turns flip from ALLOW into BLOCK.
func testAccFirewallPolicyGatewayConfig(name string, updated bool) string {
	flip := "ALLOW"
	if updated {
		flip = "BLOCK"
	}
	return fmt.Sprintf(`
data "unifi_network_firewall_zone" "internal" { zone_key = "internal" }
data "unifi_network_firewall_zone" "gateway" { zone_key = "gateway" }

resource "unifi_network_firewall_policy" "block" {
  name        = "%[1]s-block"
  action      = "BLOCK"
  logging     = %[2]t
  source      = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY" }
  destination = { zone_id = data.unifi_network_firewall_zone.gateway.id, matching_target = "ANY" }
}

resource "unifi_network_firewall_policy" "allow" {
  name        = "%[1]s-allow"
  action      = "ALLOW"
  protocol    = "tcp"
  source      = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY" }
  destination = { zone_id = data.unifi_network_firewall_zone.gateway.id, matching_target = "ANY", port = "443" }
}

resource "unifi_network_firewall_policy" "block_internal" {
  name        = "%[1]s-block-internal"
  action      = "BLOCK"
  source      = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY" }
  destination = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY" }
}

resource "unifi_network_firewall_policy" "flip" {
  name        = "%[1]s-flip"
  action      = "%[3]s"
  protocol    = "tcp"
  source      = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY" }
  destination = { zone_id = data.unifi_network_firewall_zone.internal.id, matching_target = "ANY", port = "8443" }
}
`, name, updated, flip)
}
