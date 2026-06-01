package unifi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// integrationWriteFloor is the conservative Network version at or above which
// the Integration v1 API accepts writes (networks/WLANs/zone-firewall/ACL/DNS/
// vouchers/adoption). Below it Integration is read-mostly and the router falls
// back to legacy/v2. Refined per-write as firmware behavior is confirmed
// (roadmap open question #3).
var integrationWriteFloor = version.Must(version.NewVersion("10.1.84"))

// FeatureFlags are the capability bits the router consults. They are derived
// from the detected version here; the classic-vs-zone firewall mode is detected
// lazily by firewall resources from object counts (architecture.md §5), not in
// this struct.
type FeatureFlags struct {
	// IntegrationWrites is true when firmware accepts Integration v1 writes
	// (the firmware floor is met).
	IntegrationWrites bool
	// IntegrationDNSPolicies routes local DNS to Integration dns/policies
	// instead of v2 /static-dns.
	IntegrationDNSPolicies bool
}

// StaticDNSOnV2 reports whether local DNS still uses the v2 /static-dns surface
// (the inverse of IntegrationDNSPolicies).
func (f FeatureFlags) StaticDNSOnV2() bool { return !f.IntegrationDNSPolicies }

// Capabilities is the cached result of the Configure-time probe.
type Capabilities struct {
	// Version is the parsed Network application version (nil if undetected or
	// unparseable; VersionRaw still carries the reported string).
	Version *version.Version
	// VersionRaw is the version string as the controller reported it.
	VersionRaw string
	// Sites maps a site slug (internalReference, e.g. "default") to its
	// Integration UUID and display name. Empty if the Integration API was
	// unavailable at Configure time.
	Sites map[string]models.Site
	// Flags are the derived routing capability bits.
	Flags FeatureFlags
}

// deriveFlags computes routing flags from a detected version. An undetected
// version (nil) yields the conservative all-false set: legacy/v2 everywhere.
func deriveFlags(v *version.Version) FeatureFlags {
	if v == nil {
		return FeatureFlags{}
	}
	writes := v.GreaterThanOrEqual(integrationWriteFloor)
	return FeatureFlags{
		IntegrationWrites:      writes,
		IntegrationDNSPolicies: writes,
	}
}

// probe runs the Configure-time capability detection. classic and integration
// may be nil if the matching credentials were not supplied; probe uses whatever
// is available and leaves undetermined fields zero.
func probe(ctx context.Context, classic *transport.Classic, integration *transport.Integration, site string) (*Capabilities, error) {
	caps := &Capabilities{Sites: map[string]models.Site{}}

	if classic != nil {
		v, raw, err := fetchVersionLegacy(ctx, classic, site)
		if err != nil {
			return nil, fmt.Errorf("detect controller version: %w", err)
		}
		caps.Version, caps.VersionRaw = v, raw
	}

	if integration != nil {
		sites, err := fetchSites(ctx, integration)
		if err != nil {
			return nil, fmt.Errorf("list sites (integration): %w", err)
		}
		for _, s := range sites {
			caps.Sites[s.InternalReference] = s
		}
		// If classic was absent, fall back to integration application-info for
		// the version (404s pre-9.3 — tolerated, leaving Version nil).
		if caps.Version == nil {
			if v, raw, ok := fetchVersionIntegration(ctx, integration); ok {
				caps.Version, caps.VersionRaw = v, raw
			}
		}
	}

	caps.Flags = deriveFlags(caps.Version)
	return caps, nil
}

func fetchVersionLegacy(ctx context.Context, c *transport.Classic, site string) (*version.Version, string, error) {
	resp, err := c.Do(ctx, "GET", legacyPath(site, "stat/sysinfo"), nil)
	if err != nil {
		return nil, "", err
	}
	var env models.LegacyEnvelope[models.Sysinfo]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, "", fmt.Errorf("decode sysinfo: %w", err)
	}
	if len(env.Data) == 0 || env.Data[0].Version == "" {
		return nil, "", fmt.Errorf("sysinfo response carried no version")
	}
	raw := env.Data[0].Version
	v, err := version.NewVersion(raw)
	if err != nil {
		// Present but unparseable: keep the raw string, leave parsed nil so
		// flags stay conservative.
		return nil, raw, nil
	}
	return v, raw, nil
}

func fetchSites(ctx context.Context, i *transport.Integration) ([]models.Site, error) {
	resp, err := i.Do(ctx, "GET", integrationPath("sites"), nil)
	if err != nil {
		return nil, err
	}
	var page models.IntegrationPage[models.Site]
	if err := json.Unmarshal(resp.Body, &page); err != nil {
		return nil, fmt.Errorf("decode sites: %w", err)
	}
	return page.Data, nil
}

func fetchVersionIntegration(ctx context.Context, i *transport.Integration) (*version.Version, string, bool) {
	resp, err := i.Do(ctx, "GET", integrationPath("application-info"), nil)
	if err != nil {
		return nil, "", false
	}
	var info struct {
		ApplicationVersion string `json:"applicationVersion"`
	}
	if err := json.Unmarshal(resp.Body, &info); err != nil || info.ApplicationVersion == "" {
		return nil, "", false
	}
	v, err := version.NewVersion(info.ApplicationVersion)
	if err != nil {
		return nil, info.ApplicationVersion, true
	}
	return v, info.ApplicationVersion, true
}
