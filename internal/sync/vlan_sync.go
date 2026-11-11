package sync

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/state"
)

// syncVLANsWithData synchronizes VLANs from Mist networks to NetBox
func (e *Engine) syncVLANsWithData() error {
	logger.Info("Starting VLAN synchronization")

	stats := state.ObjectStats{}

	// Fetch organization networks (VLANs)
	networks, err := e.mistClient.GetOrgNetworks()
	if err != nil {
		return fmt.Errorf("failed to fetch org networks: %w", err)
	}
	e.state.RecordAPICall("mist")

	stats.Discovered = len(networks)
	logger.WithField("count", len(networks)).Info("Discovered networks from Mist")

	// Process each network
	for _, network := range networks {
		// Extract network details
		vlanID, hasVLAN := network["vlan_id"].(float64)
		if !hasVLAN || vlanID == 0 {
			// Skip networks without VLAN ID
			continue
		}

		name, _ := network["name"].(string)
		if name == "" {
			logger.Warn("Network has no name, skipping")
			continue
		}

		subnet, _ := network["subnet"].(string)
		isolation, _ := network["isolation"].(bool)

		// Build description
		description := ""
		if subnet != "" {
			description = fmt.Sprintf("Subnet: %s", subnet)
			if isolation {
				description += " (Isolated)"
			}
		}

		// Create VLAN in NetBox
		vlan, err := e.netboxClient.FindOrCreateVLAN(int(vlanID), name, description)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"vid":   int(vlanID),
				"name":  name,
				"error": err,
			}).Error("Failed to create VLAN")
			stats.Errors++
			e.state.RecordError("vlan", name, "create", err)
			continue
		}
		e.state.RecordAPICall("netbox")

		if vlan.GetId() > 0 {
			// Check if this was newly created or already existed
			// (we can't easily tell from the API, so we'll count all as processed)
			stats.Created++
		}

		logger.WithFields(map[string]interface{}{
			"vid":    int(vlanID),
			"name":   name,
			"subnet": subnet,
		}).Debug("Processed VLAN")
	}

	e.state.RecordObjectStats("vlans", stats)
	logger.WithFields(map[string]interface{}{
		"discovered": stats.Discovered,
		"created":    stats.Created,
		"errors":     stats.Errors,
	}).Info("VLAN synchronization complete")

	return nil
}
