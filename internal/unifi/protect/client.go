package protect

import (
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/transport"
)

// Client is the UniFi Protect application client — the c.Protect() arm of the
// substruct API. Functional groups hang off it; each exposes its CRUD
// collection and actions:
//
//	p := c.Protect()
//	p.Cameras().GetByMAC(ctx, mac)
//	p.NVR().Get(ctx)
//
// The client holds no state beyond the shared session (in core). Protect is a
// console-global app, so there is no site.
type Client struct {
	core *core
}

// New builds a Protect client over an authenticated classic session. The
// session is the same cookie+CSRF session the Network app uses — no api_key.
func New(session *transport.Classic) *Client {
	return &Client{core: &core{session: session}}
}

// Cameras returns the cameras group (cameras + doorbells).
func (c *Client) Cameras() CameraService {
	return CameraService{Collection[wire.Camera]{core: c.core, name: "cameras"}}
}

// Lights returns the floodlights group.
func (c *Client) Lights() LightService {
	return LightService{Collection[wire.Light]{core: c.core, name: "lights"}}
}

// Sensors returns the UP-Sense sensors group.
func (c *Client) Sensors() SensorService {
	return SensorService{Collection[wire.Sensor]{core: c.core, name: "sensors"}}
}

// Chimes returns the chimes group.
func (c *Client) Chimes() ChimeService {
	return ChimeService{Collection[wire.Chime]{core: c.core, name: "chimes"}}
}

// Viewers returns the viewports group.
func (c *Client) Viewers() ViewerService {
	return ViewerService{Collection[wire.Viewer]{core: c.core, name: "viewers"}}
}

// Liveviews returns the liveviews group (multi-camera layouts; full CRUD).
func (c *Client) Liveviews() LiveviewService {
	return LiveviewService{Collection[wire.Liveview]{core: c.core, name: "liveviews"}}
}

// NVR returns the NVR group (singleton config).
func (c *Client) NVR() NVRService {
	return NVRService{core: c.core}
}

// Automations returns the automations group (list + batch-PATCH; no per-id or
// create/delete route — see AutomationService).
func (c *Client) Automations() AutomationService {
	return AutomationService{core: c.core}
}
