package unifi

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Config is the resolved provider configuration used to build a Client.
type Config struct {
	Host          string
	APIKey        string
	Username      string
	Password      string
	Site          string
	AllowInsecure bool
	Timeout       time.Duration
	MaxRetries    int
}

// Client is the single surface resources talk to. It hides which API backend
// serves each operation (development.md invariant 1). Typed methods grow per
// phase; the base exposes the probed capabilities and the site identifiers.
type Client interface {
	// Capabilities returns the capability set probed at Configure time.
	Capabilities() *Capabilities
	// Router returns the backend routing policy for the connected firmware.
	Router() *Router
	// Site returns the configured site slug (the legacy/v2 name, e.g.
	// "default").
	Site() string
	// SiteID returns the Integration UUID for the configured site, or "" if the
	// Integration API was unavailable or the slug was not found.
	SiteID() string
}

type client struct {
	cfg         Config
	classic     *transport.Classic
	integration *transport.Integration
	router      *Router
	caps        *Capabilities
}

// NewClient validates config, builds the available transports, eagerly
// authenticates the classic session, probes capabilities, and returns a ready
// Client. Errors are typed (ConfigError, transport.TwoFactorError,
// transport.AuthError, …) so the provider can render a helpful diagnostic.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, &ConfigError{Msg: "host is required (set `host` or UNIFI_HOST)"}
	}
	hasClassic := cfg.Username != "" && cfg.Password != ""
	hasIntegration := cfg.APIKey != ""
	if !hasClassic && !hasIntegration {
		return nil, &ConfigError{Msg: "no credentials: set username+password (a local UniFi OS admin) and/or api_key"}
	}
	if cfg.Site == "" {
		cfg.Site = "default"
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	host := normalizeHost(cfg.Host)

	c := &client{cfg: cfg}
	if hasClassic {
		// The classic session needs its own *http.Client carrying a cookie jar
		// for the TOKEN cookie; the Integration transport must not share it.
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
	}
	if hasIntegration {
		c.integration = transport.NewIntegration(host, cfg.APIKey, newHTTPClient(cfg), cfg.MaxRetries)
	}

	caps, err := probe(ctx, c.classic, c.integration, cfg.Site)
	if err != nil {
		return nil, err
	}
	c.caps = caps
	c.router = NewRouter(caps.Flags)
	return c, nil
}

func (c *client) Capabilities() *Capabilities { return c.caps }
func (c *client) Router() *Router             { return c.router }
func (c *client) Site() string                { return c.cfg.Site }

func (c *client) SiteID() string {
	if s, ok := c.caps.Sites[c.cfg.Site]; ok {
		return s.ID
	}
	return ""
}

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
