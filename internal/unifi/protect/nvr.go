package protect

import (
	"context"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

// NVRService is the NVR group (/proxy/protect/api/nvr) — a singleton. There is
// no id and no collection: GET returns the one NVR config, PATCH updates it.
// The NVR is always present, so Get never 404s; the nvr_settings resource is
// import/update-only (destroy = state removal).
type NVRService struct {
	core *core
}

// Get returns the NVR configuration document.
func (s NVRService) Get(ctx context.Context) (*wire.Nvr, error) {
	var out wire.Nvr
	if err := decode(ctx, s.core, http.MethodGet, s.core.apiPath("nvr"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update PATCHes an owned-field overlay onto the NVR and returns the stored copy.
func (s NVRService) Update(ctx context.Context, owned any) (*wire.Nvr, error) {
	var out wire.Nvr
	if err := decode(ctx, s.core, http.MethodPatch, s.core.apiPath("nvr"), owned, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Version returns the running Protect application version, read from the NVR
// document — the source for the Configure-time capability gate. Empty if the
// controller omits it.
func (s NVRService) Version(ctx context.Context) (string, error) {
	nvr, err := s.Get(ctx)
	if err != nil {
		return "", err
	}
	if nvr.Version == nil {
		return "", nil
	}
	return *nvr.Version, nil
}
