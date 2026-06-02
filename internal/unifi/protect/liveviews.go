package protect

import (
	"context"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect/wire"
)

// LiveviewService is the liveviews group (/proxy/protect/api/liveviews). Unlike
// the device groups, a liveview is a pure config object the provider fully owns:
// full CRUD (the collection's Create/Update/Delete all apply), keyed by id with
// a friendly import-by-name.
type LiveviewService struct {
	Collection[wire.Liveview]
}

// GetByName returns the first liveview with the given name, or ErrNotFound — a
// friendly import key alongside the 24-hex id.
func (s LiveviewService) GetByName(ctx context.Context, name string) (*wire.Liveview, error) {
	views, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range views {
		if views[i].Name != nil && *views[i].Name == name {
			return &views[i], nil
		}
	}
	return nil, ErrNotFound
}
