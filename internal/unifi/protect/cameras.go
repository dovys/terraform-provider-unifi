package protect

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

// CameraService is the cameras group. A camera is adopted hardware with mutable
// config (doorbells are cameras — no separate type), so the resource layer
// configures an existing camera (GET-merge-PATCH of owned fields) keyed by mac.
type CameraService struct {
	Collection[wire.Camera]
}

// Forget unadopts the camera (POST /cameras/<id>/unadopt). Destructive and
// HIL-unverified (the recon console's camera is adopted) — used only when a
// resource sets forget_on_destroy.
func (s CameraService) Forget(ctx context.Context, id string) error {
	return decode[any](ctx, s.core, http.MethodPost, s.core.apiPath("cameras/"+url.PathEscape(id)+"/unadopt"), struct{}{}, nil)
}

// GetByMAC returns the camera with the given MAC (any separator/case), or
// ErrNotFound. The friendly import key: the private API has no GET-by-mac, so
// this lists and filters — the same friendliness unifi_network_device provides
// over the id-keyed surface.
func (s CameraService) GetByMAC(ctx context.Context, mac string) (*wire.Camera, error) {
	cams, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return findByMAC(cams, mac, func(c *wire.Camera) string { return c.Mac })
}

// normMAC normalizes a MAC for comparison: lowercase, separators stripped.
func normMAC(mac string) string {
	r := strings.NewReplacer(":", "", "-", "", ".", "", " ", "")
	return strings.ToLower(r.Replace(mac))
}

// findByMAC returns the first item whose MAC (via macOf) matches mac, normalized.
func findByMAC[T any](items []T, mac string, macOf func(*T) string) (*T, error) {
	want := normMAC(mac)
	for i := range items {
		if normMAC(macOf(&items[i])) == want {
			return &items[i], nil
		}
	}
	return nil, ErrNotFound
}
