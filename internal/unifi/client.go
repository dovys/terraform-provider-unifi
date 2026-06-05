package unifi

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-version"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/console"
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

	// Console returns the UniFi OS console client (c.Console().Certificates()…).
	// It shares this client's authenticated session over the unifi-core OS API at
	// baseURL-rooted /api/... — the same one cookie session, no api_key. The
	// console OS surface is global (not site-scoped).
	Console() *console.Client

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

// Process-global cache of ready Clients keyed by configuration. The provider's
// Configure runs per acceptance test (and the sweepers build their own client),
// so without reuse one run logs in a dozen times and trips the controller's
// login rate-limit. A real `terraform apply` configures once, so the cache is a
// no-op there — but back-to-back `terraform import`s in one process benefit too.
// Reuse hands back the same authenticated session (its token auto-re-auths near
// expiry), so identical configs share one login + one capability probe. Disable
// with UNIFI_DISABLE_CLIENT_CACHE=1.
var (
	clientCacheMu sync.Mutex
	clientCache   = map[string]Client{}
)

func clientCacheDisabled() bool {
	v := strings.TrimSpace(os.Getenv("UNIFI_DISABLE_CLIENT_CACHE"))
	return v == "1" || strings.EqualFold(v, "true")
}

// clientCacheKey fingerprints the auth-relevant config (the password is hashed
// in, never held as a raw map key). Each field is length-prefixed so distinct
// field boundaries can't collide — e.g. (user "a", pass "b\x00c") must not key
// the same as (user "a\x00b", pass "c").
func clientCacheKey(cfg Config) string {
	h := sha256.New()
	for _, field := range []string{
		normalizeHost(cfg.Host), cfg.Username, cfg.Password, cfg.Site,
		strconv.FormatBool(cfg.AllowInsecure), cfg.Timeout.String(),
		strconv.Itoa(cfg.MaxRetries),
	} {
		_, _ = fmt.Fprintf(h, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// NewClient validates config, builds the cookie+CSRF (classic) session, eagerly
// authenticates it, probes capabilities, and returns a ready Client. Identical
// configs reuse a cached, already-authenticated Client (see clientCache). Errors
// are typed (ConfigError, transport.TwoFactorError, transport.AuthError, …) so
// the provider can render a helpful diagnostic.
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

	useCache := !clientCacheDisabled()
	var cacheKey string
	if useCache {
		cacheKey = clientCacheKey(cfg)
		clientCacheMu.Lock()
		cached := clientCache[cacheKey]
		clientCacheMu.Unlock()
		if cached != nil {
			return cached, nil
		}
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
	// Probe the Protect application (non-fatal): its NVR singleton is reachable
	// only when Protect is installed on this console. Any error just means "not
	// installed" — Protect resources surface a friendly diagnostic rather than a
	// raw 4xx (invariant 3). One cheap GET, shared with the Network probe.
	if raw, perr := c.Protect().NVR().Version(ctx); perr == nil {
		caps.ProtectInstalled = true
		caps.ProtectVersionRaw = raw
		if pv, verr := version.NewVersion(raw); verr == nil {
			caps.ProtectVersion = pv
		}
	}
	// Probe the unifi-core certificate surface (non-fatal): a cheap list GET. It
	// succeeds (often with an empty list) when the OS exposes userCertificates,
	// and errors on firmware that lacks it — the console_certificate resource
	// gates on this to emit a plan-time diagnostic instead of a raw 4xx
	// (invariant 3).
	if _, cerr := c.Console().Certificates().List(ctx); cerr == nil {
		caps.ConsoleCertSupported = true
	}
	c.caps = caps

	if useCache {
		clientCacheMu.Lock()
		// Another goroutine may have built the same client concurrently; prefer
		// the already-cached one so callers share a single session.
		if existing := clientCache[cacheKey]; existing != nil {
			clientCacheMu.Unlock()
			return existing, nil
		}
		clientCache[cacheKey] = c
		clientCacheMu.Unlock()
	}
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

// Console builds the UniFi OS console client over this client's authenticated
// classic session — the unifi-core OS API at baseURL-rooted /api/..., no api_key.
func (c *client) Console() *console.Client { return console.New(c.classic) }

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
