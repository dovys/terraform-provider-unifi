package unifi

import (
	"context"
	"fmt"
)

const dynamicDNSType = "dynamicdns"

// DynamicDNS is a legacy `/rest/dynamicdns` document — a dynamic-DNS updater
// that publishes the gateway's WAN IP to a DDNS provider. x_password is the
// provider credential (sensitive). Modeled from documented controller behavior
// — the field set is HIL-unverified (this network has no DDNS configs) — see
// TODO.md.
type DynamicDNS struct {
	ID        string `json:"_id,omitempty"`
	SiteID    string `json:"site_id,omitempty"`
	Service   string `json:"service,omitempty"`    // provider key, e.g. dyndns, namecheap, noip, custom
	HostName  string `json:"host_name,omitempty"`  // the FQDN to keep updated
	Server    string `json:"server,omitempty"`     // update server (for service=custom)
	Login     string `json:"login,omitempty"`      // provider username
	XPassword string `json:"x_password,omitempty"` // provider password/token (sensitive)
	Interface string `json:"interface,omitempty"`  // WAN interface to publish, e.g. wan
}

// Dynamic-DNS CRUD (legacy /rest/dynamicdns). Update is GET-merge-PUT so
// controller-managed fields survive.

func (c *client) ListDynamicDNS(ctx context.Context) ([]DynamicDNS, error) {
	return legacyList[DynamicDNS](ctx, c, dynamicDNSType)
}

func (c *client) GetDynamicDNS(ctx context.Context, id string) (*DynamicDNS, error) {
	return legacyGet[DynamicDNS](ctx, c, dynamicDNSType, id)
}

func (c *client) CreateDynamicDNS(ctx context.Context, d *DynamicDNS) (*DynamicDNS, error) {
	return legacyCreate[DynamicDNS](ctx, c, dynamicDNSType, d)
}

func (c *client) UpdateDynamicDNS(ctx context.Context, d *DynamicDNS) (*DynamicDNS, error) {
	if d.ID == "" {
		return nil, fmt.Errorf("UpdateDynamicDNS: missing id")
	}
	return legacyMergePut[DynamicDNS](ctx, c, dynamicDNSType, d.ID, d)
}

func (c *client) DeleteDynamicDNS(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, dynamicDNSType, id)
}
