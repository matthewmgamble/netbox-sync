package netbox

import (
	"fmt"

	"github.com/int13consulting/netbox-sync/internal/logger"
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
