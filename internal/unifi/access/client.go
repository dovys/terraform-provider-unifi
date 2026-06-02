package access

// Client is the ergonomic entry point for the UniFi Access application. It is
// constructed by the console unifi.Client (c.Access()) with the one shared
// classic session, and hands out groups. Access is console-global, so unlike
// the Network client there is no per-site scoping.
type Client struct {
	core *core
}

// New wraps an authenticated classic session as an Access client. Callers go
// through the console client's Access() accessor rather than calling this
// directly.
func New(session doer) *Client {
	return &Client{core: &core{session: session}}
}

// DoorGroups manages Access door groups (logical groupings of doors/floors used
// to scope access policies).
func (c *Client) DoorGroups() *DoorGroupService {
	return &DoorGroupService{core: c.core}
}
