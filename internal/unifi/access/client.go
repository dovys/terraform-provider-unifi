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

// AccessPolicies manages Access policies (which subjects may use which resources,
// on what schedule).
func (c *Client) AccessPolicies() *AccessPolicyService {
	return &AccessPolicyService{core: c.core}
}

// HolidayGroups manages Access holiday groups (named sets of holidays used to
// scope unlock schedules).
func (c *Client) HolidayGroups() *HolidayGroupService {
	return &HolidayGroupService{core: c.core}
}

// Doors configures discovered Access doors (update-only — doors are not created
// through the API).
func (c *Client) Doors() *DoorService {
	return &DoorService{core: c.core}
}

// DoorGuards manages the per-door guard user set (update-only, keyed by door id).
func (c *Client) DoorGuards() *DoorGuardService {
	return &DoorGuardService{core: c.core}
}
