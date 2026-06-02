package network

import (
	"context"
	"fmt"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// DNSService is the DNS group. Today it owns local static-DNS records on the v2
// /static-dns surface; DNS policies (v2 dns-policies) join in a later phase.
type DNSService struct {
	core *core
}

// Records is the static-DNS record collection (v2 /static-dns).
func (s DNSService) Records() RecordCollection {
	return RecordCollection{V2Collection[wire.StaticDns]{core: s.core, path: "static-dns"}}
}

// RecordCollection is the static-DNS CRUD collection. It embeds the generic v2
// collection (List/Create/Delete) and overrides the operations the v2
// static-dns surface shapes differently: there is no reliable GET-by-id, and
// PUT requires _id in both the URL and the body.
type RecordCollection struct {
	V2Collection[wire.StaticDns]
}

// Get returns the record with the given _id, or ErrNotFound. The v2 static-dns
// surface has no GET-by-id, so this lists and filters.
func (s RecordCollection) Get(ctx context.Context, id string) (*wire.StaticDns, error) {
	records, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].UnderscoreId != nil && *records[i].UnderscoreId == id {
			return &records[i], nil
		}
	}
	return nil, ErrNotFound
}

// Update PUTs the record, sending its _id in both the URL and the body per the
// v2 write contract. The record's _id (UnderscoreId) must be set.
func (s RecordCollection) Update(ctx context.Context, r *wire.StaticDns) (*wire.StaticDns, error) {
	if r.UnderscoreId == nil || *r.UnderscoreId == "" {
		return nil, fmt.Errorf("network: update static-dns record: missing _id")
	}
	return s.V2Collection.Update(ctx, *r.UnderscoreId, r)
}
