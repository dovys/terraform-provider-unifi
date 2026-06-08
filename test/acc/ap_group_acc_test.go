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
	acctest.RegisterSweeper("unifi_network_ap_group",
		func(c unifi.Client, ctx context.Context) ([]acctest.SweepItem, error) {
			gs, err := c.Network().WiFi().APGroups().List(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]acctest.SweepItem, len(gs))
			for i := range gs {
				out[i] = acctest.SweepItem{ID: acctest.Deref(gs[i].UnderscoreId), Name: gs[i].Name}
			}
			return out, nil
		},
		func(c unifi.Client, ctx context.Context, id string) error {
			return c.Network().WiFi().APGroups().Delete(ctx, id)
		})
}

func apGroupIDs(c unifi.Client, ctx context.Context) ([]string, error) {
	gs, err := c.Network().WiFi().APGroups().List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(gs))
	for i := range gs {
		ids[i] = acctest.Deref(gs[i].UnderscoreId)
	}
	return ids, nil
}

func TestAccAPGroupResource(t *testing.T) {
	name := acctest.NamePrefix + "apg"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: acctest.CheckDestroyGone("unifi_network_ap_group", func(c unifi.Client, ctx context.Context, id string) error {
			return acctest.AbsentInList(func() ([]string, error) { return apGroupIDs(c, ctx) }, id)
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccAPGroupConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_ap_group.test", "name", name),
					resource.TestCheckResourceAttrSet("unifi_network_ap_group.test", "id"),
				),
			},
			{
				Config: testAccAPGroupConfig(name + "-renamed"),
				Check:  resource.TestCheckResourceAttr("unifi_network_ap_group.test", "name", name+"-renamed"),
			},
			{ResourceName: "unifi_network_ap_group.test", ImportState: true, ImportStateVerify: true},
			{
				Config:           testAccAPGroupConfig(name + "-renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccAPGroupConfig(name string) string {
	return fmt.Sprintf(`
resource "unifi_network_ap_group" "test" {
  name = %q
}
`, name)
}
