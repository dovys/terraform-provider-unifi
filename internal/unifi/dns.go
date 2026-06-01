package unifi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

// Local DNS CRUD. Routing is the v2 /static-dns surface today (router
// OpLocalDNS). The router flips to Integration dns/policies once that path is
// implemented and firmware supports it — a one-line policy change, no schema
// change (invariant #2). Until then FeatureFlags.IntegrationDNSPolicies is
// false, so OpLocalDNS resolves to v2.

const staticDNSPath = "static-dns"

func (c *client) ListDNSRecords(ctx context.Context) ([]models.StaticDNSRecord, error) {
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	var out []models.StaticDNSRecord
	if err := v2Decode(ctx, c, http.MethodGet, staticDNSPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) GetDNSRecord(ctx context.Context, id string) (*models.StaticDNSRecord, error) {
	// v2 static-dns has no reliable GET-by-id; list and filter.
	records, err := c.ListDNSRecords(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].ID == id {
			return &records[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *client) CreateDNSRecord(ctx context.Context, r *models.StaticDNSRecord) (*models.StaticDNSRecord, error) {
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	var out models.StaticDNSRecord
	if err := v2Decode(ctx, c, http.MethodPost, staticDNSPath, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) UpdateDNSRecord(ctx context.Context, r *models.StaticDNSRecord) (*models.StaticDNSRecord, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateDNSRecord: missing id")
	}
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	// v2 write contract: _id in both URL and body.
	var out models.StaticDNSRecord
	if err := v2Decode(ctx, c, http.MethodPut, staticDNSPath+"/"+r.ID, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) DeleteDNSRecord(ctx context.Context, id string) error {
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return errIntegrationDNSUnimplemented()
	}
	err := v2Decode[any](ctx, c, http.MethodDelete, staticDNSPath+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func errIntegrationDNSUnimplemented() error {
	return &CapabilityError{
		Capability: "integration-dns-policies",
		Detail:     "local DNS via the Integration API is not implemented yet; the provider uses v2 /static-dns",
	}
}
