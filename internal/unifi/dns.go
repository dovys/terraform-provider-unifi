package unifi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Local DNS CRUD. Routing is the v2 /static-dns surface today (router
// OpLocalDNS). The router flips to Integration dns/policies once that path is
// implemented and firmware supports it — a one-line policy change, no schema
// change (invariant #2). Until then FeatureFlags.IntegrationDNSPolicies is
// false, so OpLocalDNS resolves to v2.

const staticDNSPath = "static-dns"

// StaticDNSRecord is a v2 `/static-dns` local DNS record. The v2 surface is
// bare JSON (no envelope). On 10.1+ firmware the router may move local DNS to
// the Integration dns/policies surface with no schema change (invariant #2);
// this struct models the v2 wire shape used today.
type StaticDNSRecord struct {
	ID         string `json:"_id,omitempty"`
	Key        string `json:"key"`               // the DNS name, e.g. "host.example.com"
	RecordType string `json:"record_type"`       // A | AAAA | CNAME | MX | TXT | SRV | NS
	Value      string `json:"value"`             // the target (IP / hostname / text)
	Enabled    *bool  `json:"enabled,omitempty"` // pointer: omit vs explicit false
	TTL        *int   `json:"ttl,omitempty"`     // 0 = controller default
	Port       *int   `json:"port,omitempty"`    // SRV
	Priority   *int   `json:"priority,omitempty"`
	Weight     *int   `json:"weight,omitempty"`
}

func (c *client) ListDNSRecords(ctx context.Context) ([]StaticDNSRecord, error) {
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	var out []StaticDNSRecord
	if err := v2Decode(ctx, c, http.MethodGet, staticDNSPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) GetDNSRecord(ctx context.Context, id string) (*StaticDNSRecord, error) {
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

func (c *client) CreateDNSRecord(ctx context.Context, r *StaticDNSRecord) (*StaticDNSRecord, error) {
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	var out StaticDNSRecord
	if err := v2Decode(ctx, c, http.MethodPost, staticDNSPath, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) UpdateDNSRecord(ctx context.Context, r *StaticDNSRecord) (*StaticDNSRecord, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateDNSRecord: missing id")
	}
	if c.router.Backend(OpLocalDNS) == BackendIntegration {
		return nil, errIntegrationDNSUnimplemented()
	}
	// v2 write contract: _id in both URL and body.
	var out StaticDNSRecord
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
