package access

import (
	"context"
	"net/http"
	"net/url"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/access/wire"
)

// Access-policy routes on the Access v2 surface. The list is plural
// (access_policies); the create/item endpoints are singular (access_policy). The
// write body (AccessPolicyBo) and the read body (AccessPolicy) are different
// shapes — the controller binds the BO on POST/PUT but returns the richer read
// view on GET — so Create/Update and Get use distinct wire types. These paths
// are modeled from the undocumented access_policy controller and are
// provisional pending a live/HIL capture (see TODO.md).
const (
	accessPoliciesPath = "api/v2/access_policies"
	accessPolicyPath   = "api/v2/access_policy"
)

// AccessPolicyService is the AccessPolicies group: CRUD over Access policies.
type AccessPolicyService struct {
	core *core
}

func (s *AccessPolicyService) itemPath(id string) string {
	return accessPolicyPath + "/" + url.PathEscape(id)
}

// List returns every access policy in the read view.
func (s *AccessPolicyService) List(ctx context.Context) ([]wire.AccessPolicyBoAccessPolicy, error) {
	var out []wire.AccessPolicyBoAccessPolicy
	if err := do(ctx, s.core, http.MethodGet, accessPoliciesPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches one access policy by unique_id in the read view. Returns
// ErrNotFound if absent.
func (s *AccessPolicyService) Get(ctx context.Context, id string) (*wire.AccessPolicyBoAccessPolicy, error) {
	var out wire.AccessPolicyBoAccessPolicy
	if err := do(ctx, s.core, http.MethodGet, s.itemPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create makes a new access policy from the write DTO and returns the created
// object (carrying its assigned unique_id).
func (s *AccessPolicyService) Create(ctx context.Context, params wire.AccessPolicyBoAccessPolicyBo) (*wire.AccessPolicyBoAccessPolicyBo, error) {
	var out wire.AccessPolicyBoAccessPolicyBo
	if err := do(ctx, s.core, http.MethodPost, accessPolicyPath, params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update replaces an access policy's configuration by unique_id and returns the
// updated object. The params type is omitempty-tagged, so only the fields the
// caller sets are sent.
func (s *AccessPolicyService) Update(ctx context.Context, id string, params wire.AccessPolicyBoAccessPolicyBo) (*wire.AccessPolicyBoAccessPolicyBo, error) {
	var out wire.AccessPolicyBoAccessPolicyBo
	if err := do(ctx, s.core, http.MethodPut, s.itemPath(id), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes an access policy by unique_id. A missing policy is not an error.
func (s *AccessPolicyService) Delete(ctx context.Context, id string) error {
	if err := do[struct{}](ctx, s.core, http.MethodDelete, s.itemPath(id), nil, nil); err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	return nil
}
