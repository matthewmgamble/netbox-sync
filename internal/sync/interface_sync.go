package sync

import (
	"fmt"
	"strings"

	"github.com/int13consulting/netbox-sync/internal/logger"
	"github.com/int13consulting/netbox-sync/internal/models"
	"github.com/int13consulting/netbox-sync/internal/state"
)

// syncInterfacesWithData synchronizes switch interfaces and AP uplinks based on discovered switch data
func (e *Engine) syncInterfacesWithData(mistSites []*models.Site) error {
	logger.Info("Starting interface and cable synchronization")

	stats := state.ObjectStats{}

	for _, site := range mistSites {
		logger.WithField("site", site.Name).Debug("Processing discovered switches for site")

		// Fetch discovered switches for this site
		switches, err := e.mistClient.GetDiscoveredSwitches(site.SourceID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site":  site.Name,
				"error": err,
			}).Warn("Failed to fetch discovered switches")
			continue
		}
		e.state.RecordAPICall("mist")

		if len(switches) == 0 {
			logger.WithField("site", site.Name).Debug("No discovered switches for this site")
			continue
		}

		logger.WithFields(map[string]interface{}{
			"site":   site.Name,
			"count":  len(switches),
		}).Info("Found discovered switches")

		// Process each discovered switch
		for _, switchData := range switches {
			if err := e.syncDiscoveredSwitch(switchData, &stats); err != nil {
				logger.WithField("error", err).Error("Failed to sync discovered switch")
			}
		}
	}

	e.state.RecordObjectStats("interfaces", stats)
	logger.WithFields(map[string]interface{}{
		"interfaces_created": stats.Created,
		"cables_created":     stats.Updated, // Reusing Updated field for cables
		"errors":             stats.Errors,
	}).Info("Interface and cable synchronization complete")

	return nil
}

// syncDiscoveredSwitch processes a single discovered switch and its connected APs
func (e *Engine) syncDiscoveredSwitch(switchData map[string]interface{}, stats *state.ObjectStats) error {
	// Extract switch hostname
	hostname, ok := switchData["hostname"].(string)
	if !ok || hostname == "" {
		hostname, ok = switchData["system_name"].(string)
		if !ok || hostname == "" {
			logger.Warn("Discovered switch has no hostname, skipping")
			return nil
		}
	}

	logger.WithField("switch", hostname).Debug("Processing discovered switch")

	// Find the switch device in NetBox
	switchDevice, err := e.netboxClient.FindDeviceByName(hostname)
	if err != nil {
		return fmt.Errorf("failed to find switch device %s: %w", hostname, err)
	}
	e.state.RecordAPICall("netbox")

	if switchDevice == nil {
		logger.WithField("switch", hostname).Warn("Switch device not found in NetBox, skipping")
		return nil
	}

	switchDeviceID := int64(switchDevice.GetId())
	logger.WithFields(map[string]interface{}{
		"switch":    hostname,
		"device_id": switchDeviceID,
	}).Debug("Found switch device in NetBox")

	// Process connected APs
	aps, ok := switchData["aps"].([]interface{})
	if !ok || len(aps) == 0 {
		logger.WithField("switch", hostname).Debug("No APs connected to this switch")
		return nil
	}

	logger.WithFields(map[string]interface{}{
		"switch":  hostname,
		"ap_count": len(aps),
	}).Info("Processing connected APs")

	for _, apInterface := range aps {
		apData, ok := apInterface.(map[string]interface{})
		if !ok {
			continue
		}

		if err := e.syncConnectedAP(switchDeviceID, hostname, apData, stats); err != nil {
			logger.WithField("error", err).Error("Failed to sync connected AP")
			stats.Errors++
		}
	}

	return nil
}

// syncConnectedAP creates interfaces and cable for an AP connected to a switch port
func (e *Engine) syncConnectedAP(switchDeviceID int64, switchHostname string, apData map[string]interface{}, stats *state.ObjectStats) error {
	// Extract AP details
	apHostname, _ := apData["hostname"].(string)
	apMAC, _ := apData["mac"].(string)
	portName, _ := apData["port"].(string)

	if portName == "" {
		portName, _ = apData["port_id"].(string)
	}

	if apHostname == "" && apMAC == "" {
		logger.Warn("AP has no hostname or MAC, skipping")
		return nil
	}

	if portName == "" {
		logger.WithField("ap", apHostname).Warn("AP has no port information, skipping")
		return nil
	}

	logger.WithFields(map[string]interface{}{
		"switch": switchHostname,
		"ap":     apHostname,
		"port":   portName,
	}).Debug("Processing AP connection")

	// Create/find switch port interface
	switchPort, err := e.netboxClient.FindOrCreateInterface(switchDeviceID, portName, "1000base-t")
	if err != nil {
		return fmt.Errorf("failed to create switch port %s: %w", portName, err)
	}
	e.state.RecordAPICall("netbox")
	switchPortID := int64(switchPort.GetId())

	// Find AP device in NetBox
	var apDevice interface{ GetId() int32 }
	if apHostname != "" {
		apDevice, err = e.netboxClient.FindDeviceByName(apHostname)
		e.state.RecordAPICall("netbox")
	}

	// Try MAC if hostname lookup failed
	if apDevice == nil && apMAC != "" {
		// Format MAC for NetBox (add colons if needed)
		formattedMAC := formatMAC(apMAC)
		apDevice, err = e.netboxClient.FindDeviceByMAC(formattedMAC)
		e.state.RecordAPICall("netbox")
	}

	if err != nil {
		return fmt.Errorf("failed to find AP device %s: %w", apHostname, err)
	}

	if apDevice == nil {
		logger.WithFields(map[string]interface{}{
			"ap":       apHostname,
			"mac":      apMAC,
		}).Warn("AP device not found in NetBox, skipping cable creation")
		return nil
	}

	apDeviceID := int64(apDevice.GetId())

	// Create/find AP uplink interface
	apUplink, err := e.netboxClient.FindOrCreateInterface(apDeviceID, "eth0", "1000base-t")
	if err != nil {
		return fmt.Errorf("failed to create AP uplink interface: %w", err)
	}
	e.state.RecordAPICall("netbox")
	apUplinkID := int64(apUplink.GetId())

	// Create cable between switch port and AP uplink
	cable, err := e.netboxClient.FindOrCreateCable(switchPortID, apUplinkID)
	if err != nil {
		return fmt.Errorf("failed to create cable: %w", err)
	}
	e.state.RecordAPICall("netbox")

	if cable != nil {
		// New cable was created
		stats.Updated++ // Using Updated field to count cables
		logger.WithFields(map[string]interface{}{
			"switch":      switchHostname,
			"switch_port": portName,
			"ap":          apHostname,
		}).Info("Created cable between switch and AP")
	} else {
		// Cable already existed
		logger.WithFields(map[string]interface{}{
			"switch":      switchHostname,
			"switch_port": portName,
			"ap":          apHostname,
		}).Debug("Cable already exists between switch and AP")
	}

	return nil
}

// formatMAC formats a MAC address with colons (e.g., "aabbccddeeff" -> "aa:bb:cc:dd:ee:ff")
func formatMAC(mac string) string {
	mac = strings.ToLower(mac)
	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, "-", "")

	if len(mac) != 12 {
		return mac
	}

	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		mac[0:2], mac[2:4], mac[4:6], mac[6:8], mac[8:10], mac[10:12])
}
