package unifi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const apGroupsPath = "apgroups"

// APGroup is a v2 AP group (/v2/api/site/<site>/apgroups): a named set of access
// points a WLAN can broadcast on. The controller ships a default "All APs" group
// (attr_no_delete) covering every adopted AP. The v2 surface is bare JSON and
// honors the v2 write contract (_id in URL and body on PUT; architecture.md §4).
type APGroup struct {
	ID           string   `json:"_id,omitempty"`
	SiteID       string   `json:"site_id,omitempty"`
	Name         string   `json:"name"`                     // display name
	DeviceMACs   []string `json:"device_macs"`              // APs in the group (MAC addresses)
	ForWLANConf  *bool    `json:"for_wlanconf,omitempty"`   // selectable when assigning WLANs
	AttrNoDelete *bool    `json:"attr_no_delete,omitempty"` // controller-protected (the default group)
	AttrHiddenID string   `json:"attr_hidden_id,omitempty"` // controller-internal stable key (e.g. "default")
}

// AP group CRUD (v2 /apgroups). Like the v2 static-DNS surface: bare JSON,
// list-and-filter for GET-by-id, _id in URL+body on PUT.

func (c *client) ListAPGroups(ctx context.Context) ([]APGroup, error) {
	var out []APGroup
	if err := v2Decode(ctx, c, http.MethodGet, apGroupsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) GetAPGroup(ctx context.Context, id string) (*APGroup, error) {
	groups, err := c.ListAPGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == id {
			return &groups[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *client) CreateAPGroup(ctx context.Context, g *APGroup) (*APGroup, error) {
	var out APGroup
	if err := v2Decode(ctx, c, http.MethodPost, apGroupsPath, g, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) UpdateAPGroup(ctx context.Context, g *APGroup) (*APGroup, error) {
	if g.ID == "" {
		return nil, fmt.Errorf("UpdateAPGroup: missing id")
	}
	// v2 write contract: _id in both URL and body.
	var out APGroup
	if err := v2Decode(ctx, c, http.MethodPut, apGroupsPath+"/"+g.ID, g, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) DeleteAPGroup(ctx context.Context, id string) error {
	err := v2Decode[any](ctx, c, http.MethodDelete, apGroupsPath+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
