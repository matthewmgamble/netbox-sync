package sync

import (
	"fmt"
	"strings"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	"github.com/matthewmgamble/netbox-sync/internal/state"
)

// syncDeviceTypesWithData synchronizes manufacturers, platforms, roles, and device types using cached data
func (e *Engine) syncDeviceTypesWithData(mistDevices []*models.Device) error {
	logger.Info("Starting device type synchronization")

	// Track unique device types and manufacturers
	manufacturers := make(map[string]*models.Manufacturer)
	deviceTypes := make(map[string]*models.DeviceType)
	roles := make(map[string]*models.DeviceRole)
	platforms := make(map[string]*models.Platform)

	// Collect unique types
	for _, device := range mistDevices {
		// Manufacturer
		if _, exists := manufacturers[device.Manufacturer]; !exists {
			manufacturers[device.Manufacturer] = &models.Manufacturer{
				Name: device.Manufacturer,
				Slug: config.NormalizeSlug(device.Manufacturer),
			}
		}

		// Device Type
		typeKey := fmt.Sprintf("%s-%s", device.Manufacturer, device.Model)
		if _, exists := deviceTypes[typeKey]; !exists {
			deviceTypes[typeKey] = &models.DeviceType{
				Manufacturer: device.Manufacturer,
				Model:        device.Model,
				Slug:         config.NormalizeSlug(device.Model),
				Height:       0.0, // Non-rack devices
				IsFullDepth:  false,
			}
		}

		// Role
		if _, exists := roles[device.Role]; !exists {
			roles[device.Role] = &models.DeviceRole{
				Name: device.Role,
				Slug: config.NormalizeSlug(device.Role),
			}
		}

		// Platform
		if device.Platform != "" {
			if _, exists := platforms[device.Platform]; !exists {
				platforms[device.Platform] = &models.Platform{
					Name:         device.Platform,
					Slug:         config.NormalizeSlug(device.Platform),
					Manufacturer: device.Manufacturer,
				}
			}
		}
	}

	// Create manufacturers
	mfrStats := state.ObjectStats{Discovered: len(manufacturers)}
	for _, mfr := range manufacturers {
		created, err := e.netboxClient.EnsureManufacturer(mfr)
		if err != nil {
			logger.Errorf("Failed to ensure manufacturer %s: %v", mfr.Name, err)
			mfrStats.Errors++
			continue
		}
		e.state.RecordAPICall("netbox")
		if created != nil {
			mfrStats.Created++
		} else {
			mfrStats.Unchanged++
		}
	}
	e.state.RecordObjectStats("manufacturers", mfrStats)

	// Create device roles
	roleStats := state.ObjectStats{Discovered: len(roles)}
	for _, role := range roles {
		created, err := e.netboxClient.EnsureDeviceRole(role)
		if err != nil {
			logger.Errorf("Failed to ensure device role %s: %v", role.Name, err)
			roleStats.Errors++
			continue
		}
		e.state.RecordAPICall("netbox")
		if created != nil {
			roleStats.Created++
		} else {
			roleStats.Unchanged++
		}
	}
	e.state.RecordObjectStats("device_roles", roleStats)

	// Create platforms
	platformStats := state.ObjectStats{Discovered: len(platforms)}
	for _, platform := range platforms {
		created, err := e.netboxClient.EnsurePlatform(platform)
		if err != nil {
			logger.Errorf("Failed to ensure platform %s: %v", platform.Name, err)
			platformStats.Errors++
			continue
		}
		e.state.RecordAPICall("netbox")
		if created != nil {
			platformStats.Created++
		} else {
			platformStats.Unchanged++
		}
	}
	e.state.RecordObjectStats("platforms", platformStats)

	// Create device types
	dtStats := state.ObjectStats{Discovered: len(deviceTypes)}
	for _, dt := range deviceTypes {
		created, err := e.netboxClient.EnsureDeviceType(dt)
		if err != nil {
			logger.Errorf("Failed to ensure device type %s %s: %v", dt.Manufacturer, dt.Model, err)
			dtStats.Errors++
			continue
		}
		e.state.RecordAPICall("netbox")
		if created != nil {
			dtStats.Created++
		} else {
			dtStats.Unchanged++
		}
	}
	e.state.RecordObjectStats("device_types", dtStats)

	logger.WithFields(map[string]interface{}{
		"manufacturers": mfrStats.Created,
		"device_types":  dtStats.Created,
		"roles":         roleStats.Created,
		"platforms":     platformStats.Created,
	}).Info("Device type synchronization complete")

	return nil
}

// syncDevicesWithData synchronizes devices from Mist to NetBox using cached data
func (e *Engine) syncDevicesWithData(mistDevices []*models.Device, mistSites []*models.Site) error {
	logger.Info("Starting device synchronization")

	stats := state.ObjectStats{}

	stats.Discovered = len(mistDevices)
	logger.WithField("count", len(mistDevices)).Info("Discovered devices from Mist")

	// TESTING: Limit devices to match site limit (only process devices from synced sites)
	testLimit := 100 // Reasonable number for testing
	if len(mistDevices) > testLimit {
		logger.Warnf("TESTING MODE: Processing only first %d devices (total: %d)", testLimit, len(mistDevices))
		mistDevices = mistDevices[:testLimit]
	}

	// Build site ID to name mapping
	siteMap := make(map[string]string)
	for _, site := range mistSites {
		siteMap[site.SourceID] = site.Name
	}

	// Process each device
	for _, device := range mistDevices {
		// Apply filters
		if e.shouldSkipDevice(device.Model) {
			logger.WithField("device", device.Name).Debug("Skipping filtered device")
			continue
		}

		// Resolve site name from site ID
		if siteIDStr, ok := device.CustomFields["mist_site_id"].(string); ok {
			if siteName, ok := siteMap[siteIDStr]; ok {
				device.Site = siteName
			}
		}

		// Skip devices without a site assignment
		if device.Site == "" {
			logger.WithField("device", device.Name).Warn("Device has no site assignment, skipping")
			stats.Errors++
			continue
		}

		// Set default platform if not set
		if device.Platform == "" {
			device.Platform = "Juniper Mist"
		}

		// Compute checksum for change detection
		checksum, err := computeChecksum(device)
		if err != nil {
			logger.WithField("device", device.Name).Warnf("Failed to compute checksum: %v", err)
		}

		// Compare: Check if device exists (by serial number for uniqueness)
		// TEMPORARY: Skip FindDevice check since NetBox shows 0 devices but FindDevice returns ID=0
		var existingDevice interface{}
		var skipFind = false // Set to true to bypass FindDevice and always CREATE

		if !skipFind {
			if device.Serial != "" {
				existingDevice, err = e.netboxClient.FindDeviceBySerial(device.Serial)
			} else {
				existingDevice, err = e.netboxClient.FindDeviceByName(device.Name)
			}
		} else {
			logger.WithField("device", device.Name).Debug("Skipping FindDevice check - will attempt CREATE")
			err = nil
		}

		if err != nil {
			logger.WithField("device", device.Name).Warnf("Failed to check for existing device: %v", err)
			e.state.RecordError("device", device.SourceID, "find", err)
			stats.Errors++
			continue
		}
		e.state.RecordAPICall("netbox")

		// DEBUG: Log what FindDevice returned
		if existingDevice != nil {
			netboxDevice := existingDevice.(interface{ GetId() int32 })
			deviceID := netboxDevice.GetId()
			logger.WithFields(map[string]interface{}{
				"device":     device.Name,
				"serial":     device.Serial,
				"netbox_id":  deviceID,
			}).Debug("Found existing device in NetBox")

			// If ID is 0 or negative, treat as not found
			if deviceID <= 0 {
				logger.WithFields(map[string]interface{}{
					"device": device.Name,
					"serial": device.Serial,
					"id":     deviceID,
				}).Warn("Found device has invalid ID, treating as not found")
				existingDevice = nil
			}
		} else {
			logger.WithFields(map[string]interface{}{
				"device": device.Name,
				"serial": device.Serial,
			}).Debug("Device not found in NetBox, will create")
		}

		if existingDevice == nil {
			// Apply: Create new device
			created, err := e.netboxClient.CreateDevice(device, e.tags)
			if err != nil {
				logger.WithField("device", device.Name).Errorf("Failed to create device: %v", err)
				e.state.RecordError("device", device.SourceID, "create", err)
				stats.Errors++
				continue
			}

			e.state.RecordAPICall("netbox")
			deviceID := int64(created.GetId())
			e.state.TrackObject("devices", device.SourceID, deviceID, checksum)
			stats.Created++

			// Create IP addresses and set as primary
			if err := e.syncDeviceIPs(device, deviceID); err != nil {
				logger.WithField("device", device.Name).Warnf("Failed to sync device IPs: %v", err)
			}
		} else {
			// Check if update needed
			if e.state.IsChanged("devices", device.SourceID, checksum) {
				// Apply: Update existing device
				netboxDevice := existingDevice.(interface{ GetId() int32 })
				deviceID := int64(netboxDevice.GetId())
				_, err := e.netboxClient.UpdateDevice(deviceID, device, e.tags)
				if err != nil {
					logger.WithField("device", device.Name).Errorf("Failed to update device: %v", err)
					e.state.RecordError("device", device.SourceID, "update", err)
					stats.Errors++
					continue
				}

				e.state.RecordAPICall("netbox")
				e.state.TrackObject("devices", device.SourceID, deviceID, checksum)
				stats.Updated++

				// Sync IP addresses and set as primary
				if err := e.syncDeviceIPs(device, deviceID); err != nil {
					logger.WithField("device", device.Name).Warnf("Failed to sync device IPs: %v", err)
				}
			} else {
				stats.Unchanged++
				if logger.Get().Level >= 4 { // Debug level
					logger.WithField("device", device.Name).Debug("Device unchanged")
				}
			}
		}
	}

	e.state.RecordObjectStats("devices", stats)
	logger.WithFields(map[string]interface{}{
		"created":   stats.Created,
		"updated":   stats.Updated,
		"unchanged": stats.Unchanged,
		"errors":    stats.Errors,
	}).Info("Device synchronization complete")

	return nil
}

// shouldSkipDevice checks if a device should be skipped based on filters
func (e *Engine) shouldSkipDevice(model string) bool {
	for _, pattern := range e.config.Filter.ExcludeDeviceModels {
		// Simple wildcard matching
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)) {
				return true
			}
		} else if strings.EqualFold(model, pattern) {
			return true
		}
	}
	return false
}

// syncDeviceIPs creates IP address objects and sets them as primary on the device
func (e *Engine) syncDeviceIPs(device *models.Device, deviceID int64) error {
	var primaryIPv4ID *int64
	var primaryIPv6ID *int64

	// Process management IP
	if mgmtIP, ok := device.CustomFields["management_ip"].(string); ok && mgmtIP != "" {
		// Create management interface first
		mgmtIface, err := e.netboxClient.FindOrCreateManagementInterface(deviceID)
		if err != nil {
			return fmt.Errorf("failed to create management interface: %w", err)
		}
		e.state.RecordAPICall("netbox")
		ifaceID := int64(mgmtIface.GetId())

		logger.WithFields(map[string]interface{}{
			"device_id":    deviceID,
			"interface_id": ifaceID,
			"ip":           mgmtIP,
		}).Debug("Step 1: Created management interface")

		// Create IP WITHOUT assignment (we'll assign to interface separately)
		// This allows it to be set as device primary IP
		ipAddr, err := e.netboxClient.FindOrCreateIPAddress(mgmtIP, nil)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"ip":    mgmtIP,
				"error": err,
			}).Error("Failed to create management IP")
			return fmt.Errorf("failed to create management IP: %w", err)
		}
		e.state.RecordAPICall("netbox")
		ipID := int64(ipAddr.GetId())

		logger.WithFields(map[string]interface{}{
			"ip":    mgmtIP,
			"ip_id": ipID,
		}).Debug("Step 2: Got IP address (created or found existing)")

		// Now assign the IP to the management interface
		logger.WithFields(map[string]interface{}{
			"ip_id":        ipID,
			"interface_id": ifaceID,
			"device_id":    deviceID,
		}).Debug("Step 3: About to assign IP to interface")

		if err := e.netboxClient.AssignIPToInterface(ipID, ifaceID); err != nil {
			logger.WithFields(map[string]interface{}{
				"ip":           mgmtIP,
				"ip_id":        ipID,
				"interface_id": ifaceID,
				"error":        err,
			}).Error("Step 3 FAILED: Could not assign IP to interface")
			return fmt.Errorf("failed to assign IP to interface: %w", err)
		}
		e.state.RecordAPICall("netbox")

		logger.WithFields(map[string]interface{}{
			"ip_id":        ipID,
			"interface_id": ifaceID,
		}).Info("Step 3 SUCCESS: IP assigned to interface")

		// Determine if IPv4 or IPv6 for primary IP
		if strings.Contains(mgmtIP, ":") {
			primaryIPv6ID = &ipID
		} else {
			primaryIPv4ID = &ipID
		}
	}

	// Process external IP (create but don't assign to interface or set as primary)
	if extIP, ok := device.CustomFields["external_ip"].(string); ok && extIP != "" {
		_, err := e.netboxClient.FindOrCreateIPAddress(extIP, nil)
		if err != nil {
			logger.WithField("device", device.Name).Warnf("Failed to create external IP: %v", err)
		} else {
			e.state.RecordAPICall("netbox")
		}
	}

	// Set primary IPs on device if we have any
	if primaryIPv4ID != nil || primaryIPv6ID != nil {
		logger.WithFields(map[string]interface{}{
			"device_id":       deviceID,
			"primary_ipv4_id": primaryIPv4ID,
			"primary_ipv6_id": primaryIPv6ID,
		}).Debug("Step 4: About to set device primary IPs")

		if err := e.netboxClient.SetDevicePrimaryIPs(deviceID, primaryIPv4ID, primaryIPv6ID); err != nil {
			return fmt.Errorf("failed to set primary IPs: %w", err)
		}
		e.state.RecordAPICall("netbox")

		logger.WithFields(map[string]interface{}{
			"device_id": deviceID,
		}).Info("Step 4 SUCCESS: Device primary IPs set")
	}

	return nil
}
