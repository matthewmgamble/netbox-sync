package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreateManagementInterface finds or creates a management interface on a device
func (c *Client) FindOrCreateManagementInterface(deviceID int64) (*netbox.Interface, error) {
	c.rateLimit()

	interfaceName := "Management"

	// Check if management interface already exists
	req := c.client.DcimAPI.DcimInterfacesList(c.ctx).DeviceId([]int32{int32(deviceID)}).Name([]string{interfaceName})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query interfaces: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"name":      interfaceName,
				"id":        existing.GetId(),
			}).Debug("Management interface already exists")
		}

		return existing, nil
	}

	// Create management interface
	ifaceReq := netbox.NewWritableInterfaceRequestWithDefaults()
	ifaceReq.SetName(interfaceName)
	ifaceReq.SetType(netbox.INTERFACETYPEVALUE_VIRTUAL)

	// Assign to device
	deviceIDInt32 := int32(deviceID)
	deviceUnion := netbox.Int32AsBriefInterfaceRequestDevice(&deviceIDInt32)
	ifaceReq.SetDevice(deviceUnion)

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"device_id": deviceID,
			"name":      interfaceName,
		}).Debug("Creating management interface")
	}

	created, _, err := c.client.DcimAPI.DcimInterfacesCreate(c.ctx).WritableInterfaceRequest(*ifaceReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"device_id": deviceID,
		"name":      interfaceName,
		"id":        created.GetId(),
	}).Info("Created management interface")

	return created, nil
}

// FindOrCreateInterface finds or creates a physical interface on a device
func (c *Client) FindOrCreateInterface(deviceID int64, name string, interfaceType string) (*netbox.Interface, error) {
	c.rateLimit()

	// Check if interface already exists
	req := c.client.DcimAPI.DcimInterfacesList(c.ctx).DeviceId([]int32{int32(deviceID)}).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query interfaces: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"name":      name,
				"id":        existing.GetId(),
			}).Debug("Interface already exists")
		}

		return existing, nil
	}

	// Create interface
	ifaceReq := netbox.NewWritableInterfaceRequestWithDefaults()
	ifaceReq.SetName(name)

	// Set interface type
	ifaceReq.SetType(netbox.InterfaceTypeValue(interfaceType))

	// Assign to device
	deviceIDInt32 := int32(deviceID)
	deviceUnion := netbox.Int32AsBriefInterfaceRequestDevice(&deviceIDInt32)
	ifaceReq.SetDevice(deviceUnion)

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"device_id": deviceID,
			"name":      name,
			"type":      interfaceType,
		}).Debug("Creating interface")
	}

	created, _, err := c.client.DcimAPI.DcimInterfacesCreate(c.ctx).WritableInterfaceRequest(*ifaceReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"device_id": deviceID,
		"name":      name,
		"id":        created.GetId(),
	}).Info("Created interface")

	return created, nil
}

// UpdateInterfaceCustomFields updates custom fields on an interface
func (c *Client) UpdateInterfaceCustomFields(interfaceID int64, customFields map[string]interface{}) error {
	c.rateLimit()

	updateReq := netbox.NewPatchedWritableInterfaceRequestWithDefaults()
	updateReq.SetCustomFields(customFields)

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"interface_id":   interfaceID,
			"custom_fields": customFields,
		}).Debug("Updating interface custom fields")
	}

	_, _, err := c.client.DcimAPI.DcimInterfacesPartialUpdate(c.ctx, int32(interfaceID)).PatchedWritableInterfaceRequest(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to update interface custom fields: %w", err)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"interface_id": interfaceID,
		}).Debug("Updated interface custom fields")
	}

	return nil
}

// AssignVLANToInterface assigns a VLAN to an interface in access mode
func (c *Client) AssignVLANToInterface(interfaceID int64, vlanID int64, mode string) error {
	c.rateLimit()

	updateReq := netbox.NewPatchedWritableInterfaceRequestWithDefaults()

	// Set mode (access or tagged)
	if mode == "access" {
		updateReq.SetMode(netbox.PATCHEDWRITABLEINTERFACEREQUESTMODE_ACCESS)
		// Set untagged VLAN
		vlanIDInt32 := int32(vlanID)
		vlanUnion := netbox.Int32AsInterfaceRequestUntaggedVlan(&vlanIDInt32)
		updateReq.SetUntaggedVlan(vlanUnion)
	} else if mode == "tagged" || mode == "trunk" {
		updateReq.SetMode(netbox.PATCHEDWRITABLEINTERFACEREQUESTMODE_TAGGED)
		// Add to tagged VLANs
		vlanIDInt32 := int32(vlanID)
		updateReq.SetTaggedVlans([]int32{vlanIDInt32})
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"interface_id": interfaceID,
			"vlan_id":      vlanID,
			"mode":         mode,
		}).Debug("Assigning VLAN to interface")
	}

	_, _, err := c.client.DcimAPI.DcimInterfacesPartialUpdate(c.ctx, int32(interfaceID)).PatchedWritableInterfaceRequest(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to assign VLAN to interface: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"interface_id": interfaceID,
		"vlan_id":      vlanID,
		"mode":         mode,
	}).Info("Assigned VLAN to interface")

	return nil
}

// AssignMultipleVLANsToInterface assigns multiple VLANs to an interface in trunk/tagged mode
func (c *Client) AssignMultipleVLANsToInterface(interfaceID int64, vlanIDs []int64) error {
	c.rateLimit()

	if len(vlanIDs) == 0 {
		return nil
	}

	updateReq := netbox.NewPatchedWritableInterfaceRequestWithDefaults()

	// Set mode to tagged (trunk)
	updateReq.SetMode(netbox.PATCHEDWRITABLEINTERFACEREQUESTMODE_TAGGED)

	// Convert VLAN IDs to int32
	taggedVlans := make([]int32, len(vlanIDs))
	for i, vlanID := range vlanIDs {
		taggedVlans[i] = int32(vlanID)
	}
	updateReq.SetTaggedVlans(taggedVlans)

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"interface_id": interfaceID,
			"vlan_count":   len(vlanIDs),
		}).Debug("Assigning multiple VLANs to interface")
	}

	_, _, err := c.client.DcimAPI.DcimInterfacesPartialUpdate(c.ctx, int32(interfaceID)).PatchedWritableInterfaceRequest(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to assign VLANs to interface: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"interface_id": interfaceID,
		"vlan_count":   len(vlanIDs),
	}).Info("Assigned VLANs to interface in trunk mode")

	return nil
}


