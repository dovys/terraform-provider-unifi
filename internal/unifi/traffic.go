package unifi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	trafficRulesPath  = "trafficrules"
	trafficRoutesPath = "trafficroutes"
)

// TrafficTargetDevice scopes a traffic rule/route to who it applies to: every
// client, a specific client (by MAC), or a whole network.
type TrafficTargetDevice struct {
	Type      string `json:"type,omitempty"`       // ALL_CLIENTS | CLIENT | NETWORK
	ClientMAC string `json:"client_mac,omitempty"` // when type=CLIENT
	NetworkID string `json:"network_id,omitempty"` // when type=NETWORK
}

// TrafficBandwidthLimit is the rate cap applied by a SPEED_LIMIT traffic rule.
type TrafficBandwidthLimit struct {
	Enabled           *bool `json:"enabled,omitempty"`
	DownloadLimitKbps *int  `json:"download_limit_kbps,omitempty"`
	UploadLimitKbps   *int  `json:"upload_limit_kbps,omitempty"`
}

// TrafficRule is a v2 traffic-management rule (/v2/api/site/<site>/trafficrules):
// block/allow/speed-limit traffic matched by destination (internet, domains, IPs,
// regions, apps, app categories) for a set of target devices. Bare JSON, v2 write
// contract (_id in URL+body on PUT). Modeled from documented behavior — the field
// set is HIL-unverified (this network has no traffic rules) — see TODO.md.
type TrafficRule struct {
	ID             string                 `json:"_id,omitempty"`
	Enabled        *bool                  `json:"enabled,omitempty"`
	Action         string                 `json:"action,omitempty"`          // BLOCK | ALLOW | SPEED_LIMIT
	MatchingTarget string                 `json:"matching_target,omitempty"` // INTERNET | DOMAIN | IP | REGION | APP | APP_CATEGORY
	Description    string                 `json:"description,omitempty"`
	Domains        []string               `json:"domains,omitempty"`
	IPAddresses    []string               `json:"ip_addresses,omitempty"`
	IPRanges       []string               `json:"ip_ranges,omitempty"`
	Regions        []string               `json:"regions,omitempty"`
	AppIDs         []int                  `json:"app_ids,omitempty"`
	AppCategoryIDs []int                  `json:"app_category_ids,omitempty"`
	NetworkIDs     []string               `json:"network_ids,omitempty"`
	TargetDevices  []TrafficTargetDevice  `json:"target_devices,omitempty"`
	BandwidthLimit *TrafficBandwidthLimit `json:"bandwidth_limit,omitempty"`
}

// TrafficRoute is a v2 policy route (/v2/api/site/<site>/trafficroutes): send
// traffic matched by destination out a specific interface/VPN (network_id) for a
// set of target devices, optionally with a kill switch. Bare JSON, v2 write
// contract. Modeled from documented behavior — HIL-unverified (no live routes) —
// see TODO.md.
type TrafficRoute struct {
	ID                string                `json:"_id,omitempty"`
	Enabled           *bool                 `json:"enabled,omitempty"`
	Description       string                `json:"description,omitempty"`
	MatchingTarget    string                `json:"matching_target,omitempty"` // DOMAIN | IP | REGION | INTERNET
	Domains           []string              `json:"domains,omitempty"`
	IPAddresses       []string              `json:"ip_addresses,omitempty"`
	IPRanges          []string              `json:"ip_ranges,omitempty"`
	Regions           []string              `json:"regions,omitempty"`
	NetworkID         string                `json:"network_id,omitempty"` // egress interface / VPN the matched traffic routes through
	TargetDevices     []TrafficTargetDevice `json:"target_devices,omitempty"`
	KillSwitchEnabled *bool                 `json:"kill_switch_enabled,omitempty"`
}

// Traffic rule CRUD (v2 /trafficrules). Bare JSON, list-and-filter for GET-by-id,
// _id in URL+body on PUT (the v2 write contract).

func (c *client) ListTrafficRules(ctx context.Context) ([]TrafficRule, error) {
	var out []TrafficRule
	if err := v2Decode(ctx, c, http.MethodGet, trafficRulesPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) GetTrafficRule(ctx context.Context, id string) (*TrafficRule, error) {
	rules, err := c.ListTrafficRules(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *client) CreateTrafficRule(ctx context.Context, r *TrafficRule) (*TrafficRule, error) {
	var out TrafficRule
	if err := v2Decode(ctx, c, http.MethodPost, trafficRulesPath, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) UpdateTrafficRule(ctx context.Context, r *TrafficRule) (*TrafficRule, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateTrafficRule: missing id")
	}
	var out TrafficRule
	if err := v2Decode(ctx, c, http.MethodPut, trafficRulesPath+"/"+r.ID, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) DeleteTrafficRule(ctx context.Context, id string) error {
	err := v2Decode[any](ctx, c, http.MethodDelete, trafficRulesPath+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Traffic route CRUD (v2 /trafficroutes). Same v2 contract as traffic rules.

func (c *client) ListTrafficRoutes(ctx context.Context) ([]TrafficRoute, error) {
	var out []TrafficRoute
	if err := v2Decode(ctx, c, http.MethodGet, trafficRoutesPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) GetTrafficRoute(ctx context.Context, id string) (*TrafficRoute, error) {
	routes, err := c.ListTrafficRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].ID == id {
			return &routes[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *client) CreateTrafficRoute(ctx context.Context, r *TrafficRoute) (*TrafficRoute, error) {
	var out TrafficRoute
	if err := v2Decode(ctx, c, http.MethodPost, trafficRoutesPath, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) UpdateTrafficRoute(ctx context.Context, r *TrafficRoute) (*TrafficRoute, error) {
	if r.ID == "" {
		return nil, fmt.Errorf("UpdateTrafficRoute: missing id")
	}
	var out TrafficRoute
	if err := v2Decode(ctx, c, http.MethodPut, trafficRoutesPath+"/"+r.ID, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) DeleteTrafficRoute(ctx context.Context, id string) error {
	err := v2Decode[any](ctx, c, http.MethodDelete, trafficRoutesPath+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
