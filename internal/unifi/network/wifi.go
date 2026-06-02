package network

import "github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"

// WiFiService is the WiFi group: wireless networks (WLANs), WLAN groups, and AP
// groups. WLANs/WLAN-groups are legacy /rest objects; AP groups are v2.
type WiFiService struct{ core *core }

// WiFi returns the WiFi group.
func (c *Client) WiFi() WiFiService { return WiFiService{core: c.core} }

// WLANs is the wireless-network collection (legacy /rest/wlanconf).
func (s WiFiService) WLANs() LegacyCollection[wire.Wlanconf] {
	return LegacyCollection[wire.Wlanconf]{core: s.core, typ: "wlanconf"}
}

// WLANGroups is the WLAN-group collection (legacy /rest/wlangroup).
func (s WiFiService) WLANGroups() LegacyCollection[wire.Wlangroup] {
	return LegacyCollection[wire.Wlangroup]{core: s.core, typ: "wlangroup"}
}

// APGroups is the AP-group collection (v2 /apgroups).
func (s WiFiService) APGroups() V2Collection[wire.Apgroups] {
	return V2Collection[wire.Apgroups]{core: s.core, path: "apgroups"}
}
