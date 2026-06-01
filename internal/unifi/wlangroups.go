package unifi

import (
	"context"
	"fmt"
)

const wlanGroupType = "wlangroup"

// WLANGroup is a legacy `/rest/wlangroup` document — a named grouping of WLANs
// (the older organizational unit that predates AP groups). Confirmed against a
// live controller: only `name` is user-configurable; the rest are controller-
// managed protection flags. The controller ships a protected default group.
type WLANGroup struct {
	ID           string `json:"_id,omitempty"`
	SiteID       string `json:"site_id,omitempty"`
	Name         string `json:"name"`
	AttrNoDelete *bool  `json:"attr_no_delete,omitempty"` // controller-protected (the default group)
	AttrNoEdit   *bool  `json:"attr_no_edit,omitempty"`
}

// WLAN group CRUD (legacy /rest/wlangroup). Update is GET-merge-PUT so
// controller-managed fields survive.

func (c *client) ListWLANGroups(ctx context.Context) ([]WLANGroup, error) {
	return legacyList[WLANGroup](ctx, c, wlanGroupType)
}

func (c *client) GetWLANGroup(ctx context.Context, id string) (*WLANGroup, error) {
	return legacyGet[WLANGroup](ctx, c, wlanGroupType, id)
}

func (c *client) CreateWLANGroup(ctx context.Context, g *WLANGroup) (*WLANGroup, error) {
	return legacyCreate[WLANGroup](ctx, c, wlanGroupType, g)
}

func (c *client) UpdateWLANGroup(ctx context.Context, g *WLANGroup) (*WLANGroup, error) {
	if g.ID == "" {
		return nil, fmt.Errorf("UpdateWLANGroup: missing id")
	}
	return legacyMergePut[WLANGroup](ctx, c, wlanGroupType, g.ID, g)
}

func (c *client) DeleteWLANGroup(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, wlanGroupType, id)
}
