package unifi

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Config is the resolved provider configuration used to build a Client.
type Config struct {
	Host          string
	Username      string
	Password      string
	Site          string
	AllowInsecure bool
	Timeout       time.Duration
	MaxRetries    int
}

// Client is the authenticated console session: it owns the one cookie+CSRF
// (classic) transport and exposes the console-level operations (probed
// capabilities, the configured site, site management) plus the Network()
// accessor for the substruct Network application client. Network resource CRUD
// runs through Network(); resources never touch a transport directly
// (development.md invariant 1). Every call shares this session against the
// controller's legacy `/api/s/<site>/...` and v2 `/v2/api/site/<site>/...`
// surfaces.
type Client interface {
	// Capabilities returns the capability set probed at Configure time.
	Capabilities() *Capabilities
	// Site returns the configured site slug (the legacy/v2 name, e.g.
	// "default").
	Site() string

	// Network returns the substruct Network application client
	// (c.Network().WAN().SLAs()…) — the surface every resource is built on.
	// It shares this client's authenticated session. Network CRUD lives entirely
	// on this substruct client; the top-level client carries only the
	// console-level operations below.
	Network() *network.Client

	// Protect returns the substruct Protect application client
	// (c.Protect().Cameras()…). It shares this client's authenticated session
	// over the private REST API at /proxy/protect/api — the same one cookie
	// session, no api_key.
	Protect() *protect.Client

	// Access returns the substruct Access application client
	// (c.Access().DoorGroups()…). It shares this client's authenticated session
	// over the private web-UI API at /proxy/access/api — the same one cookie
	// session, no api_key. Access is console-global (not site-scoped).
	Access() *access.Client

	// Sites (console-level: /api/self/sites read, /cmd/sitemgr writes).
	ListSites(ctx context.Context) ([]Site, error)
	CreateSite(ctx context.Context, description string) (*LegacySite, error)
	UpdateSite(ctx context.Context, legacyID, description string) error
	DeleteSite(ctx context.Context, legacyID string) error
}

type client struct {
	cfg     Config
	classic *transport.Classic
	caps    *Capabilities
}

// NewClient validates config, builds the cookie+CSRF (classic) session, eagerly
// authenticates it, probes capabilities, and returns a ready Client. Errors are
// typed (ConfigError, transport.TwoFactorError, transport.AuthError, …) so the
// provider can render a helpful diagnostic.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, &ConfigError{Msg: "host is required (set `host` or UNIFI_HOST)"}
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, &ConfigError{Msg: "username and password are required (a local UniFi OS admin account)"}
	}
	if cfg.Site == "" {
		cfg.Site = "default"
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	host := normalizeHost(cfg.Host)

	c := &client{cfg: cfg}
	// The classic session carries the TOKEN cookie in its own jar.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	classicHTTP := newHTTPClient(cfg)
	classicHTTP.Jar = jar
	c.classic = transport.NewClassic(host, cfg.Username, cfg.Password, classicHTTP, cfg.MaxRetries)
	if err := c.classic.Login(ctx); err != nil {
		return nil, err
	}

	caps, err := probe(ctx, c.classic, cfg.Site)
	if err != nil {
		return nil, err
	}
	c.caps = caps
	return c, nil
}

func (c *client) Capabilities() *Capabilities { return c.caps }
func (c *client) Site() string                { return c.cfg.Site }

// Network builds the substruct Network client over this client's authenticated
// classic session — the single surface every network resource is built on.
func (c *client) Network() *network.Client { return network.New(c.classic, c.cfg.Site) }

// Protect builds the substruct Protect client over this client's authenticated
// classic session — the private REST API at /proxy/protect/api, no api_key.
func (c *client) Protect() *protect.Client { return protect.New(c.classic) }

// Access builds the substruct Access client over this client's authenticated
// classic session — the private web-UI API at /proxy/access/api, no api_key.
func (c *client) Access() *access.Client { return access.New(c.classic) }

func newHTTPClient(cfg Config) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			// Opt-in only: UniFi consoles ship self-signed certs on local IPs,
			// so allow_insecure is commonly needed against a LAN console.
			InsecureSkipVerify: cfg.AllowInsecure, //nolint:gosec // user-controlled, documented
		},
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func normalizeHost(host string) string {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	return host
}
