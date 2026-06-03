//go:build hil

// W0 — pure control-plane data. None of these touch the dataplane, RF, or any
// device: a leaked teardown leaves an inert, unreferenced object. See
// docs/testing.md §"Write verification".
package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
)

func init() {
	registerHILSweeper("unifi_network_firewall_group", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().Firewall().Groups()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep firewall_group %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_user_group", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().UserGroups()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep user_group %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_radius_profile", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().RADIUS().Profiles()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep radius_profile %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_radius_account", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().RADIUS().Accounts()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep radius_account %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_protect_liveview", func(ctx context.Context, c unifi.Client) error {
		coll := c.Protect().Liveviews()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, items[i].Id); err != nil { // Liveview.Id is a non-pointer string
				return fmt.Errorf("sweep liveview %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_dhcp_reservation", func(ctx context.Context, c unifi.Client) error {
		// A reservation is a fixed-IP marker on a client record. Destroy clears
		// use_fixedip but keeps the record; the sweep hard-deletes the test record.
		coll := c.Network().Clients().Users()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep dhcp_reservation %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
}

// requireMACNotActive refuses to write a reservation for a MAC that is already a
// known client record — the test MAC must be unused so the reservation binds to
// nothing real (rail 1: additive, never mutate an existing object).
func requireMACNotActive(t *testing.T, mac string) {
	t.Helper()
	c, err := sweepClient()
	if err != nil {
		t.Fatalf("precondition client: %v", err)
	}
	if _, err := c.Network().Clients().GetByMAC(context.Background(), mac); err == nil {
		t.Fatalf("refusing to write a reservation: MAC %s is already a known client", mac)
	} else if !isNotFoundErr(err) {
		t.Fatalf("checking MAC %s: %v", mac, err)
	}
}

func TestHILWrite_W0_FirewallGroup(t *testing.T) {
	const addr = "unifi_network_firewall_group.test"
	name := tfaccHILPrefix + "fwgroup"
	cfg := func(member string) string {
		return fmt.Sprintf(`
resource "unifi_network_firewall_group" "test" {
  name    = %q
  type    = "address-group"
  members = [%q]
}
`, name, member)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: cfg("198.51.100.1"),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "type", "address-group"),
			resource.TestCheckResourceAttr(addr, "members.#", "1"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg("198.51.100.2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckTypeSetElemAttr(addr, "members.*", "198.51.100.2"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_firewall_group", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().Firewall().Groups().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W0_UserGroup(t *testing.T) {
	const addr = "unifi_network_user_group.test"
	name := tfaccHILPrefix + "usergroup"
	cfg := func(down int) string {
		return fmt.Sprintf(`
resource "unifi_network_user_group" "test" {
  name              = %q
  qos_rate_max_down = %d
}
`, name, down)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: cfg(10000),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "qos_rate_max_down", "10000"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(20000),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "qos_rate_max_down", "20000"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_user_group", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().UserGroups().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W0_RadiusProfile(t *testing.T) {
	const addr = "unifi_network_radius_profile.test"
	name := tfaccHILPrefix + "radprofile"
	cfg := func(interim bool) string {
		return fmt.Sprintf(`
resource "unifi_network_radius_profile" "test" {
  name                   = %q
  interim_update_enabled = %t
}
`, name, interim)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: cfg(false),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "interim_update_enabled", "false"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(true),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "interim_update_enabled", "true"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_radius_profile", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().RADIUS().Profiles().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W0_RadiusAccount(t *testing.T) {
	const addr = "unifi_network_radius_account.test"
	name := tfaccHILPrefix + "raduser"
	cfg := func(vlan int) string {
		return fmt.Sprintf(`
resource "unifi_network_radius_account" "test" {
  name     = %q
  password = "tfacc-hil-secret"
  vlan     = %d
}
`, name, vlan)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: cfg(100),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "vlan", "100"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(200),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "vlan", "200"),
		},
		// password is write-only — the controller never echoes it back.
		importIgnore: []string{"password"},
		checkDestroy: checkDestroyByGet("unifi_network_radius_account", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().RADIUS().Accounts().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W0_ProtectLiveview(t *testing.T) {
	const addr = "unifi_protect_liveview.test"
	name := tfaccHILPrefix + "liveview"
	cfg := func(global bool) string {
		return fmt.Sprintf(`
resource "unifi_protect_liveview" "test" {
  name      = %q
  is_global = %t
}
`, name, global)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: cfg(false),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "is_global", "false"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(true),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "is_global", "true"),
		},
		checkDestroy: checkDestroyByGet("unifi_protect_liveview", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Protect().Liveviews().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W0_DNSRecord(t *testing.T) {
	const addr = "unifi_network_dns_record.test"
	name := tfaccHILPrefix + "host.test"
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		createConfig: testAccDNSRecordConfig(name, "A", "192.0.2.10"),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "value", "192.0.2.10"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: testAccDNSRecordConfig(name, "A", "192.0.2.11"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "value", "192.0.2.11"),
		},
		checkDestroy: testAccCheckDNSRecordDestroy, // reused from the Tier-2 test
	})
}

func TestHILWrite_W0_DHCPReservation(t *testing.T) {
	const addr = "unifi_network_dhcp_reservation.test"
	if os.Getenv("UNIFI_HIL_ALLOW_WRITES") != "1" {
		t.Skip("write-verification HIL tier is opt-in: set UNIFI_HIL_ALLOW_WRITES=1")
	}
	// The reservation IP must fall inside the operator's own default-network DHCP
	// scope, which varies per console — the operator supplies an unused one.
	ip := os.Getenv("UNIFI_HIL_RESERVATION_IP")
	if ip == "" {
		t.Skip("set UNIFI_HIL_RESERVATION_IP to an unused IPv4 inside your default network's DHCP scope (nothing else may claim it)")
	}
	mac := hilTestMAC(0x01)
	name := tfaccHILPrefix + "reservation"
	cfg := func(suffix string) string {
		return fmt.Sprintf(`
resource "unifi_network_dhcp_reservation" "test" {
  mac        = %q
  ip_address = %q
  name       = %q
}
`, mac, ip, name+suffix)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W0",
		resourceAddr: addr,
		preApply:     func(t *testing.T) { requireMACNotActive(t, mac) },
		createConfig: cfg(""),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "mac", mac),
			resource.TestCheckResourceAttr(addr, "ip_address", ip),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg("-2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name+"-2"),
		},
		// destroy reverts the fixed-IP rather than deleting the client record
		// (invariant 4: never forget) — so verify use_fixedip cleared, not absence.
		checkDestroy: func(s *terraform.State) error {
			c, err := sweepClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "unifi_network_dhcp_reservation" {
					continue
				}
				u, err := c.Network().Clients().GetByMAC(ctx, mac)
				if isNotFoundErr(err) {
					continue // record fully gone is also acceptable
				}
				if err != nil {
					return fmt.Errorf("checking destroy of reservation %s: %w", mac, err)
				}
				if u.UseFixedip != nil && *u.UseFixedip {
					return fmt.Errorf("reservation for %s still has use_fixedip=true after destroy", mac)
				}
			}
			return nil
		},
	})
}
