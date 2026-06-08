//go:build hil

package hil

// Local copies of the DNS-record config + CheckDestroy helpers. The Tier-2
// versions live in package test/acc (a different black-box package, not
// importable), so the W0 DNS write test carries its own.

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
)

func dnsRecordConfig(name, recType, value string) string {
	return fmt.Sprintf(`
resource "unifi_network_dns_record" "test" {
  name  = %q
  type  = %q
  value = %q
}
`, name, recType, value)
}

func checkDNSRecordDestroy(s *terraform.State) error {
	c, err := acctest.Client()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "unifi_network_dns_record" {
			continue
		}
		_, err := c.Network().DNS().Records().Get(ctx, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("dns record %s still exists after destroy", rs.Primary.ID)
		}
		if !acctest.IsNotFound(err) {
			return fmt.Errorf("checking destroy of %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}
