package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreateVLAN finds or creates a VLAN in NetBox
func (c *Client) FindOrCreateVLAN(vlanID int, name string, description string) (*netbox.VLAN, error) {
	c.rateLimit()

	// Check if VLAN already exists (by VID and name)
	req := c.client.IpamAPI.IpamVlansList(c.ctx).Vid([]int32{int32(vlanID)}).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query VLANs: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"vid":  vlanID,
				"name": name,
				"id":   existing.GetId(),
			}).Debug("VLAN already exists")
		}

		return existing, nil
	}

	// Create VLAN
	vlanReq := netbox.NewWritableVLANRequestWithDefaults()
	vlanReq.SetVid(int32(vlanID))
	vlanReq.SetName(name)

	if description != "" {
		vlanReq.SetDescription(description)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"vid":  vlanID,
			"name": name,
		}).Debug("Creating VLAN")
	}

	created, _, err := c.client.IpamAPI.IpamVlansCreate(c.ctx).WritableVLANRequest(*vlanReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create VLAN: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"vid":  vlanID,
		"name": name,
		"id":   created.GetId(),
	}).Info("Created VLAN")

	return created, nil
}

// FindVLANByVID finds a VLAN by its VLAN ID (VID)
func (c *Client) FindVLANByVID(vid int) (*netbox.VLAN, error) {
	c.rateLimit()

	req := c.client.IpamAPI.IpamVlansList(c.ctx).Vid([]int32{int32(vid)})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query VLANs: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

