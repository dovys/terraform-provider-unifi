package network

import (
	"context"
	"testing"
)

// TestClientGroupPaths spot-checks the surface + path wiring for the CRUD groups
// across both surfaces: legacy /rest/<type> (with the {meta,data} envelope) and
// v2 /<resource> (bare JSON). One List per group is enough to prove the route.
func TestClientGroupPaths(t *testing.T) {
	ctx := context.Background()
	const base = "/proxy/network/api/s/default/rest/"
	const v2 = "/proxy/network/v2/api/site/default/"
	cases := []struct {
		name, want string
		legacy     bool
		call       func(*Client) error
	}{
		{"wifi.wlans", base + "wlanconf", true, func(c *Client) error { _, e := c.WiFi().WLANs().List(ctx); return e }},
		{"wifi.wlangroups", base + "wlangroup", true, func(c *Client) error { _, e := c.WiFi().WLANGroups().List(ctx); return e }},
		{"wifi.apgroups", v2 + "apgroups", false, func(c *Client) error { _, e := c.WiFi().APGroups().List(ctx); return e }},
		{"firewall.groups", base + "firewallgroup", true, func(c *Client) error { _, e := c.Firewall().Groups().List(ctx); return e }},
		{"firewall.rules", base + "firewallrule", true, func(c *Client) error { _, e := c.Firewall().Rules().List(ctx); return e }},
		{"portforwards", base + "portforward", true, func(c *Client) error { _, e := c.PortForwards().List(ctx); return e }},
		{"traffic.rules", v2 + "trafficrules", false, func(c *Client) error { _, e := c.Traffic().Rules().List(ctx); return e }},
		{"traffic.routes", v2 + "trafficroutes", false, func(c *Client) error { _, e := c.Traffic().Routes().List(ctx); return e }},
		{"traffic.qos", v2 + "qos-rules", false, func(c *Client) error { _, e := c.Traffic().QoS().List(ctx); return e }},
		{"radius.profiles", base + "radiusprofile", true, func(c *Client) error { _, e := c.RADIUS().Profiles().List(ctx); return e }},
		{"radius.accounts", base + "account", true, func(c *Client) error { _, e := c.RADIUS().Accounts().List(ctx); return e }},
		{"portprofiles", base + "portconf", true, func(c *Client) error { _, e := c.PortProfiles().List(ctx); return e }},
		{"usergroups", base + "usergroup", true, func(c *Client) error { _, e := c.UserGroups().List(ctx); return e }},
		{"dynamicdns", base + "dynamicdns", true, func(c *Client) error { _, e := c.DynamicDNS().List(ctx); return e }},
		{"dpiapps", base + "dpiapp", true, func(c *Client) error { _, e := c.DPIApps().List(ctx); return e }},
		{"dpigroups", base + "dpigroup", true, func(c *Client) error { _, e := c.DPIGroups().List(ctx); return e }},
	}
	for _, tc := range cases {
		body := "[]"
		if tc.legacy {
			body = `{"meta":{"rc":"ok"},"data":[]}`
		}
		f := &fakeDoer{resp: okJSON(body)}
		if err := tc.call(newTestClient(f)); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if f.method != "GET" || f.path != tc.want {
			t.Errorf("%s: got %s %s, want GET %s", tc.name, f.method, f.path, tc.want)
		}
	}
}
