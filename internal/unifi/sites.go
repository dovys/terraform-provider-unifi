package unifi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

// Sites & application info.
//
// Reads use the Integration API (sites by UUID + internalReference slug;
// router OpSiteRead). Site writes use the legacy /cmd/sitemgr command surface
// (the Integration API has no site CRUD). The sitemgr wire shapes are not in
// the single-site fixture, so the site resource is acceptance-tested against a
// multi-site controller (Tier 2/3), not unit-asserted here.

func (c *client) ListSites(ctx context.Context) ([]models.Site, error) {
	return integrationListAll[models.Site](ctx, c, "sites")
}

func (c *client) GetApplicationInfo(ctx context.Context) (*models.ApplicationInfo, error) {
	return integrationGet[models.ApplicationInfo](ctx, c, "application-info")
}

func (c *client) sitemgr(ctx context.Context, body map[string]any) ([]models.LegacySite, error) {
	return legacyData[models.LegacySite](ctx, c, http.MethodPost, legacyPath(c.cfg.Site, "cmd/sitemgr"), body)
}

func (c *client) CreateSite(ctx context.Context, description string) (*models.LegacySite, error) {
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
