package unifi

import (
	"context"
	"fmt"
	"net/http"
)

// Sites & application info.
//
// Reads use the Integration API (sites by UUID + internalReference slug;
// router OpSiteRead). Site writes use the legacy /cmd/sitemgr command surface
// (the Integration API has no site CRUD). The sitemgr wire shapes are not in
// the single-site fixture, so the site resource is acceptance-tested against a
// multi-site controller (Tier 2/3), not unit-asserted here.

// Site is an Integration v1 site object (GET /integration/v1/sites).
//
// Three namespaces name the same site: the Integration API keys by UUID (ID),
// legacy/v2 key by the slug in InternalReference (e.g. "default"), and Name is
// the human label ("Default"). The provider matches on InternalReference
// (architecture.md §5).
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// LegacySite is the object the legacy `/cmd/sitemgr` returns for add-site:
// Name is the slug (the v2/legacy site key, == Site.InternalReference) and Desc
// is the human display name. ID is the Mongo `_id` used by delete/update-site.
type LegacySite struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ApplicationInfo is the Integration v1 `/application-info` response. Present
// on Network 9.3+ (404s below). Used by the data source and as a version
// fallback when only the Integration API key is configured.
type ApplicationInfo struct {
	ApplicationVersion string `json:"applicationVersion"`
}

func (c *client) ListSites(ctx context.Context) ([]Site, error) {
	return integrationListAll[Site](ctx, c, "sites")
}

func (c *client) GetApplicationInfo(ctx context.Context) (*ApplicationInfo, error) {
	return integrationGet[ApplicationInfo](ctx, c, "application-info")
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
