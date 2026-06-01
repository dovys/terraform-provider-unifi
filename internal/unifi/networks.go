package unifi

import (
	"context"
	"fmt"

	"github.com/dovys/terraform-provider-unifi/internal/unifi/models"
)

const networkconfType = "networkconf"

// Network CRUD. The networkconf doc backs both unifi_network_network and
// unifi_network_wan; routing is legacy REST for both read and write
// (router OpNetworkRead/OpNetworkWrite). Update is GET-merge-PUT so
// controller-managed fields (attr_hidden_id, networkgroup, …) survive.

func (c *client) ListNetworks(ctx context.Context) ([]models.NetworkConf, error) {
	return legacyList[models.NetworkConf](ctx, c, networkconfType)
}

func (c *client) GetNetwork(ctx context.Context, id string) (*models.NetworkConf, error) {
	return legacyGet[models.NetworkConf](ctx, c, networkconfType, id)
}

func (c *client) CreateNetwork(ctx context.Context, n *models.NetworkConf) (*models.NetworkConf, error) {
	return legacyCreate[models.NetworkConf](ctx, c, networkconfType, n)
}

func (c *client) UpdateNetwork(ctx context.Context, n *models.NetworkConf) (*models.NetworkConf, error) {
	if n.ID == "" {
		return nil, fmt.Errorf("UpdateNetwork: missing id")
	}
	existing, err := legacyGetMerged(ctx, c, networkconfType, n.ID)
	if err != nil {
		return nil, err
	}
	owned, err := structToRawMap(n)
	if err != nil {
		return nil, err
	}
	for k, v := range owned {
		existing[k] = v
	}
	return legacyPut[models.NetworkConf](ctx, c, networkconfType, n.ID, existing)
}

func (c *client) DeleteNetwork(ctx context.Context, id string) error {
	return legacyDelete(ctx, c, networkconfType, id)
}
