package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

func init() {
	resource.AddTestSweepers("unifi_network_dns_record", &resource.Sweeper{
		Name: "unifi_network_dns_record",
		F: func(_ string) error {
			c, err := sweepClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			recs, err := c.ListDNSRecords(ctx)
			if err != nil {
				return err
			}
			for i := range recs {
				if !strings.HasPrefix(recs[i].Key, accNamePrefix) {
					continue
				}
				if err := c.DeleteDNSRecord(ctx, recs[i].ID); err != nil {
					return fmt.Errorf("sweep dns %s (%s): %w", recs[i].Key, recs[i].ID, err)
				}
			}
			return nil
		},
	})
}

func TestAccDNSRecordResource(t *testing.T) {
	name := accNamePrefix + "host.lan"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSRecordDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccDNSRecordConfig(name, "A", "192.0.2.10"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network_dns_record.test", "name", name),
					resource.TestCheckResourceAttr("unifi_network_dns_record.test", "type", "A"),
					resource.TestCheckResourceAttr("unifi_network_dns_record.test", "value", "192.0.2.10"),
					resource.TestCheckResourceAttr("unifi_network_dns_record.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("unifi_network_dns_record.test", "id"),
				),
			},
			{ // update the target
				Config: testAccDNSRecordConfig(name, "A", "192.0.2.11"),
				Check:  resource.TestCheckResourceAttr("unifi_network_dns_record.test", "value", "192.0.2.11"),
			},
			{ // import
				ResourceName:      "unifi_network_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // second consecutive plan must be empty
				Config: testAccDNSRecordConfig(name, "A", "192.0.2.11"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccDNSRecordConfig(name, recType, value string) string {
	return fmt.Sprintf(`
resource "unifi_network_dns_record" "test" {
  name  = %q
  type  = %q
  value = %q
}
`, name, recType, value)
}

func testAccCheckDNSRecordDestroy(s *terraform.State) error {
	c, err := sweepClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "unifi_network_dns_record" {
			continue
		}
		_, err := c.GetDNSRecord(ctx, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("dns record %s still exists after destroy", rs.Primary.ID)
		}
		if !errors.Is(err, unifi.ErrNotFound) {
			return fmt.Errorf("checking destroy of %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}
