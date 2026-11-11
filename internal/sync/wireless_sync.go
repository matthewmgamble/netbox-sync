package sync

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/state"
)

// syncWirelessLANsWithData synchronizes wireless LANs (SSIDs) from Mist to NetBox
func (e *Engine) syncWirelessLANsWithData() error {
	logger.Info("Starting wireless LAN synchronization")

	stats := state.ObjectStats{}

	// Fetch organization WLANs (SSID templates)
	wlans, err := e.mistClient.GetOrgWLANs()
	if err != nil {
		return fmt.Errorf("failed to fetch org WLANs: %w", err)
	}
	e.state.RecordAPICall("mist")

	stats.Discovered = len(wlans)
	logger.WithField("count", len(wlans)).Info("Discovered wireless LANs from Mist")

	// Track NetBox VLAN IDs we've already looked up
	vlanCache := make(map[int]int64) // Mist VLAN ID -> NetBox VLAN ID

	// Process each WLAN
	for _, wlan := range wlans {
		// Extract WLAN details
		ssid, _ := wlan["ssid"].(string)
		if ssid == "" {
			logger.Warn("WLAN has no SSID, skipping")
			continue
		}

		enabled, _ := wlan["enabled"].(bool)
		if !enabled {
			logger.WithField("ssid", ssid).Debug("WLAN is disabled, skipping")
			continue
		}

		vlanEnabled, _ := wlan["vlan_enabled"].(bool)
		mistVLANID, hasVLAN := wlan["vlan_id"].(float64)

		// Build description
		description := ""
		authType := ""
		if auth, ok := wlan["auth"].(map[string]interface{}); ok {
			if authTypeVal, ok := auth["type"].(string); ok {
				authType = authTypeVal
				description = fmt.Sprintf("Auth: %s", authType)
			}
		}

		bands, _ := wlan["bands"].([]interface{})
		if len(bands) > 0 {
			bandStr := ""
			for i, band := range bands {
				if i > 0 {
					bandStr += ", "
				}
				bandStr += fmt.Sprintf("%v", band)
			}
			if description != "" {
				description += " | "
			}
			description += fmt.Sprintf("Bands: %s", bandStr)
		}

		isolation, _ := wlan["isolation"].(bool)
		if isolation {
			if description != "" {
				description += " | "
			}
			description += "Isolated"
		}

		// Find NetBox VLAN ID if this WLAN uses VLAN tagging
		var netboxVLANID *int64
		if vlanEnabled && hasVLAN && int(mistVLANID) > 0 {
			// Check cache first
			if cachedID, exists := vlanCache[int(mistVLANID)]; exists {
				netboxVLANID = &cachedID
			} else {
				// Look up VLAN in NetBox by VID
				vlan, err := e.netboxClient.FindVLANByVID(int(mistVLANID))
				if err != nil {
					logger.WithFields(map[string]interface{}{
						"ssid":    ssid,
						"vlan_id": int(mistVLANID),
						"error":   err,
					}).Warn("Failed to find VLAN for wireless LAN")
				} else if vlan != nil {
					vlanID := int64(vlan.GetId())
					vlanCache[int(mistVLANID)] = vlanID
					netboxVLANID = &vlanID
					logger.WithFields(map[string]interface{}{
						"ssid":           ssid,
						"vlan_id":        int(mistVLANID),
						"netbox_vlan_id": vlanID,
					}).Debug("Found VLAN for wireless LAN")
				} else {
					logger.WithFields(map[string]interface{}{
						"ssid":    ssid,
						"vlan_id": int(mistVLANID),
					}).Warn("VLAN not found in NetBox for wireless LAN")
				}
				e.state.RecordAPICall("netbox")
			}
		}

		// Create or find wireless LAN in NetBox
		wlan, err := e.netboxClient.FindOrCreateWirelessLAN(ssid, netboxVLANID, description)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"ssid":  ssid,
				"error": err,
			}).Error("Failed to create wireless LAN")
			stats.Errors++
			e.state.RecordError("wireless_lan", ssid, "create", err)
			continue
		}
		e.state.RecordAPICall("netbox")

		// If VLAN linkage exists and wireless LAN already existed, update it
		if netboxVLANID != nil && wlan != nil {
			wlanID := int64(wlan.GetId())
			// Update with VLAN linkage
			if err := e.netboxClient.UpdateWirelessLAN(wlanID, netboxVLANID); err != nil {
				logger.WithFields(map[string]interface{}{
					"ssid":  ssid,
					"error": err,
				}).Warn("Failed to update wireless LAN with VLAN linkage")
			} else {
				e.state.RecordAPICall("netbox")
				stats.Updated++
			}
		} else {
			stats.Created++
		}

		logger.WithFields(map[string]interface{}{
			"ssid":    ssid,
			"vlan_id": int(mistVLANID),
			"auth":    authType,
		}).Debug("Processed wireless LAN")
	}

	e.state.RecordObjectStats("wireless_lans", stats)
	logger.WithFields(map[string]interface{}{
		"discovered": stats.Discovered,
		"created":    stats.Created,
		"errors":     stats.Errors,
	}).Info("Wireless LAN synchronization complete")

	return nil
}
