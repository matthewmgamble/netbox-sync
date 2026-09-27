package netbox

import (
	"fmt"

	"github.com/int13consulting/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreateWirelessLAN finds or creates a wireless LAN (SSID) in NetBox
func (c *Client) FindOrCreateWirelessLAN(ssid string, vlanID *int64, description string) (*netbox.WirelessLAN, error) {
	c.rateLimit()

	// Check if wireless LAN already exists by SSID
	req := c.client.WirelessAPI.WirelessWirelessLansList(c.ctx).Ssid([]string{ssid})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query wireless LANs: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"ssid": ssid,
				"id":   existing.GetId(),
			}).Debug("Wireless LAN already exists")
		}

		return existing, nil
	}

	// Create wireless LAN
	wlanReq := netbox.NewWritableWirelessLANRequestWithDefaults()
	wlanReq.SetSsid(ssid)

	if description != "" {
		wlanReq.SetDescription(description)
	}

	// Link to VLAN if provided
	if vlanID != nil && *vlanID > 0 {
		vlanIDInt32 := int32(*vlanID)
		vlanUnion := netbox.Int32AsInterfaceRequestUntaggedVlan(&vlanIDInt32)
		wlanReq.SetVlan(vlanUnion)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"ssid":    ssid,
			"vlan_id": vlanID,
		}).Debug("Creating wireless LAN")
	}

	created, _, err := c.client.WirelessAPI.WirelessWirelessLansCreate(c.ctx).WritableWirelessLANRequest(*wlanReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create wireless LAN: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"ssid": ssid,
		"id":   created.GetId(),
	}).Info("Created wireless LAN")

	return created, nil
}

// UpdateWirelessLAN updates an existing wireless LAN with VLAN linkage
func (c *Client) UpdateWirelessLAN(wlanID int64, vlanID *int64) error {
	c.rateLimit()

	updateReq := netbox.NewPatchedWritableWirelessLANRequestWithDefaults()

	// Set VLAN if provided
	if vlanID != nil && *vlanID > 0 {
		vlanIDInt32 := int32(*vlanID)
		vlanUnion := netbox.Int32AsInterfaceRequestUntaggedVlan(&vlanIDInt32)
		updateReq.SetVlan(vlanUnion)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"wlan_id": wlanID,
			"vlan_id": vlanID,
		}).Debug("Updating wireless LAN")
	}

	_, _, err := c.client.WirelessAPI.WirelessWirelessLansPartialUpdate(c.ctx, int32(wlanID)).PatchedWritableWirelessLANRequest(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to update wireless LAN: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"wlan_id": wlanID,
		"vlan_id": vlanID,
	}).Info("Updated wireless LAN with VLAN linkage")

	return nil
}

