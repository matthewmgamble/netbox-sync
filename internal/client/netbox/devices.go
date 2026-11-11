package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindDeviceByName finds a device by its name
func (c *Client) FindDeviceByName(name string) (*netbox.DeviceWithConfigContext, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimDevicesList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// FindDeviceBySerial finds a device by its serial number
func (c *Client) FindDeviceBySerial(serial string) (*netbox.DeviceWithConfigContext, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimDevicesList(c.ctx).Serial([]string{serial})
	result, resp, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}

	count := result.GetCount()

	// DEBUG: Log the query details
	logger.WithFields(map[string]interface{}{
		"serial":        serial,
		"result_count":  count,
		"http_status":   resp.StatusCode,
	}).Debug("FindDeviceBySerial query")

	if count == 0 {
		logger.WithField("serial", serial).Debug("No devices found with this serial")
		return nil, nil
	}

	results := result.GetResults()
	if len(results) == 0 {
		logger.WithField("serial", serial).Warn("Count > 0 but results array is empty!")
		return nil, nil
	}

	device := &results[0]
	return device, nil
}

// CreateDevice creates a new device in NetBox
func (c *Client) CreateDevice(device *models.Device, tags []string) (*netbox.DeviceWithConfigContext, error) {
	c.rateLimit()

	req := netbox.NewWritableDeviceWithConfigContextRequestWithDefaults()
	req.SetName(device.Name)

	// Set device type
	deviceType, err := c.findDeviceTypeByModelAndManufacturer(device.Model, device.Manufacturer)
	if err != nil || deviceType == nil {
		return nil, fmt.Errorf("device type not found for %s %s", device.Manufacturer, device.Model)
	}
	dtID := int32(deviceType.GetId())
	dtUnion := netbox.Int32AsDeviceBayTemplateRequestDeviceType(&dtID)
	req.SetDeviceType(dtUnion)

	// Set device role
	role, err := c.FindDeviceRoleByName(device.Role)
	if err != nil || role == nil {
		return nil, fmt.Errorf("device role not found: %s", device.Role)
	}
	roleID := int32(role.GetId())
	roleUnion := netbox.Int32AsDeviceWithConfigContextRequestRole(&roleID)
	req.SetRole(roleUnion)

	// Set site
	site, err := c.FindSiteByName(device.Site)
	if err != nil || site == nil {
		return nil, fmt.Errorf("site not found: %s", device.Site)
	}
	siteID := int32(site.GetId())
	siteUnion := netbox.Int32AsDeviceWithConfigContextRequestSite(&siteID)
	req.SetSite(siteUnion)

	// Set serial number
	if device.Serial != "" {
		req.SetSerial(device.Serial)
	}

	// Set asset tag
	if device.AssetTag != "" {
		req.SetAssetTag(device.AssetTag)
	}

	// Set description
	if device.Description != "" {
		req.SetDescription(device.Description)
	}

	// Set status
	if device.Status != "" {
		status := c.mapDeviceStatus(device.Status)
		req.SetStatus(status)
	}

	// Set platform
	if device.Platform != "" {
		platform, err := c.FindPlatformByName(device.Platform)
		if err == nil && platform != nil {
			platformID := int32(platform.GetId())
			platformUnion := netbox.Int32AsDeviceTypeRequestDefaultPlatform(&platformID)
			req.SetPlatform(platformUnion)
		}
	}

	// Set tags
	if len(tags) > 0 {
		tagRequests := make([]netbox.NestedTagRequest, len(tags))
		for i, tag := range tags {
			tagRequests[i] = *netbox.NewNestedTagRequest(tag, config.NormalizeSlug(tag))
		}
		req.SetTags(tagRequests)
	}

	// Set custom fields
	if len(device.CustomFields) > 0 {
		req.SetCustomFields(device.CustomFields)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"device": device.Name,
			"serial": device.Serial,
		}).Debug("Creating device")
	}

	result, resp, err := c.client.DcimAPI.DcimDevicesCreate(c.ctx).WritableDeviceWithConfigContextRequest(*req).Execute()
	if err != nil {
		// Extract detailed error information from NetBox response
		errorDetails := err.Error()

		// Log detailed error information
		logFields := map[string]interface{}{
			"device":       device.Name,
			"serial":       device.Serial,
			"site":         device.Site,
			"device_type":  device.Model,
			"manufacturer": device.Manufacturer,
			"role":         device.Role,
			"status":       device.Status,
			"platform":     device.Platform,
			"full_error":   errorDetails,
		}

		if resp != nil {
			logFields["http_status"] = resp.StatusCode
		}

		logger.WithFields(logFields).Error("Device creation failed - NetBox returned error")

		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"device": device.Name,
		"serial": device.Serial,
	}).Info("Created device")

	return result, nil
}

// UpdateDevice updates an existing device in NetBox
func (c *Client) UpdateDevice(deviceID int64, device *models.Device, tags []string) (*netbox.DeviceWithConfigContext, error) {
	c.rateLimit()

	req := netbox.NewPatchedWritableDeviceWithConfigContextRequestWithDefaults()

	// Update description
	if device.Description != "" {
		req.SetDescription(device.Description)
	}

	// Update asset tag
	if device.AssetTag != "" {
		req.SetAssetTag(device.AssetTag)
	}

	// Update status
	if device.Status != "" {
		status := c.mapDeviceStatus(device.Status)
		req.SetStatus(status)
	}

	// Update platform
	if device.Platform != "" {
		platform, err := c.FindPlatformByName(device.Platform)
		if err == nil && platform != nil {
			platformID := int32(platform.GetId())
			platformUnion := netbox.Int32AsDeviceTypeRequestDefaultPlatform(&platformID)
			req.SetPlatform(platformUnion)
		}
	}

	// Update site (if device moved)
	if device.Site != "" {
		site, err := c.FindSiteByName(device.Site)
		if err == nil && site != nil {
			siteID := int32(site.GetId())
			siteUnion := netbox.Int32AsDeviceWithConfigContextRequestSite(&siteID)
			req.SetSite(siteUnion)
		}
	}

	// Update role (if changed)
	if device.Role != "" {
		role, err := c.FindDeviceRoleByName(device.Role)
		if err == nil && role != nil {
			roleID := int32(role.GetId())
			roleUnion := netbox.Int32AsDeviceWithConfigContextRequestRole(&roleID)
			req.SetRole(roleUnion)
		}
	}

	// Update tags
	if len(tags) > 0 {
		tagRequests := make([]netbox.NestedTagRequest, len(tags))
		for i, tag := range tags {
			tagRequests[i] = *netbox.NewNestedTagRequest(tag, config.NormalizeSlug(tag))
		}
		req.SetTags(tagRequests)
	}

	// Update custom fields
	if len(device.CustomFields) > 0 {
		req.SetCustomFields(device.CustomFields)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"device": device.Name,
			"serial": device.Serial,
		}).Debug("Updating device")
	}

	result, _, err := c.client.DcimAPI.DcimDevicesPartialUpdate(c.ctx, int32(deviceID)).PatchedWritableDeviceWithConfigContextRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to update device: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"device": device.Name,
		"serial": device.Serial,
	}).Info("Updated device")

	return result, nil
}

// mapDeviceStatus maps our status to NetBox device status value
func (c *Client) mapDeviceStatus(status string) netbox.DeviceStatusValue {
	// NetBox device statuses: offline, active, planned, staged, failed, inventory, decommissioning
	statusMap := map[string]netbox.DeviceStatusValue{
		"offline":         netbox.DEVICESTATUSVALUE_OFFLINE,
		"active":          netbox.DEVICESTATUSVALUE_ACTIVE,
		"planned":         netbox.DEVICESTATUSVALUE_PLANNED,
		"staged":          netbox.DEVICESTATUSVALUE_STAGED,
		"failed":          netbox.DEVICESTATUSVALUE_FAILED,
		"inventory":       netbox.DEVICESTATUSVALUE_INVENTORY,
		"decommissioning": netbox.DEVICESTATUSVALUE_DECOMMISSIONING,
	}

	if val, ok := statusMap[status]; ok {
		return val
	}

	// Default to active
	return netbox.DEVICESTATUSVALUE_ACTIVE
}

// FindDeviceByMAC finds a device by its MAC address
// Note: NetBox doesn't have a direct MAC search, so this is a placeholder
// In practice, we'll rely on hostname matching for APs
func (c *Client) FindDeviceByMAC(mac string) (*netbox.DeviceWithConfigContext, error) {
	// TODO: Implement MAC search if needed
	// For now, return nil to fall back to hostname search
	return nil, nil
}
