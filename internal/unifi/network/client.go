package network

import "github.com/dovys/terraform-provider-unifi/internal/unifi/transport"

// Client is the UniFi Network application client — the c.Network() arm of the
// substruct API. Functional groups hang off it; each group exposes its CRUD
// collections and actions:
//
//	n := c.Network()
//	n.DNS().Records().List(ctx)
//
// Groups are added per rebuild phase. The client holds no state beyond the
// shared session + site (in core).
type Client struct {
	core *core
}

// New builds a Network client over an authenticated classic session for a site.
// The session carries both the legacy /rest and v2 surfaces.
func New(session *transport.Classic, site string) *Client {
	return &Client{core: &core{session: session, site: site}}
}

// Site returns the site slug this client targets.
func (c *Client) Site() string { return c.core.site }

// DNS returns the DNS group: local static-DNS records (DNS policies later).
func (c *Client) DNS() DNSService { return DNSService{core: c.core} }
