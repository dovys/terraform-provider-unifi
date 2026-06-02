package network

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// DevicesService is the devices group: UniFi hardware (APs, switches, gateways).
// Reads come from the authoritative legacy /stat/device list (telemetry + the
// full config document); config writes are a partial PUT to /rest/device/<id>
// (GET-merge-PUT, owned fields only); adopt/forget go through /cmd/devmgr.
// The adopt/forget verbs are HIL-unverified (no adoptable hardware on the recon
// network) — see TODO.md.
type DevicesService struct{ core *core }

// Devices returns the devices group.
func (c *Client) Devices() DevicesService { return DevicesService{core: c.core} }

// configColl is the /rest/device collection used for the merge-update path.
func (s DevicesService) configColl() LegacyCollection[wire.Device] {
	return LegacyCollection[wire.Device]{core: s.core, typ: "device"}
}

// List returns every device on the site (legacy /stat/device).
func (s DevicesService) List(ctx context.Context) ([]wire.Device, error) {
	return legacyData[wire.Device](ctx, s.core, http.MethodGet, s.core.legacyPath("stat/device"), nil)
}

// Get returns the device with the given controller _id, or ErrNotFound. Read is
// from the authoritative /stat/device list (the /rest/device GET is thin).
func (s DevicesService) Get(ctx context.Context, id string) (*wire.Device, error) {
	devices, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].UnderscoreId != nil && *devices[i].UnderscoreId == id {
			return &devices[i], nil
		}
	}
	return nil, ErrNotFound
}

// GetByMAC returns the device with the given MAC (case-insensitive), or
// ErrNotFound — the lookup adopt-if-needed does before configuring.
//
// wire.Device is typed from the writable api/fields set and so omits read-only
// stat fields like "mac"; the MAC match scans the raw /stat/device documents,
// then decodes the matched one into the typed Device. (Enriching device.json
// with the stat/identity fields is tracked in TODO.md.)
func (s DevicesService) GetByMAC(ctx context.Context, mac string) (*wire.Device, error) {
	want := strings.ToLower(strings.TrimSpace(mac))
	raws, err := legacyData[map[string]json.RawMessage](ctx, s.core, http.MethodGet, s.core.legacyPath("stat/device"), nil)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		var m string
		if rawMac, ok := raw["mac"]; ok && json.Unmarshal(rawMac, &m) == nil && strings.ToLower(m) == want {
			var d wire.Device
			b, err := json.Marshal(raw)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(b, &d); err != nil {
				return nil, err
			}
			return &d, nil
		}
	}
	return nil, ErrNotFound
}

// UpdateConfig applies the owned fields onto the live /rest/device/<id> document
// via GET-merge-PUT, leaving controller-managed and unmodeled fields intact.
func (s DevicesService) UpdateConfig(ctx context.Context, id string, owned any) (*wire.Device, error) {
	return s.configColl().UpdateMerge(ctx, id, owned)
}

// Adopt adopts a pending device into the site (/cmd/devmgr adopt). HIL-unverified.
func (s DevicesService) Adopt(ctx context.Context, mac string) error {
	return s.devmgr(ctx, "adopt", mac)
}

// Forget removes a device from the site (/cmd/devmgr delete-device). HIL-unverified.
func (s DevicesService) Forget(ctx context.Context, mac string) error {
	return s.devmgr(ctx, "delete-device", mac)
}

func (s DevicesService) devmgr(ctx context.Context, cmd, mac string) error {
	body := map[string]any{"cmd": cmd, "mac": strings.ToLower(strings.TrimSpace(mac))}
	_, err := legacyData[map[string]any](ctx, s.core, http.MethodPost, s.core.legacyPath("cmd/devmgr"), body)
	return err
}
