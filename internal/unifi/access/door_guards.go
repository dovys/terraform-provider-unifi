package access

import (
	"context"
	"net/http"
	"net/url"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

// Door-guard routes on the Access v2 surface. A door guard is a per-door
// configuration (the set of users authorized to act as guards for that door),
// read and written under the door's path. There is no create or delete — the
// guard set is replaced in place via PUT, so it is an update-only sub-resource
// keyed by door_id. Modeled from the reverse-engineered door_guard controller;
// provisional pending a live/HIL capture (see TODO.md). The user_ids are opaque
// references the operator supplies — the provider does not model Access users.
const doorGuardSuffix = "/door_guard"

// DoorGuardService is the DoorGuards group: read + replace the guard user set of
// a door.
type DoorGuardService struct {
	core *core
}

func (s *DoorGuardService) path(doorID string) string {
	return doorPath + "/" + url.PathEscape(doorID) + doorGuardSuffix
}

// Get returns the door-guard configuration of a door. Returns ErrNotFound if the
// door is absent.
func (s *DoorGuardService) Get(ctx context.Context, doorID string) (*wire.DoorGuardBoDoorGuardBo, error) {
	var out wire.DoorGuardBoDoorGuardBo
	if err := do(ctx, s.core, http.MethodGet, s.path(doorID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update replaces the guard user set of a door and returns the updated
// configuration.
func (s *DoorGuardService) Update(ctx context.Context, doorID string, params wire.DoorGuardBoUpdateDoorGuard) (*wire.DoorGuardBoDoorGuardBo, error) {
	var out wire.DoorGuardBoDoorGuardBo
	if err := do(ctx, s.core, http.MethodPut, s.path(doorID), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
