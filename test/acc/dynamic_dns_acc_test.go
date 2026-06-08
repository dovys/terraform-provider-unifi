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

const tfaccDDNSHost = acctest.NamePrefix + "ddns.example.com"

func init() {
	acctest.RegisterSweeper("unifi_network_dynamic_dns",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			ds, err := c.Network().DynamicDNS().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(ds))
			for i := range ds {
				out[i] = acctest.SweepItem{ID: acctest.Deref(ds[i].UnderscoreId), Name: acctest.Deref(ds[i].HostName)}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().DynamicDNS().Delete(ctx, id)
		})
}

func TestAccDynamicDNSResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_dynamic_dns", func(c unifi.Client, ctx context.Context, id string) error {
			_, err := c.Network().DynamicDNS().Get(ctx, id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccDynamicDNSConfig("ddns-user"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_dynamic_dns.test", "hostname", tfaccDDNSHost),
					resource.TestCheckResourceAttr("unifi_network_dynamic_dns.test", "service", "dyndns"),
					resource.TestCheckResourceAttrSet("unifi_network_dynamic_dns.test", "id"),
				),
			},
			{
				Config: testAccDynamicDNSConfig("ddns-user2"),
				Check:  resource.TestCheckResourceAttr("unifi_network_dynamic_dns.test", "login", "ddns-user2"),
			},
			{
				ResourceName:            "unifi_network_dynamic_dns.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"}, // write-only secret
			},
			{
				Config:           testAccDynamicDNSConfig("ddns-user2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccDynamicDNSConfig(login string) string {
	return fmt.Sprintf(`
resource "unifi_network_dynamic_dns" "test" {
  service   = "dyndns"
  hostname  = %q
  login     = %q
  password  = "tfacc-ddns-pw"
  interface = "wan"
}
`, tfaccDDNSHost, login)
}
