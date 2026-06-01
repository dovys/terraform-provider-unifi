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
	Username      string
	Password      string
	Site          string
	AllowInsecure bool
	Timeout       time.Duration
	MaxRetries    int
}

// Client is the single surface resources talk to (development.md invariant 1).
// Every operation runs over one cookie+CSRF (classic) session against the
// controller's legacy `/api/s/<site>/...` and v2 `/v2/api/site/<site>/...`
// surfaces; resources never touch a transport directly. Typed methods grow per
// phase; the base exposes the probed capabilities and the configured site.
type Client interface {
	// Capabilities returns the capability set probed at Configure time.
	Capabilities() *Capabilities
	// Site returns the configured site slug (the legacy/v2 name, e.g.
	// "default").
	Site() string

	// Networks (networkconf) — backs unifi_network_network and unifi_network_wan.
	ListNetworks(ctx context.Context) ([]NetworkConf, error)
	GetNetwork(ctx context.Context, id string) (*NetworkConf, error)
	CreateNetwork(ctx context.Context, n *NetworkConf) (*NetworkConf, error)
	UpdateNetwork(ctx context.Context, n *NetworkConf) (*NetworkConf, error)
	DeleteNetwork(ctx context.Context, id string) error

	// WLANs (wlanconf) — backs unifi_network_wlan.
	ListWLANs(ctx context.Context) ([]WLANConf, error)
	GetWLAN(ctx context.Context, id string) (*WLANConf, error)
	CreateWLAN(ctx context.Context, w *WLANConf) (*WLANConf, error)
	UpdateWLAN(ctx context.Context, w *WLANConf) (*WLANConf, error)
	DeleteWLAN(ctx context.Context, id string) error

	// AP groups (v2 apgroups) — backs unifi_network_ap_group.
	ListAPGroups(ctx context.Context) ([]APGroup, error)
	GetAPGroup(ctx context.Context, id string) (*APGroup, error)
	CreateAPGroup(ctx context.Context, g *APGroup) (*APGroup, error)
	UpdateAPGroup(ctx context.Context, g *APGroup) (*APGroup, error)
	DeleteAPGroup(ctx context.Context, id string) error

	// Firewall (classic /rest/firewallgroup + /rest/firewallrule) — backs
	// unifi_network_firewall_group and unifi_network_firewall_rule.
	ListFirewallGroups(ctx context.Context) ([]FirewallGroup, error)
	GetFirewallGroup(ctx context.Context, id string) (*FirewallGroup, error)
	CreateFirewallGroup(ctx context.Context, g *FirewallGroup) (*FirewallGroup, error)
	UpdateFirewallGroup(ctx context.Context, g *FirewallGroup) (*FirewallGroup, error)
	DeleteFirewallGroup(ctx context.Context, id string) error

	ListFirewallRules(ctx context.Context) ([]FirewallRule, error)
	GetFirewallRule(ctx context.Context, id string) (*FirewallRule, error)
	CreateFirewallRule(ctx context.Context, r *FirewallRule) (*FirewallRule, error)
	UpdateFirewallRule(ctx context.Context, r *FirewallRule) (*FirewallRule, error)
	DeleteFirewallRule(ctx context.Context, id string) error

	// Clients (legacy /rest/user) — the one client touchpoint, backing
	// unifi_network_dhcp_reservation (find-or-create by MAC).
	ListUsers(ctx context.Context) ([]User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByMAC(ctx context.Context, mac string) (*User, error)
	CreateUser(ctx context.Context, u *User) (*User, error)
	UpdateUser(ctx context.Context, u *User) (*User, error)

	// Port profiles (legacy /rest/portconf) — backs unifi_network_port_profile.
	ListPortProfiles(ctx context.Context) ([]PortProfile, error)
	GetPortProfile(ctx context.Context, id string) (*PortProfile, error)
	CreatePortProfile(ctx context.Context, p *PortProfile) (*PortProfile, error)
	UpdatePortProfile(ctx context.Context, p *PortProfile) (*PortProfile, error)
	DeletePortProfile(ctx context.Context, id string) error

	// Devices (legacy /stat/device read, /rest/device config write, /cmd/devmgr
	// adopt/forget) — backs unifi_network_device and data.unifi_network_device.
	ListDevices(ctx context.Context) ([]Device, error)
	GetDevice(ctx context.Context, id string) (*Device, error)
	GetDeviceByMAC(ctx context.Context, mac string) (*Device, error)
	UpdateDevice(ctx context.Context, d *Device, ownedPortIdx []int) (*Device, error)
	AdoptDevice(ctx context.Context, mac string) error
	ForgetDevice(ctx context.Context, mac string) error

	// Local DNS (v2 static-dns) — backs unifi_network_dns_record.
	ListDNSRecords(ctx context.Context) ([]StaticDNSRecord, error)
	GetDNSRecord(ctx context.Context, id string) (*StaticDNSRecord, error)
	CreateDNSRecord(ctx context.Context, r *StaticDNSRecord) (*StaticDNSRecord, error)
	UpdateDNSRecord(ctx context.Context, r *StaticDNSRecord) (*StaticDNSRecord, error)
	DeleteDNSRecord(ctx context.Context, id string) error

	// User groups (legacy /rest/usergroup) — backs unifi_network_user_group.
	ListUserGroups(ctx context.Context) ([]UserGroup, error)
	GetUserGroup(ctx context.Context, id string) (*UserGroup, error)
	CreateUserGroup(ctx context.Context, g *UserGroup) (*UserGroup, error)
	UpdateUserGroup(ctx context.Context, g *UserGroup) (*UserGroup, error)
	DeleteUserGroup(ctx context.Context, id string) error

	// RADIUS profiles (legacy /rest/radiusprofile) — backs unifi_network_radius_profile.
	ListRadiusProfiles(ctx context.Context) ([]RadiusProfile, error)
	GetRadiusProfile(ctx context.Context, id string) (*RadiusProfile, error)
	CreateRadiusProfile(ctx context.Context, p *RadiusProfile) (*RadiusProfile, error)
	UpdateRadiusProfile(ctx context.Context, p *RadiusProfile) (*RadiusProfile, error)
	DeleteRadiusProfile(ctx context.Context, id string) error

	// RADIUS accounts (legacy /rest/account) — backs unifi_network_radius_account.
	ListRadiusAccounts(ctx context.Context) ([]RadiusAccount, error)
	GetRadiusAccount(ctx context.Context, id string) (*RadiusAccount, error)
	CreateRadiusAccount(ctx context.Context, a *RadiusAccount) (*RadiusAccount, error)
	UpdateRadiusAccount(ctx context.Context, a *RadiusAccount) (*RadiusAccount, error)
	DeleteRadiusAccount(ctx context.Context, id string) error

	// Static routes (legacy /rest/routing) — backs unifi_network_static_route.
	ListStaticRoutes(ctx context.Context) ([]StaticRoute, error)
	GetStaticRoute(ctx context.Context, id string) (*StaticRoute, error)
	CreateStaticRoute(ctx context.Context, r *StaticRoute) (*StaticRoute, error)
	UpdateStaticRoute(ctx context.Context, r *StaticRoute) (*StaticRoute, error)
	DeleteStaticRoute(ctx context.Context, id string) error

	// Port forwards (legacy /rest/portforward) — backs unifi_network_port_forward.
	ListPortForwards(ctx context.Context) ([]PortForward, error)
	GetPortForward(ctx context.Context, id string) (*PortForward, error)
	CreatePortForward(ctx context.Context, p *PortForward) (*PortForward, error)
	UpdatePortForward(ctx context.Context, p *PortForward) (*PortForward, error)
	DeletePortForward(ctx context.Context, id string) error

	// Dynamic DNS (legacy /rest/dynamicdns) — backs unifi_network_dynamic_dns.
	ListDynamicDNS(ctx context.Context) ([]DynamicDNS, error)
	GetDynamicDNS(ctx context.Context, id string) (*DynamicDNS, error)
	CreateDynamicDNS(ctx context.Context, d *DynamicDNS) (*DynamicDNS, error)
	UpdateDynamicDNS(ctx context.Context, d *DynamicDNS) (*DynamicDNS, error)
	DeleteDynamicDNS(ctx context.Context, id string) error

	// WLAN groups (legacy /rest/wlangroup) — backs unifi_network_wlan_group.
	ListWLANGroups(ctx context.Context) ([]WLANGroup, error)
	GetWLANGroup(ctx context.Context, id string) (*WLANGroup, error)
	CreateWLANGroup(ctx context.Context, g *WLANGroup) (*WLANGroup, error)
	UpdateWLANGroup(ctx context.Context, g *WLANGroup) (*WLANGroup, error)
	DeleteWLANGroup(ctx context.Context, id string) error

	// Traffic rules (v2 trafficrules) — backs unifi_network_traffic_rule.
	ListTrafficRules(ctx context.Context) ([]TrafficRule, error)
	GetTrafficRule(ctx context.Context, id string) (*TrafficRule, error)
	CreateTrafficRule(ctx context.Context, r *TrafficRule) (*TrafficRule, error)
	UpdateTrafficRule(ctx context.Context, r *TrafficRule) (*TrafficRule, error)
	DeleteTrafficRule(ctx context.Context, id string) error

	// Traffic routes (v2 trafficroutes) — backs unifi_network_traffic_route.
	ListTrafficRoutes(ctx context.Context) ([]TrafficRoute, error)
	GetTrafficRoute(ctx context.Context, id string) (*TrafficRoute, error)
	CreateTrafficRoute(ctx context.Context, r *TrafficRoute) (*TrafficRoute, error)
	UpdateTrafficRoute(ctx context.Context, r *TrafficRoute) (*TrafficRoute, error)
	DeleteTrafficRoute(ctx context.Context, id string) error

	// Sites.
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
