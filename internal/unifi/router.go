package unifi

// Backend identifies which API surface an operation routes to.
type Backend int

const (
	BackendLegacy Backend = iota
	BackendV2
	BackendIntegration
)

func (b Backend) String() string {
	switch b {
	case BackendLegacy:
		return "legacy"
	case BackendV2:
		return "v2"
	case BackendIntegration:
		return "integration"
	default:
		return "unknown"
	}
}

// Op is a logical, backend-agnostic operation. Resources name an Op; the router
// maps it to a Backend. This is the single place a backend migration happens:
// flip the policy here plus the capability flag and no resource code changes
// (development.md invariants 1 & 2).
type Op string

const (
	OpNetworkRead     Op = "network.read"
	OpNetworkWrite    Op = "network.write"
	OpWLANRead        Op = "wlan.read"
	OpWLANWrite       Op = "wlan.write"
	OpAPGroup         Op = "ap_group"
	OpDeviceRead      Op = "device.read"
	OpDeviceConfig    Op = "device.config"
	OpDeviceAdopt     Op = "device.adopt"
	OpPortProfile     Op = "port_profile"
	OpFirewallClassic Op = "firewall.classic"
	OpFirewallZone    Op = "firewall.zone"
	OpTraffic         Op = "traffic"
	OpRouting         Op = "routing" // static routes, port forwards, dynamic DNS
	OpLocalDNS        Op = "local_dns"
	OpRADIUS          Op = "radius"
	OpUserGroup       Op = "user_group"
	OpDHCPReservation Op = "dhcp_reservation"
	OpSetting         Op = "setting"
	OpSiteRead        Op = "site.read"
	OpSiteWrite       Op = "site.write"
	OpVoucher         Op = "voucher"
)

// Router holds the routing policy. It consults capability flags so a
// firmware-gated write lands on Integration where supported and falls back to
// legacy/v2 otherwise — invisibly to resource code.
type Router struct {
	flags FeatureFlags
}

func NewRouter(flags FeatureFlags) *Router { return &Router{flags: flags} }

// Backend returns the surface for an operation under the current capabilities.
// Today most writes are legacy (the widest, most stable surface); v2 owns the
// newer zone-firewall / traffic / static-DNS / AP-group surfaces; Integration
// owns the reads it serves well, plus adoption and local DNS once firmware
// supports the write (architecture.md, routing table).
func (r *Router) Backend(op Op) Backend {
	switch op {
	case OpAPGroup, OpFirewallZone, OpTraffic:
		return BackendV2
	case OpLocalDNS:
		if r.flags.IntegrationDNSPolicies {
			return BackendIntegration
		}
		return BackendV2
	case OpDeviceAdopt:
		if r.flags.IntegrationWrites {
			return BackendIntegration
		}
		return BackendLegacy
	case OpSiteRead, OpVoucher:
		return BackendIntegration
	default:
		// networks/WAN, WLAN, device read+config, port profiles, classic
		// firewall, routing, RADIUS, user groups, reservations, settings, site
		// write: legacy REST — the widest, most stable write surface today.
		return BackendLegacy
	}
}

// IntegrationAvailable reports whether the Integration write surface is usable
// on the connected firmware (the version floor is met).
func (r *Router) IntegrationAvailable() bool { return r.flags.IntegrationWrites }
