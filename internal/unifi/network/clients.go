package network

import (
	"context"
	"strings"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"
)

// ClientsService is the clients group: known client records (legacy /rest/user).
// This is the provider's one client-device touchpoint — it backs DHCP fixed-IP
// reservations (find-or-create by MAC, use_fixedip). The provider does not model
// clients as managed devices beyond this.
type ClientsService struct{ core *core }

// Clients returns the clients group.
func (c *Client) Clients() ClientsService { return ClientsService{core: c.core} }

// Users is the client-record collection (legacy /rest/user).
func (s ClientsService) Users() LegacyCollection[wire.User] {
	return LegacyCollection[wire.User]{core: s.core, typ: "user"}
}

// GetByMAC returns the client record with the given MAC (case-insensitive), or
// ErrNotFound — the lookup a DHCP reservation does before find-or-create.
func (s ClientsService) GetByMAC(ctx context.Context, mac string) (*wire.User, error) {
	want := strings.ToLower(strings.TrimSpace(mac))
	users, err := s.Users().List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].Mac != nil && strings.ToLower(*users[i].Mac) == want {
			return &users[i], nil
		}
	}
	return nil, ErrNotFound
}
