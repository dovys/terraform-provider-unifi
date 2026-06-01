package unifi

import (
	"context"
	"fmt"
)

const (
	radiusProfileType = "radiusprofile"
	accountType       = "account"
)

// RadiusServer is one entry in a RADIUS profile's auth or accounting server
// list. The list is ordered (primary, then fallbacks); x_secret is the shared
// secret and is sensitive. The controller echoes x_secret back on read.
type RadiusServer struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	XSecret string `json:"x_secret,omitempty"`
}

// RadiusProfile is a legacy `/rest/radiusprofile` document — a named set of
// RADIUS servers (and the assignment behavior) that WLANs and wired 802.1X
// reference for authentication/accounting (confirmed against a live UCG Max:
// name, auth_servers[]{ip,port,x_secret}, acct_servers[], use_usg_auth_server,
// attr_no_edit/attr_no_delete). The controller ships a protected default
// profile. The server lists are full-replace (non-omitempty) so removing a
// server actually drops it; everything else is omitempty/pointer so a minimal
// profile leaves the controller's defaults untouched (GET-merge-PUT).
type RadiusProfile struct {
	ID                    string         `json:"_id,omitempty"`
	SiteID                string         `json:"site_id,omitempty"`
	Name                  string         `json:"name"`
	AuthServers           []RadiusServer `json:"auth_servers"` // full-replace: empty list clears the servers
	AcctServers           []RadiusServer `json:"acct_servers"` // full-replace
	UseUSGAuthServer      *bool          `json:"use_usg_auth_server,omitempty"`
	UseUSGAcctServer      *bool          `json:"use_usg_acct_server,omitempty"`
	InterimUpdateEnabled  *bool          `json:"interim_update_enabled,omitempty"`
	InterimUpdateInterval *int           `json:"interim_update_interval,omitempty"`
	VLANEnabled           *bool          `json:"vlan_enabled,omitempty"`
	VLANWLANMode          string         `json:"vlan_wlan_mode,omitempty"` // disabled | optional | required
	AttrNoDelete          *bool          `json:"attr_no_delete,omitempty"`
	AttrNoEdit            *bool          `json:"attr_no_edit,omitempty"`
}

// RadiusAccount is a legacy `/rest/account` document — a RADIUS *user* served by
// the gateway's built-in RADIUS server (username + password, with optional
// dynamic-VLAN tunnel attributes). x_password is the secret. The tunnel fields
// drive RADIUS-assigned VLAN: tunnel_type=13 (VLAN), tunnel_medium_type=6 (802),
// vlan=<id>. Modeled from documented controller behavior; the field set is
// HIL-unverified (this network has no RADIUS users) — see TODO.md.
type RadiusAccount struct {
	ID               string `json:"_id,omitempty"`
	SiteID           string `json:"site_id,omitempty"`
	Name             string `json:"name"`
	XPassword        string `json:"x_password,omitempty"`
	TunnelType       *int   `json:"tunnel_type,omitempty"`
	TunnelMediumType *int   `json:"tunnel_medium_type,omitempty"`
	VLAN             string `json:"vlan,omitempty"`
}

// RADIUS profile CRUD (legacy /rest/radiusprofile). Update is GET-merge-PUT so
// controller-managed fields (attr_no_edit/attr_no_delete) survive.

func (c *client) ListRadiusProfiles(ctx context.Context) ([]RadiusProfile, error) {
	return legacyList[RadiusProfile](ctx, c, radiusProfileType)
}

func (c *client) GetRadiusProfile(ctx context.Context, id string) (*RadiusProfile, error) {
	return legacyGet[RadiusProfile](ctx, c, radiusProfileType, id)
}

func (c *client) CreateRadiusProfile(ctx context.Context, p *RadiusProfile) (*RadiusProfile, error) {
	return legacyCreate[RadiusProfile](ctx, c, radiusProfileType, p)
}

func (c *client) UpdateRadiusProfile(ctx context.Context, p *RadiusProfile) (*RadiusProfile, error) {
	if p.ID == "" {
		return nil, fmt.Errorf("UpdateRadiusProfile: missing id")
	}
	return legacyMergePut[RadiusProfile](ctx, c, radiusProfileType, p.ID, p)
}

func (c *client) DeleteRadiusProfile(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, radiusProfileType, id)
}

// RADIUS account CRUD (legacy /rest/account). Update is GET-merge-PUT.

func (c *client) ListRadiusAccounts(ctx context.Context) ([]RadiusAccount, error) {
	return legacyList[RadiusAccount](ctx, c, accountType)
}

func (c *client) GetRadiusAccount(ctx context.Context, id string) (*RadiusAccount, error) {
	return legacyGet[RadiusAccount](ctx, c, accountType, id)
}

func (c *client) CreateRadiusAccount(ctx context.Context, a *RadiusAccount) (*RadiusAccount, error) {
	return legacyCreate[RadiusAccount](ctx, c, accountType, a)
}

func (c *client) UpdateRadiusAccount(ctx context.Context, a *RadiusAccount) (*RadiusAccount, error) {
	if a.ID == "" {
		return nil, fmt.Errorf("UpdateRadiusAccount: missing id")
	}
	return legacyMergePut[RadiusAccount](ctx, c, accountType, a.ID, a)
}

func (c *client) DeleteRadiusAccount(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, accountType, id)
}
