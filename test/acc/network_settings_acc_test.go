//go:build acc

package acc

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
)

// network_settings is a singleton whose sections are managed à la carte; destroy
// is a no-op state removal (the site settings always exist), so there is no
// CheckDestroy, and import takes a hand-written config rather than
// ImportStateVerify (testing.md). The lifecycle here is create → update → empty
// plan on one section (locale).
func TestAccNetworkSettingsResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNetworkSettingsConfig("America/New_York"),
				Check:  resource.TestCheckResourceAttr("unifi_network_settings.test", "locale.timezone", "America/New_York"),
			},
			{
				Config: testAccNetworkSettingsConfig("Europe/London"),
				Check:  resource.TestCheckResourceAttr("unifi_network_settings.test", "locale.timezone", "Europe/London"),
			},
			{
				Config:           testAccNetworkSettingsConfig("Europe/London"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func testAccNetworkSettingsConfig(tz string) string {
	return fmt.Sprintf(`
resource "unifi_network_settings" "test" {
  locale = {
    timezone = %q
  }
}
`, tz)
}
