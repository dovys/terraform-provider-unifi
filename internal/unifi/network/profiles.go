package network

import "github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"

// RADIUSService is the RADIUS group: server profiles and user accounts (both
// legacy /rest objects).
type RADIUSService struct{ core *core }

// RADIUS returns the RADIUS group.
func (c *Client) RADIUS() RADIUSService { return RADIUSService{core: c.core} }

// Profiles is the RADIUS server-profile collection (legacy /rest/radiusprofile).
func (s RADIUSService) Profiles() LegacyCollection[wire.Radiusprofile] {
	return LegacyCollection[wire.Radiusprofile]{core: s.core, typ: "radiusprofile"}
}

// Accounts is the RADIUS user-account collection (legacy /rest/account).
func (s RADIUSService) Accounts() LegacyCollection[wire.Account] {
	return LegacyCollection[wire.Account]{core: s.core, typ: "account"}
}

// PortProfiles is the switch-port profile collection (legacy /rest/portconf).
func (c *Client) PortProfiles() LegacyCollection[wire.Portconf] {
	return LegacyCollection[wire.Portconf]{core: c.core, typ: "portconf"}
}

// UserGroups is the client user-group (bandwidth limit) collection
// (legacy /rest/usergroup).
func (c *Client) UserGroups() LegacyCollection[wire.Usergroup] {
	return LegacyCollection[wire.Usergroup]{core: c.core, typ: "usergroup"}
}

// DynamicDNS is the dynamic-DNS service collection (legacy /rest/dynamicdns).
func (c *Client) DynamicDNS() LegacyCollection[wire.Dynamicdns] {
	return LegacyCollection[wire.Dynamicdns]{core: c.core, typ: "dynamicdns"}
}

// DPIApps is the DPI application-override collection (legacy /rest/dpiapp).
func (c *Client) DPIApps() LegacyCollection[wire.Dpiapp] {
	return LegacyCollection[wire.Dpiapp]{core: c.core, typ: "dpiapp"}
}

// DPIGroups is the DPI group collection (legacy /rest/dpigroup).
func (c *Client) DPIGroups() LegacyCollection[wire.Dpigroup] {
	return LegacyCollection[wire.Dpigroup]{core: c.core, typ: "dpigroup"}
}
