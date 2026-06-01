package unifi

import (
	"context"
	"fmt"
	"strings"
)

const userType = "user"

// User is a legacy `/rest/user` document — the controller's record of a network
// client, keyed by MAC. This is the one client-level object the provider touches
// (invariant: model infrastructure, not clients), and only for DHCP fixed-IP
// reservations: a reservation is find-or-create by MAC (the controller may
// already track the client from a DHCP lease). Only the reservation-relevant
// fields are modeled; telemetry (last_seen, oui, fingerprint, …) and other
// client config are carried through untouched by GET-merge-PUT.
type User struct {
	ID         string `json:"_id,omitempty"`         // controller object ID
	SiteID     string `json:"site_id,omitempty"`     // owning site (controller-assigned)
	MAC        string `json:"mac,omitempty"`         // client MAC (the natural key); omitempty so a partial revert PUT can't clobber it
	Name       string `json:"name,omitempty"`        // friendly alias
	UseFixedIP *bool  `json:"use_fixedip,omitempty"` // whether a DHCP fixed IP is assigned
	FixedIP    string `json:"fixed_ip,omitempty"`    // the reserved address (within network_id's subnet)
	NetworkID  string `json:"network_id,omitempty"`  // network whose DHCP scope holds the reservation

	// Local DNS A record companion (an optional name → fixed-IP record).
	LocalDNSRecord        string `json:"local_dns_record,omitempty"`
	LocalDNSRecordEnabled *bool  `json:"local_dns_record_enabled,omitempty"`

	Hostname string `json:"hostname,omitempty"` // DHCP-reported hostname (read-only)
}

// User CRUD (legacy /rest/user; router OpDHCPReservation → classic backend).
// Update is GET-merge-PUT so controller-managed client fields survive.

func (c *client) ListUsers(ctx context.Context) ([]User, error) {
	return legacyList[User](ctx, c, userType)
}

func (c *client) GetUser(ctx context.Context, id string) (*User, error) {
	return legacyGet[User](ctx, c, userType, id)
}

// GetUserByMAC finds a client record by MAC, returning ErrNotFound if the
// controller has never seen the device.
func (c *client) GetUserByMAC(ctx context.Context, mac string) (*User, error) {
	users, err := c.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(mac)
	for i := range users {
		if strings.ToLower(users[i].MAC) == want {
			return &users[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *client) CreateUser(ctx context.Context, u *User) (*User, error) {
	return legacyCreate[User](ctx, c, userType, u)
}

func (c *client) UpdateUser(ctx context.Context, u *User) (*User, error) {
	if u.ID == "" {
		return nil, fmt.Errorf("UpdateUser: missing id")
	}
	return legacyMergePut[User](ctx, c, userType, u.ID, u)
}
