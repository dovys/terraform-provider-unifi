package network

import (
	"context"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

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

// Features is the site's v2 feature-availability list (e.g. "IPS", "BGP_ROUTING").
// Membership means the firmware/hardware SUPPORTS the feature, not that it's active
// — an entry can be present while the feature still needs a paid subscription (e.g.
// CyberSecure) to enable. Gate the wholly-unsupported case with Has (invariant 3);
// active state shows only in the controller's response to a write.
type Features []string

// Has reports whether the named feature flag is present.
func (f Features) Has(name string) bool {
	for _, x := range f {
		if x == name {
			return true
		}
	}
	return false
}

// Features fetches the site's v2 feature list (a bare JSON string array).
// ErrNotFound on firmware without the surface.
func (c *Client) Features(ctx context.Context) (Features, error) {
	var out Features
	if err := v2Decode(ctx, c.core, http.MethodGet, "features", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DNS returns the DNS group: local static-DNS records (DNS policies later).
func (c *Client) DNS() DNSService { return DNSService{core: c.core} }
