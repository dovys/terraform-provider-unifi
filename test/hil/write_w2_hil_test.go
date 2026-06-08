//go:build hil

// W2 — AP/switch provisioning. Creating/destroying these (empty, unreferenced)
// reprovisions the APs; a brief management-plane blip is possible (no client
// drop expected). Run during low usage. See docs/testing.md §"Write verification".
package hil

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/dovys/terraform-provider-unifi/internal/acctest"
	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

func init() {
	registerHILSweeper("unifi_network_wlan_group", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().WiFi().WLANGroups()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(acctest.Deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, acctest.Deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep wlan_group %s: %w", acctest.Deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_ap_group", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().WiFi().APGroups()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(items[i].Name, tfaccHILPrefix) { // Apgroups.Name is a non-pointer string
				continue
			}
			if err := coll.Delete(ctx, acctest.Deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep ap_group %s: %w", items[i].Name, err)
			}
		}
		return nil
	})
}

func TestHILWrite_W2_WLANGroup(t *testing.T) {
	const addr = "unifi_network_wlan_group.test"
	base := tfaccHILPrefix + "wlangroup"
	cfg := func(name string) string {
		return fmt.Sprintf(`
resource "unifi_network_wlan_group" "test" {
  name = %q
}
`, name)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W2",
		resourceAddr: addr,
		createConfig: cfg(base),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", base),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(base + "-2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", base+"-2"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_wlan_group", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().WiFi().WLANGroups().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W2_APGroup(t *testing.T) {
	const addr = "unifi_network_ap_group.test"
	base := tfaccHILPrefix + "apgroup"
	cfg := func(name string) string {
		return fmt.Sprintf(`
resource "unifi_network_ap_group" "test" {
  name = %q
  device_macs = []
}
`, name)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W2",
		resourceAddr: addr,
		inertTokens:  []string{"device_macs = []"}, // no APs assigned
		createConfig: cfg(base),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", base),
			resource.TestCheckResourceAttr(addr, "device_macs.#", "0"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(base + "-2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", base+"-2"),
		},
		checkDestroy: checkDestroyByList("unifi_network_ap_group", func(ctx context.Context, c unifi.Client) ([]string, error) {
			items, err := c.Network().WiFi().APGroups().List(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(items))
			for i := range items {
				ids = append(ids, acctest.Deref(items[i].UnderscoreId))
			}
			return ids, nil
		}),
	})
}
