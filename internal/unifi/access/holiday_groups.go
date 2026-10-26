package access

import (
	"context"
	"net/http"
	"net/url"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

// Holiday-group routes on the Access v2 surface. The list is plural; the
// create/item endpoints are singular with the unique_id in the path. List
// returns the thin "simple" view (with a holiday count); Get returns the detail
// view (with the embedded holidays). These paths are modeled from the
// undocumented holiday controller and are provisional pending a live/HIL
// capture (see TODO.md).
const (
	holidayGroupsPath = "api/v2/holiday_groups"
	holidayGroupPath  = "api/v2/holiday_group"
)

// HolidayGroupService is the HolidayGroups group: CRUD over Access holiday
// groups (named sets of holidays used to scope unlock schedules).
type HolidayGroupService struct {
	core *core
}

func (s *HolidayGroupService) itemPath(id string) string {
	return holidayGroupPath + "/" + url.PathEscape(id)
}

// List returns every holiday group in the simple view (counts, no member detail).
func (s *HolidayGroupService) List(ctx context.Context) ([]wire.HolidayBoHolidayGroupSimple, error) {
	var out []wire.HolidayBoHolidayGroupSimple
	if err := do(ctx, s.core, http.MethodGet, holidayGroupsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches one holiday group by unique_id in the detail view (with member
// holidays). Returns ErrNotFound if absent.
func (s *HolidayGroupService) Get(ctx context.Context, id string) (*wire.HolidayBoHolidayGroupDetail, error) {
	var out wire.HolidayBoHolidayGroupDetail
	if err := do(ctx, s.core, http.MethodGet, s.itemPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create makes a new holiday group and returns the created object (carrying its
// assigned unique_id).
func (s *HolidayGroupService) Create(ctx context.Context, params wire.HolidayBoHolidayGroupParams) (*wire.HolidayBoHolidayGroupDetail, error) {
	var out wire.HolidayBoHolidayGroupDetail
	if err := do(ctx, s.core, http.MethodPost, holidayGroupPath, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update replaces a holiday group's configuration by unique_id and returns the
// updated object. The params type is omitempty-tagged, so only the fields the
// caller sets are sent.
func (s *HolidayGroupService) Update(ctx context.Context, id string, params wire.HolidayBoHolidayGroupParams) (*wire.HolidayBoHolidayGroupDetail, error) {
	var out wire.HolidayBoHolidayGroupDetail
	if err := do(ctx, s.core, http.MethodPut, s.itemPath(id), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a holiday group by unique_id. A missing group is not an error.
func (s *HolidayGroupService) Delete(ctx context.Context, id string) error {
	if err := do[struct{}](ctx, s.core, http.MethodDelete, s.itemPath(id), nil, nil); err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	return nil
}
