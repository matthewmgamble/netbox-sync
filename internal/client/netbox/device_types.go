package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureDeviceType ensures a device type exists in NetBox
func (c *Client) EnsureDeviceType(dt *models.DeviceType) (*netbox.DeviceType, error) {
	c.rateLimit()

	// Check if device type exists
	existing, err := c.findDeviceTypeByModelAndManufacturer(dt.Model, dt.Manufacturer)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"model":        dt.Model,
				"manufacturer": dt.Manufacturer,
			}).Debug("Device type already exists")
		}
		return existing, nil
	}

	// Create device type
	return c.createDeviceType(dt)
}

// findDeviceTypeByModelAndManufacturer finds a device type by model and manufacturer
func (c *Client) findDeviceTypeByModelAndManufacturer(model, manufacturerName string) (*netbox.DeviceType, error) {
	c.rateLimit()

	// First get the manufacturer
	mfr, err := c.FindManufacturerByName(manufacturerName)
	if err != nil {
		return nil, err
	}
	if mfr == nil {
		return nil, nil
	}

	// Search for device type
	req := c.client.DcimAPI.DcimDeviceTypesList(c.ctx).
		Model([]string{model}).
		ManufacturerId([]int32{mfr.GetId()})

	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query device types: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// createDeviceType creates a new device type
func (c *Client) createDeviceType(dt *models.DeviceType) (*netbox.DeviceType, error) {
	c.rateLimit()

	// Get manufacturer
	mfr, err := c.FindManufacturerByName(dt.Manufacturer)
	if err != nil || mfr == nil {
		return nil, fmt.Errorf("manufacturer not found: %s", dt.Manufacturer)
	}

	req := netbox.NewWritableDeviceTypeRequestWithDefaults()
	req.SetModel(dt.Model)
	req.SetSlug(dt.Slug)

	// Create manufacturer reference as union type
	mfrID := int32(mfr.GetId())
	mfrUnion := netbox.Int32AsBriefDeviceTypeRequestManufacturer(&mfrID)
	req.SetManufacturer(mfrUnion)

	if dt.PartNumber != "" {
		req.SetPartNumber(dt.PartNumber)
	}

	if dt.Description != "" {
		req.SetDescription(dt.Description)
	}

	// Set U height (0 for non-rack devices like APs)
	if dt.Height > 0 {
		req.SetUHeight(dt.Height)
		req.SetIsFullDepth(dt.IsFullDepth)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"model":        dt.Model,
			"manufacturer": dt.Manufacturer,
		}).Debug("Creating device type")
	}

	result, _, err := c.client.DcimAPI.DcimDeviceTypesCreate(c.ctx).WritableDeviceTypeRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create device type: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"model":        dt.Model,
		"manufacturer": dt.Manufacturer,
	}).Info("Created device type")

	return result, nil
}
