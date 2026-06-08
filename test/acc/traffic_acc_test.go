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
	acctest.RegisterSweeper("unifi_network_traffic_rule",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			rs, err := c.Network().Traffic().Rules().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(rs))
			for i := range rs {
				out[i] = acctest.SweepItem{ID: acctest.Deref(rs[i].UnderscoreId), Name: acctest.Deref(rs[i].Description)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().Traffic().Rules().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_traffic_route",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			rs, err := c.Network().Traffic().Routes().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(rs))
			for i := range rs {
				out[i] = acctest.SweepItem{ID: acctest.Deref(rs[i].UnderscoreId), Name: acctest.Deref(rs[i].Description)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().Traffic().Routes().Delete(ctx, id)
		})
}

func trafficRuleIDs(c unifi.Client, ctx context.Context) ([]string, error) {
	rs, err := c.Network().Traffic().Rules().List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rs))
	for i := range rs {
		ids[i] = acctest.Deref(rs[i].UnderscoreId)
	}
	return ids, nil
}

func TestAccTrafficRuleResource(t *testing.T) {
	desc := acctest.NamePrefix + "trule"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_traffic_rule", func(c unifi.Client, ctx context.Context, id string) error {
			return acctest.AbsentInList(func() ([]string, error) { return trafficRuleIDs(c, ctx) }, id)
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficRuleConfig(desc, `[{ domain = "ads.example.com" }]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_traffic_rule.test", "description", desc),
					resource.TestCheckResourceAttr("unifi_network_traffic_rule.test", "action", "BLOCK"),
					resource.TestCheckResourceAttr("unifi_network_traffic_rule.test", "domains.#", "1"),
					resource.TestCheckResourceAttrSet("unifi_network_traffic_rule.test", "id"),
				),
			},
			{
				Config: testAccTrafficRuleConfig(desc, `[{ domain = "ads.example.com" }, { domain = "tracker.example.net" }]`),
				Check:  resource.TestCheckResourceAttr("unifi_network_traffic_rule.test", "domains.#", "2"),
			},
			{ResourceName: "unifi_network_traffic_rule.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccTrafficRuleConfig(desc, `[{ domain = "ads.example.com" }, { domain = "tracker.example.net" }]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccTrafficRuleConfig(desc, domains string) string {
	return fmt.Sprintf(`
resource "unifi_network_traffic_rule" "test" {
  action          = "BLOCK"
  matching_target = "DOMAIN"
  description     = %q
  domains         = %s
  target_devices  = [{ type = "ALL_CLIENTS" }]
}
`, desc, domains)
}

func trafficRouteIDs(c unifi.Client, ctx context.Context) ([]string, error) {
	rs, err := c.Network().Traffic().Routes().List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rs))
	for i := range rs {
		ids[i] = acctest.Deref(rs[i].UnderscoreId)
	}
	return ids, nil
}

func TestAccTrafficRouteResource(t *testing.T) {
	// A route's egress network type must be compatible with the matching target
	// (a VPN/WAN egress, not a LAN), and the plain Docker controller has no such
	// network — the controller rejects every combination with "Invalid network
	// type and matching target combination". Covered at Tier-3 (HIL) instead.
	t.Skip("traffic_route needs a VPN/WAN egress network the Docker controller can't provide; HIL-covered")
	desc := acctest.NamePrefix + "troute"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_traffic_route", func(c unifi.Client, ctx context.Context, id string) error {
			return acctest.AbsentInList(func() ([]string, error) { return trafficRouteIDs(c, ctx) }, id)
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficRouteConfig(desc, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_traffic_route.test", "description", desc),
					resource.TestCheckResourceAttr("unifi_network_traffic_route.test", "matching_target", "DOMAIN"),
					resource.TestCheckResourceAttrSet("unifi_network_traffic_route.test", "id"),
				),
			},
			{
				Config: testAccTrafficRouteConfig(desc, true),
				Check:  resource.TestCheckResourceAttr("unifi_network_traffic_route.test", "kill_switch_enabled", "true"),
			},
			{ResourceName: "unifi_network_traffic_route.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccTrafficRouteConfig(desc, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccTrafficRouteConfig(desc string, killSwitch bool) string {
	// A route needs an egress network (networkId is required), so create one to
	// point it at.
	return fmt.Sprintf(`
resource "unifi_network_network" "egress" {
  name    = "%[1]s-net"
  purpose = "corporate"
  vlan_id = 3991
  subnet  = "198.51.100.1/24"
}

resource "unifi_network_traffic_route" "test" {
  matching_target     = "DOMAIN"
  description         = %[1]q
  domains             = [{ domain = "netflix.com" }]
  network_id          = unifi_network_network.egress.id
  kill_switch_enabled = %[2]t
  target_devices      = [{ type = "ALL_CLIENTS" }]
}
`, desc, killSwitch)
}
