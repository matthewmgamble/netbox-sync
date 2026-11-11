package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreatePrefix finds or creates an IP prefix in NetBox
func (c *Client) FindOrCreatePrefix(prefix string, vlanID *int64, description string) (*netbox.Prefix, error) {
	c.rateLimit()

	// Check if prefix already exists
	req := c.client.IpamAPI.IpamPrefixesList(c.ctx).Prefix([]string{prefix})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query prefixes: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"prefix": prefix,
				"id":     existing.GetId(),
			}).Debug("Prefix already exists")
		}

		return existing, nil
	}

	// Create prefix
	prefixReq := netbox.NewWritablePrefixRequestWithDefaults()
	prefixReq.SetPrefix(prefix)

	if description != "" {
		prefixReq.SetDescription(description)
	}

	// Link to VLAN if provided
	if vlanID != nil && *vlanID > 0 {
		vlanIDInt32 := int32(*vlanID)
		vlanUnion := netbox.Int32AsInterfaceRequestUntaggedVlan(&vlanIDInt32)
		prefixReq.SetVlan(vlanUnion)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"prefix":  prefix,
			"vlan_id": vlanID,
		}).Debug("Creating prefix")
	}

	created, _, err := c.client.IpamAPI.IpamPrefixesCreate(c.ctx).WritablePrefixRequest(*prefixReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create prefix: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"prefix": prefix,
		"id":     created.GetId(),
	}).Info("Created prefix")

	return created, nil
}

// UpdatePrefixVLAN updates an existing prefix with VLAN linkage
func (c *Client) UpdatePrefixVLAN(prefixID int64, vlanID *int64) error {
	c.rateLimit()

	updateReq := netbox.NewPatchedWritablePrefixRequestWithDefaults()

	// Set VLAN if provided
	if vlanID != nil && *vlanID > 0 {
		vlanIDInt32 := int32(*vlanID)
		vlanUnion := netbox.Int32AsInterfaceRequestUntaggedVlan(&vlanIDInt32)
		updateReq.SetVlan(vlanUnion)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"prefix_id": prefixID,
			"vlan_id":   vlanID,
		}).Debug("Updating prefix VLAN linkage")
	}

	_, _, err := c.client.IpamAPI.IpamPrefixesPartialUpdate(c.ctx, int32(prefixID)).PatchedWritablePrefixRequest(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to update prefix VLAN linkage: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"prefix_id": prefixID,
		"vlan_id":   vlanID,
	}).Info("Updated prefix with VLAN linkage")

	return nil
}

// FindVLANByNetBoxID finds a VLAN by its NetBox ID
func (c *Client) FindVLANByNetBoxID(vlanID int64) (*netbox.VLAN, error) {
	c.rateLimit()

	vlan, _, err := c.client.IpamAPI.IpamVlansRetrieve(c.ctx, int32(vlanID)).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VLAN: %w", err)
	}

	return vlan, nil
}
