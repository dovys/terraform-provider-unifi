package unifi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Sysinfo is the subset of the object returned by legacy GET /stat/sysinfo that
// the provider needs. The Network application version lives here and is present
// whenever the classic (cookie) session is authenticated.
type Sysinfo struct {
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
	Name    string `json:"name,omitempty"`
}

// Capabilities is the cached result of the Configure-time probe. It carries the
// detected Network application version, which future resources gate on (e.g.
// the classic-vs-zone firewall mode); that lazy, count-based detection lives in
// the firewall resources, not here (architecture.md §5).
type Capabilities struct {
	// Version is the parsed Network application version (nil if undetected or
	// unparseable; VersionRaw still carries the reported string).
	Version *version.Version
	// VersionRaw is the version string as the controller reported it.
	VersionRaw string
}

// probe runs the Configure-time capability detection over the classic session:
// it reads the controller version from legacy /stat/sysinfo.
func probe(ctx context.Context, classic *transport.Classic, site string) (*Capabilities, error) {
	v, raw, err := fetchVersionLegacy(ctx, classic, site)
	if err != nil {
		return nil, fmt.Errorf("detect controller version: %w", err)
	}
	return &Capabilities{Version: v, VersionRaw: raw}, nil
}

func fetchVersionLegacy(ctx context.Context, c *transport.Classic, site string) (*version.Version, string, error) {
	resp, err := c.Do(ctx, "GET", legacyPath(site, "stat/sysinfo"), nil)
	if err != nil {
		return nil, "", err
	}
	var env LegacyEnvelope[Sysinfo]
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, "", fmt.Errorf("decode sysinfo: %w", err)
	}
	if len(env.Data) == 0 || env.Data[0].Version == "" {
		return nil, "", fmt.Errorf("sysinfo response carried no version")
	}
	raw := env.Data[0].Version
	v, err := version.NewVersion(raw)
	if err != nil {
		// Present but unparseable: keep the raw string, leave parsed nil.
		return nil, raw, nil
	}
	return v, raw, nil
}
