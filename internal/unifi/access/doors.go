package access

import (
	"context"
	"net/http"
	"net/url"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

// Door routes on the Access v2 surface. Doors are discovered location objects
// (adopted hardware lives under them), not created through the API: the surface
// exposes a list and a per-door update, but no create/delete. There is no
// per-door GET, so Get lists and filters by Location.unique_id. These paths are
// modeled from the reverse-engineered location controller and are provisional
// pending a live/HIL capture (see TODO.md).
const (
	doorsPath = "api/v2/locations/doors"
	doorPath  = "api/v2/locations/door"
)

// DoorService is the Doors group: list + per-door configuration update. Doors
// are update-only (no create/delete) — the resource configures an existing,
// discovered door.
type DoorService struct {
	core *core
}

func (s *DoorService) itemPath(id string) string {
	return doorPath + "/" + url.PathEscape(id)
}

// List returns every door with its location embed and live state.
func (s *DoorService) List(ctx context.Context) ([]wire.LocationBoDoor, error) {
	var out []wire.LocationBoDoor
	if err := do(ctx, s.core, http.MethodGet, doorsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns one door by its location unique_id. The surface has no per-door
// GET, so it lists and filters. Returns ErrNotFound if no door matches.
func (s *DoorService) Get(ctx context.Context, id string) (*wire.LocationBoDoor, error) {
	doors, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range doors {
		d := &doors[i]
		if d.Location != nil && d.Location.UniqueId != nil && *d.Location.UniqueId == id {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

// Update configures an existing door by unique_id and returns the updated door.
func (s *DoorService) Update(ctx context.Context, id string, params wire.LocationBoUpdateDoorParams) (*wire.LocationBoDoor, error) {
	var out wire.LocationBoDoor
	if err := do(ctx, s.core, http.MethodPut, s.itemPath(id), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
