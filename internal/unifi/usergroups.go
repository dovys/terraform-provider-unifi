package unifi

import (
	"context"
	"fmt"
)

const userGroupType = "usergroup"

// UserGroup is a legacy `/rest/usergroup` document — a bandwidth/QoS group that
// caps a client's up/down rate (confirmed against a live UCG Max). The
// controller ships a protected default group (attr_no_delete). Rate caps are in
// kbps; omit a cap for no limit. Pointers + omitempty give three-state semantics
// on GET-merge-PUT (nil = leave the controller's value, set = write it).
type UserGroup struct {
	ID             string `json:"_id,omitempty"`               // controller object ID; the resource import key
	SiteID         string `json:"site_id,omitempty"`           // owning site (controller-assigned)
	Name           string `json:"name"`                        // display name
	QOSRateMaxUp   *int   `json:"qos_rate_max_up,omitempty"`   // max upload rate (kbps); absent = unlimited
	QOSRateMaxDown *int   `json:"qos_rate_max_down,omitempty"` // max download rate (kbps); absent = unlimited
	AttrNoDelete   *bool  `json:"attr_no_delete,omitempty"`    // controller-protected (the default group)
}

// User group CRUD (legacy /rest/usergroup). Update is GET-merge-PUT so
// controller-managed fields survive.

func (c *client) ListUserGroups(ctx context.Context) ([]UserGroup, error) {
	return legacyList[UserGroup](ctx, c, userGroupType)
}

func (c *client) GetUserGroup(ctx context.Context, id string) (*UserGroup, error) {
	return legacyGet[UserGroup](ctx, c, userGroupType, id)
}

func (c *client) CreateUserGroup(ctx context.Context, g *UserGroup) (*UserGroup, error) {
	return legacyCreate[UserGroup](ctx, c, userGroupType, g)
}

func (c *client) UpdateUserGroup(ctx context.Context, g *UserGroup) (*UserGroup, error) {
	if g.ID == "" {
		return nil, fmt.Errorf("UpdateUserGroup: missing id")
	}
	return legacyMergePut[UserGroup](ctx, c, userGroupType, g.ID, g)
}

func (c *client) DeleteUserGroup(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, userGroupType, id)
}
