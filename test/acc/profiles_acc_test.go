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
	acctest.RegisterSweeper("unifi_network_port_profile",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ps, err := c.Network().PortProfiles().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(ps))
			for i := range ps {
				out[i] = acctest.SweepItem{ID: acctest.Deref(ps[i].UnderscoreId), Name: acctest.Deref(ps[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().PortProfiles().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_user_group",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			gs, err := c.Network().UserGroups().List(ctx)
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
			return c.Network().UserGroups().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_radius_profile",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ps, err := c.Network().RADIUS().Profiles().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(ps))
			for i := range ps {
				out[i] = acctest.SweepItem{ID: acctest.Deref(ps[i].UnderscoreId), Name: acctest.Deref(ps[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().RADIUS().Profiles().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_radius_account",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			as, err := c.Network().RADIUS().Accounts().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(as))
			for i := range as {
				out[i] = acctest.SweepItem{ID: acctest.Deref(as[i].UnderscoreId), Name: acctest.Deref(as[i].Name)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().RADIUS().Accounts().Delete(ctx, id)
		})
}

func TestAccPortProfileResource(t *testing.T) {
	name := acctest.NamePrefix + "pp"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_port_profile", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().PortProfiles().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccPortProfileConfig(name, "auto"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_port_profile.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_port_profile.test", "poe_mode", "auto"),
					resource.TestCheckResourceAttrSet("unifi_network_port_profile.test", "id"),
				),
			},
			{
				Config: testAccPortProfileConfig(name, "off"),
				Check:  resource.TestCheckResourceAttr("unifi_network_port_profile.test", "poe_mode", "off"),
			},
			{ResourceName: "unifi_network_port_profile.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccPortProfileConfig(name, "off"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccPortProfileConfig(name, poe string) string {
	return fmt.Sprintf(`
resource "unifi_network_port_profile" "test" {
  name             = %q
  forward          = "all"
  tagged_vlan_mgmt = "auto"
  poe_mode         = %q
}
`, name, poe)
}

func TestAccUserGroupResource(t *testing.T) {
	name := acctest.NamePrefix + "ug"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_user_group", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().UserGroups().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccUserGroupConfig(name, 20000),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_user_group.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_user_group.test", "qos_rate_max_down", "20000"),
					resource.TestCheckResourceAttrSet("unifi_network_user_group.test", "id"),
				),
			},
			{
				Config: testAccUserGroupConfig(name, 30000),
				Check:  resource.TestCheckResourceAttr("unifi_network_user_group.test", "qos_rate_max_down", "30000"),
			},
			{ResourceName: "unifi_network_user_group.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccUserGroupConfig(name, 30000),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccUserGroupConfig(name string, down int) string {
	return fmt.Sprintf(`
resource "unifi_network_user_group" "test" {
  name              = %q
  qos_rate_max_down = %d
  qos_rate_max_up   = 5000
}
`, name, down)
}

func TestAccRadiusProfileResource(t *testing.T) {
	name := acctest.NamePrefix + "rp"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_radius_profile", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().RADIUS().Profiles().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccRadiusProfileConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_radius_profile.test", "name", name),
					resource.TestCheckResourceAttrSet("unifi_network_radius_profile.test", "id"),
				),
			},
			{
				Config: testAccRadiusProfileConfig(name, true),
				Check:  resource.TestCheckResourceAttr("unifi_network_radius_profile.test", "vlan_enabled", "true"),
			},
			{ResourceName: "unifi_network_radius_profile.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccRadiusProfileConfig(name, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccRadiusProfileConfig(name string, vlanEnabled bool) string {
	return fmt.Sprintf(`
resource "unifi_network_radius_profile" "test" {
  name                = %q
  use_usg_auth_server = true
  use_usg_acct_server = true
  vlan_enabled        = %t
}
`, name, vlanEnabled)
}

func TestAccRadiusAccountResource(t *testing.T) {
	name := acctest.NamePrefix + "ra"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_radius_account", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().RADIUS().Accounts().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccRadiusAccountConfig(name, 30),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_radius_account.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_radius_account.test", "vlan", "30"),
					resource.TestCheckResourceAttrSet("unifi_network_radius_account.test", "id"),
				),
			},
			{
				Config: testAccRadiusAccountConfig(name, 40),
				Check:  resource.TestCheckResourceAttr("unifi_network_radius_account.test", "vlan", "40"),
			},
			{
				ResourceName:            "unifi_network_radius_account.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"}, // write-only secret
			},
			{
				Config:           testAccRadiusAccountConfig(name, 40),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccRadiusAccountConfig(name string, vlan int) string {
	return fmt.Sprintf(`
resource "unifi_network_radius_account" "test" {
  name               = %q
  password           = "tfacc-radius-pw-123"
  tunnel_type        = 13
  tunnel_medium_type = 6
  vlan               = %d
}
`, name, vlan)
}
