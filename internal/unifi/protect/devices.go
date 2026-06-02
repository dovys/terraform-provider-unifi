package protect

import (
	"context"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

// The non-camera Protect devices: floodlights, UP-Sense sensors, chimes, and
// viewports. Each is configure-existing (GET-merge-PATCH of owned fields,
// keyed by mac) — no live instance on the recon console, so writes are
// HIL-verified. Each group embeds the generic collection and adds GetByMAC for
// the friendly import key.

// LightService is the floodlights group (/proxy/protect/api/lights).
type LightService struct {
	Collection[wire.Light]
}

// GetByMAC returns the light with the given MAC, or ErrNotFound.
func (s LightService) GetByMAC(ctx context.Context, mac string) (*wire.Light, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findByMAC(items, mac, func(l *wire.Light) string { return l.Mac })
}

// SensorService is the UP-Sense sensors group (/proxy/protect/api/sensors).
type SensorService struct {
	Collection[wire.Sensor]
}

// GetByMAC returns the sensor with the given MAC, or ErrNotFound.
func (s SensorService) GetByMAC(ctx context.Context, mac string) (*wire.Sensor, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findByMAC(items, mac, func(d *wire.Sensor) string { return d.Mac })
}

// ChimeService is the chimes group (/proxy/protect/api/chimes).
type ChimeService struct {
	Collection[wire.Chime]
}

// GetByMAC returns the chime with the given MAC, or ErrNotFound.
func (s ChimeService) GetByMAC(ctx context.Context, mac string) (*wire.Chime, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findByMAC(items, mac, func(d *wire.Chime) string { return d.Mac })
}

// ViewerService is the viewports group (/proxy/protect/api/viewers).
type ViewerService struct {
	Collection[wire.Viewer]
}

// GetByMAC returns the viewer with the given MAC, or ErrNotFound.
func (s ViewerService) GetByMAC(ctx context.Context, mac string) (*wire.Viewer, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findByMAC(items, mac, func(d *wire.Viewer) string { return d.Mac })
}
