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
	acctest.RegisterSweeper("unifi_network_static_route",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			rs, err := c.Network().StaticRoutes().List(ctx)
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
			return c.Network().StaticRoutes().Delete(ctx, id)
		})

	acctest.RegisterSweeper("unifi_network_port_forward",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ps, err := c.Network().PortForwards().List(ctx)
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
			return c.Network().PortForwards().Delete(ctx, id)
		})
}

func TestAccStaticRouteResource(t *testing.T) {
	name := acctest.NamePrefix + "route"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_static_route", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().StaticRoutes().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccStaticRouteConfig(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_static_route.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_static_route.test", "network", "203.0.113.0/24"),
					resource.TestCheckResourceAttr("unifi_network_static_route.test", "distance", "1"),
					resource.TestCheckResourceAttrSet("unifi_network_static_route.test", "id"),
				),
			},
			{
				Config: testAccStaticRouteConfig(name, 5),
				Check:  resource.TestCheckResourceAttr("unifi_network_static_route.test", "distance", "5"),
			},
			{ResourceName: "unifi_network_static_route.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccStaticRouteConfig(name, 5),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccStaticRouteConfig(name string, distance int) string {
	return fmt.Sprintf(`
resource "unifi_network_static_route" "test" {
  name       = %q
  network    = "203.0.113.0/24"
  route_type = "nexthop-route"
  next_hop   = "192.0.2.2"
  distance   = %d
}
`, name, distance)
}

func TestAccPortForwardResource(t *testing.T) {
	name := acctest.NamePrefix + "pf"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_port_forward", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().PortForwards().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccPortForwardConfig(name, "443"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_port_forward.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_port_forward.test", "forward_ip", "192.0.2.10"),
					resource.TestCheckResourceAttrSet("unifi_network_port_forward.test", "id"),
				),
			},
			{
				Config: testAccPortForwardConfig(name, "8443"),
				Check:  resource.TestCheckResourceAttr("unifi_network_port_forward.test", "forward_port", "8443"),
			},
			{ResourceName: "unifi_network_port_forward.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccPortForwardConfig(name, "8443"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccPortForwardConfig(name, fwdPort string) string {
	return fmt.Sprintf(`
resource "unifi_network_port_forward" "test" {
  name             = %q
  interface        = "wan"
  protocol         = "tcp"
  destination_port = "8080"
  forward_ip       = "192.0.2.10"
  forward_port     = %q
}
`, name, fwdPort)
}
