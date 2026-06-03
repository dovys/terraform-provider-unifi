//go:build hil

// W1 — installed into the gateway config (a provision/reload) but inert by
// construction: disabled rules, blackhole routes, and a port profile created
// unassigned. Each create/destroy reloads the gateway ruleset (no WAN/LAN drop
// expected). See docs/testing.md §"Write verification".
package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
)

func init() {
	registerHILSweeper("unifi_network_static_route", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().StaticRoutes()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep static_route %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_port_forward", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().PortForwards()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep port_forward %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_traffic_rule", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().Traffic().Rules()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Description), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep traffic_rule %s: %w", deref(items[i].Description), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_traffic_route", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().Traffic().Routes()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Description), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep traffic_route %s: %w", deref(items[i].Description), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_firewall_rule", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().Firewall().Rules()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep firewall_rule %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
	registerHILSweeper("unifi_network_port_profile", func(ctx context.Context, c unifi.Client) error {
		coll := c.Network().PortProfiles()
		items, err := coll.List(ctx)
		if err != nil {
			return err
		}
		for i := range items {
			if !strings.HasPrefix(deref(items[i].Name), tfaccHILPrefix) {
				continue
			}
			if err := coll.Delete(ctx, deref(items[i].UnderscoreId)); err != nil {
				return fmt.Errorf("sweep port_profile %s: %w", deref(items[i].Name), err)
			}
		}
		return nil
	})
}

// checkPortProfileUnassigned proves the created profile is inert: no device port
// references it. (Binding a profile to a live port would reconfigure that port —
// out of scope; this asserts the test never did.)
func checkPortProfileUnassigned(addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not found in state", addr)
		}
		id := rs.Primary.ID
		c, err := sweepClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		devices, err := c.Network().Devices().List(ctx)
		if err != nil {
			return err
		}
		for i := range devices {
			overrides, err := network.PortOverridesOf(&devices[i])
			if err != nil {
				continue
			}
			for _, ov := range overrides {
				if ov.PortconfID == id {
					return fmt.Errorf("port_profile %s is assigned to a device port — not inert", id)
				}
			}
		}
		return nil
	}
}

func TestHILWrite_W1_StaticRoute(t *testing.T) {
	const addr = "unifi_network_static_route.test"
	name := tfaccHILPrefix + "route"
	cfg := func(distance int) string {
		return fmt.Sprintf(`
resource "unifi_network_static_route" "test" {
  name = %q
  network = "192.0.2.0/24"
  route_type = "blackhole"
  enabled = false
  distance = %d
}
`, name, distance)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		inertTokens:  []string{"192.0.2.0/24", `route_type = "blackhole"`, "enabled = false"},
		createConfig: cfg(100),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "network", "192.0.2.0/24"),
			resource.TestCheckResourceAttr(addr, "route_type", "blackhole"),
			resource.TestCheckResourceAttr(addr, "enabled", "false"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(150),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "distance", "150"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_static_route", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().StaticRoutes().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W1_PortForward(t *testing.T) {
	const addr = "unifi_network_port_forward.test"
	name := tfaccHILPrefix + "fwd"
	cfg := func(fwdPort string) string {
		return fmt.Sprintf(`
resource "unifi_network_port_forward" "test" {
  name = %q
  enabled = false
  destination_port = "65000"
  forward_ip = "192.0.2.10"
  forward_port = %q
}
`, name, fwdPort)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		inertTokens:  []string{"enabled = false", "192.0.2.10"},
		createConfig: cfg("8081"),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "enabled", "false"),
			resource.TestCheckResourceAttr(addr, "forward_ip", "192.0.2.10"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg("8082"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "forward_port", "8082"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_port_forward", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().PortForwards().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W1_TrafficRule(t *testing.T) {
	const addr = "unifi_network_traffic_rule.test"
	desc := tfaccHILPrefix + "trule"
	cfg := func(suffix string) string {
		return fmt.Sprintf(`
resource "unifi_network_traffic_rule" "test" {
  description = %q
  enabled = false
  action = "BLOCK"
  matching_target = "INTERNET"
  target_devices = [
    { type = "ALL_CLIENTS" }
  ]
}
`, desc+suffix)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		inertTokens:  []string{"enabled = false"},
		createConfig: cfg(""),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "description", desc),
			resource.TestCheckResourceAttr(addr, "enabled", "false"),
			resource.TestCheckResourceAttr(addr, "action", "BLOCK"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg("-2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "description", desc+"-2"),
		},
		checkDestroy: checkDestroyByList("unifi_network_traffic_rule", func(ctx context.Context, c unifi.Client) ([]string, error) {
			items, err := c.Network().Traffic().Rules().List(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(items))
			for i := range items {
				ids = append(ids, deref(items[i].UnderscoreId))
			}
			return ids, nil
		}),
	})
}

func TestHILWrite_W1_TrafficRoute(t *testing.T) {
	const addr = "unifi_network_traffic_route.test"
	desc := tfaccHILPrefix + "troute"
	cfg := func(suffix string) string {
		return fmt.Sprintf(`
resource "unifi_network_traffic_route" "test" {
  description = %q
  enabled = false
  matching_target = "INTERNET"
  target_devices = [
    { type = "ALL_CLIENTS" }
  ]
}
`, desc+suffix)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		inertTokens:  []string{"enabled = false"},
		createConfig: cfg(""),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "description", desc),
			resource.TestCheckResourceAttr(addr, "enabled", "false"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg("-2"),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "description", desc+"-2"),
		},
		checkDestroy: checkDestroyByList("unifi_network_traffic_route", func(ctx context.Context, c unifi.Client) ([]string, error) {
			items, err := c.Network().Traffic().Routes().List(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(items))
			for i := range items {
				ids = append(ids, deref(items[i].UnderscoreId))
			}
			return ids, nil
		}),
	})
}

func TestHILWrite_W1_FirewallRule(t *testing.T) {
	const addr = "unifi_network_firewall_rule.test"
	name := tfaccHILPrefix + "rule"
	idx := 4000 // a high, normally-unused index; override if it collides on your console
	if v := os.Getenv("UNIFI_HIL_FW_RULE_INDEX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			idx = n
		}
	}
	cfg := func(logging bool) string {
		return fmt.Sprintf(`
resource "unifi_network_firewall_rule" "test" {
  name = %q
  ruleset = "LAN_LOCAL"
  rule_index = %d
  action = "drop"
  enabled = false
  logging = %t
}
`, name, idx, logging)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		inertTokens:  []string{"enabled = false"},
		createConfig: cfg(false),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttr(addr, "ruleset", "LAN_LOCAL"),
			resource.TestCheckResourceAttr(addr, "rule_index", strconv.Itoa(idx)),
			resource.TestCheckResourceAttr(addr, "enabled", "false"),
			resource.TestCheckResourceAttrSet(addr, "id"),
		},
		updateConfig: cfg(true),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "logging", "true"),
		},
		checkDestroy: checkDestroyByGet("unifi_network_firewall_rule", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().Firewall().Rules().Get(ctx, id)
			return err
		}),
	})
}

func TestHILWrite_W1_PortProfile(t *testing.T) {
	const addr = "unifi_network_port_profile.test"
	name := tfaccHILPrefix + "portprofile"
	cfg := func(isolation bool) string {
		return fmt.Sprintf(`
resource "unifi_network_port_profile" "test" {
  name = %q
  isolation = %t
}
`, name, isolation)
	}
	runHILWrite(t, hilWriteTest{
		tier:         "W1",
		resourceAddr: addr,
		createConfig: cfg(false),
		createChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "name", name),
			resource.TestCheckResourceAttrSet(addr, "id"),
			checkPortProfileUnassigned(addr), // inertness: bound to no device port
		},
		updateConfig: cfg(true),
		updateChecks: []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(addr, "isolation", "true"),
			checkPortProfileUnassigned(addr),
		},
		checkDestroy: checkDestroyByGet("unifi_network_port_profile", func(ctx context.Context, c unifi.Client, id string) error {
			_, err := c.Network().PortProfiles().Get(ctx, id)
			return err
		}),
	})
}
