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
	acctest.RegisterSweeper("unifi_network_firewall_group",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			gs, err := c.Network().Firewall().Groups().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(gs))
			for i := range gs {
				out[i] = acctest.SweepItem{ID: acctest.Deref(gs[i].UnderscoreId), Name: acctest.Deref(gs[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().Firewall().Groups().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_firewall_rule",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			rs, err := c.Network().Firewall().Rules().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(rs))
			for i := range rs {
				out[i] = acctest.SweepItem{ID: acctest.Deref(rs[i].UnderscoreId), Name: acctest.Deref(rs[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().Firewall().Rules().Delete(ctx, id)
		})
}

func TestAccFirewallGroupResource(t *testing.T) {
	name := acctest.NamePrefix + "fwg"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_firewall_group", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().Firewall().Groups().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccFirewallGroupConfig(name, `["192.0.2.0/24"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_firewall_group.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_firewall_group.test", "type", "address-group"),
					resource.TestCheckResourceAttr("unifi_network_firewall_group.test", "members.#", "1"),
					resource.TestCheckResourceAttrSet("unifi_network_firewall_group.test", "id"),
				),
			},
			{
				Config: testAccFirewallGroupConfig(name, `["192.0.2.0/24", "198.51.100.0/24"]`),
				Check:  resource.TestCheckResourceAttr("unifi_network_firewall_group.test", "members.#", "2"),
			},
			{ResourceName: "unifi_network_firewall_group.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccFirewallGroupConfig(name, `["192.0.2.0/24", "198.51.100.0/24"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccFirewallGroupConfig(name, members string) string {
	return fmt.Sprintf(`
resource "unifi_network_firewall_group" "test" {
  name    = %q
  type    = "address-group"
  members = %s
}
`, name, members)
}

func TestAccFirewallRuleResource(t *testing.T) {
	name := acctest.NamePrefix + "fwr"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_firewall_rule", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().Firewall().Rules().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccFirewallRuleConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_firewall_rule.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_firewall_rule.test", "ruleset", "LAN_IN"),
					resource.TestCheckResourceAttr("unifi_network_firewall_rule.test", "action", "drop"),
					resource.TestCheckResourceAttrSet("unifi_network_firewall_rule.test", "id"),
				),
			},
			{
				Config: testAccFirewallRuleConfig(name, true),
				Check:  resource.TestCheckResourceAttr("unifi_network_firewall_rule.test", "logging", "true"),
			},
			{ResourceName: "unifi_network_firewall_rule.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccFirewallRuleConfig(name, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccFirewallRuleConfig(name string, logging bool) string {
	return fmt.Sprintf(`
resource "unifi_network_firewall_group" "dst" {
  name    = "%[1]s-dst"
  type    = "address-group"
  members = ["198.51.100.0/24"]
}

resource "unifi_network_firewall_rule" "test" {
  name                   = %[1]q
  ruleset                = "LAN_IN"
  rule_index             = 20011
  action                 = "drop"
  protocol               = "all"
  state_new              = true
  dst_firewall_group_ids = [unifi_network_firewall_group.dst.id]
  logging                = %[2]t
}
`, name, logging)
}
