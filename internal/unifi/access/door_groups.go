package access

import (
	"context"
	"net/http"
	"net/url"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

// Door-group API routes on the Access v2 surface. The list is plural; the item
// endpoints are singular with the unique_id in the path. These paths are modeled
// from the undocumented door_group_controller surface and are provisional
// pending a live/HIL capture (see TODO.md).
const (
	doorGroupsPath = "api/v2/access/door_groups"
	doorGroupPath  = "api/v2/access/door_group"
)

// DoorGroupService is the DoorGroups group: CRUD over Access door groups plus
// the dedicated door-membership read. The list and item reads use distinct wire
// types — List returns the controller "info" view (with member counts), Get the
// thin domain object — matching the controller's two response shapes.
type DoorGroupService struct {
	core *core
}

func (s *DoorGroupService) itemPath(id string) string {
	return doorGroupPath + "/" + url.PathEscape(id)
}

// List returns every door group with its member counts.
func (s *DoorGroupService) List(ctx context.Context) ([]wire.DoorGroupControllerDoorGroupInfo, error) {
	var out []wire.DoorGroupControllerDoorGroupInfo
	if err := do(ctx, s.core, http.MethodGet, doorGroupsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches one door group by its unique_id. Returns ErrNotFound if absent.
func (s *DoorGroupService) Get(ctx context.Context, id string) (*wire.DoorGroupBoDoorGroup, error) {
	var out wire.DoorGroupBoDoorGroup
	if err := do(ctx, s.core, http.MethodGet, s.itemPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DoorIDs returns the door member ids of a group. The members live behind a
// dedicated endpoint rather than on the group object; the response is decoded
// as a flat id list (shape provisional — see TODO.md).
func (s *DoorGroupService) DoorIDs(ctx context.Context, id string) ([]string, error) {
	var out []string
	if err := do(ctx, s.core, http.MethodGet, s.itemPath(id)+"/door_ids", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Create makes a new door group and returns the created object (carrying its
// assigned unique_id).
func (s *DoorGroupService) Create(ctx context.Context, params wire.DoorGroupControllerDoorGroupParams) (*wire.DoorGroupBoDoorGroup, error) {
	var out wire.DoorGroupBoDoorGroup
	if err := do(ctx, s.core, http.MethodPost, doorGroupPath, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update replaces a door group's configuration by unique_id and returns the
// updated object. The params type is omitempty-tagged, so only the fields the
// caller sets are sent.
func (s *DoorGroupService) Update(ctx context.Context, id string, params wire.DoorGroupControllerDoorGroupParams) (*wire.DoorGroupBoDoorGroup, error) {
	var out wire.DoorGroupBoDoorGroup
	if err := do(ctx, s.core, http.MethodPut, s.itemPath(id), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a door group by unique_id. A missing group is not an error.
func (s *DoorGroupService) Delete(ctx context.Context, id string) error {
	if err := do[struct{}](ctx, s.core, http.MethodDelete, s.itemPath(id), nil, nil); err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	return nil
}
