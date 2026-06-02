package network

import "github.com/dovys/terraform-provider-unifi/internal/unifi/network/wire"

// TrafficService is the traffic group: traffic rules, traffic routes, and QoS
// rules — all on the v2 surface. (Traffic matching lists join later.)
type TrafficService struct{ core *core }

// Traffic returns the traffic group.
func (c *Client) Traffic() TrafficService { return TrafficService{core: c.core} }

// Rules is the traffic-rule collection (v2 /trafficrules).
func (s TrafficService) Rules() V2Collection[wire.Trafficrules] {
	return V2Collection[wire.Trafficrules]{core: s.core, path: "trafficrules"}
}

// Routes is the traffic-route collection (v2 /trafficroutes).
func (s TrafficService) Routes() V2Collection[wire.Trafficroutes] {
	return V2Collection[wire.Trafficroutes]{core: s.core, path: "trafficroutes"}
}

// QoS is the QoS-rule collection (v2 /qos-rules).
func (s TrafficService) QoS() V2Collection[wire.QosRules] {
	return V2Collection[wire.QosRules]{core: s.core, path: "qos-rules"}
}
