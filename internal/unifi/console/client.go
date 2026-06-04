package console

import "github.com/dovys/terraform-provider-unifi/internal/unifi/transport"

// Client is the UniFi OS console client — the c.Console() arm of the substruct
// API. Groups hang off it; each exposes its operations:
//
//	c.Console().Certificates().List(ctx)
//
// The client holds no state beyond the shared session (in core). The console OS
// surface is global, so there is no site.
type Client struct {
	core *core
}

// New builds a console client over an authenticated classic session — the same
// one cookie+CSRF session the Network/Protect/Access apps use, no api_key.
func New(session *transport.Classic) *Client {
	return &Client{core: &core{session: session}}
}

// Certificates returns the userCertificates group (BYO upload + the ACME
// challenge chain) on the unifi-core OS surface.
func (c *Client) Certificates() CertificateService {
	return CertificateService{core: c.core}
}
