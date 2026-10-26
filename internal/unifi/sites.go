package unifi

import (
	"context"
	"fmt"
	"net/http"
)

// Sites.
//
// Reads list the sites the admin can access via legacy `/api/self/sites`.
// Writes use the legacy `/cmd/sitemgr` command surface. The sitemgr wire shapes
// are not in the single-site fixture, so the site resource is acceptance-tested
// against a multi-site controller (Tier 2/3), not unit-asserted here.

// Site is a legacy site object (GET /api/self/sites). Name is the slug (the
// legacy/v2 site key, e.g. "default"); Desc is the human label ("Default"); ID
// is the controller `_id`. The provider matches on the slug (architecture.md §3).
type Site struct {
	ID   string `json:"_id"`
	Name string `json:"name"` // slug
	Desc string `json:"desc"` // display name
}

// LegacySite is the object the legacy `/cmd/sitemgr` returns for add-site:
// Name is the slug (the v2/legacy site key) and Desc is the human display name.
// ID is the Mongo `_id` used by delete/update-site.
type LegacySite struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

func (c *client) ListSites(ctx context.Context) ([]Site, error) {
	return legacyData[Site](ctx, c, http.MethodGet, "/proxy/network/api/self/sites", nil)
}

func (c *client) sitemgr(ctx context.Context, body map[string]any) ([]LegacySite, error) {
	return legacyData[LegacySite](ctx, c, http.MethodPost, legacyPath(c.cfg.Site, "cmd/sitemgr"), body)
}

func (c *client) CreateSite(ctx context.Context, description string) (*LegacySite, error) {
	items, err := c.sitemgr(ctx, map[string]any{"cmd": "add-site", "desc": description})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("add-site returned no site")
	}
	return &items[0], nil
}

func (c *client) UpdateSite(ctx context.Context, legacyID, description string) error {
	_, err := c.sitemgr(ctx, map[string]any{"cmd": "update-site", "site": legacyID, "desc": description})
	return err
}

func (c *client) DeleteSite(ctx context.Context, legacyID string) error {
	_, err := c.sitemgr(ctx, map[string]any{"cmd": "delete-site", "site": legacyID})
	if isNotFound(err) {
		return nil
	}
	return err
}
