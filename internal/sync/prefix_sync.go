package sync

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/state"
)

// syncPrefixesWithData synchronizes IP prefixes from Mist networks to NetBox
func (e *Engine) syncPrefixesWithData() error {
	logger.Info("Starting IP prefix synchronization")

	stats := state.ObjectStats{}

	// Fetch organization networks (includes subnet information)
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
		subnet, hasSubnet := network["subnet"].(string)
		if !hasSubnet || subnet == "" {
			// Skip networks without subnet
			continue
		}

		vlanID, _ := network["vlan_id"].(float64)
		name, _ := network["name"].(string)
		isolation, _ := network["isolation"].(bool)

		// Build description
		description := ""
		if name != "" {
			description = fmt.Sprintf("Network: %s", name)
			if isolation {
				description += " (Isolated)"
			}
		}

		// Find the VLAN in NetBox to get its NetBox ID
		var netboxVLANID *int64
		if vlanID > 0 {
			vlan, err := e.netboxClient.FindVLANByVID(int(vlanID))
			if err != nil {
				logger.WithFields(map[string]interface{}{
					"vid":   int(vlanID),
					"error": err,
				}).Warn("Failed to find VLAN for prefix linkage")
			} else if vlan != nil {
				e.state.RecordAPICall("netbox")
				id := int64(vlan.GetId())
				netboxVLANID = &id
			}
		}

		// Create prefix in NetBox
		prefix, err := e.netboxClient.FindOrCreatePrefix(subnet, netboxVLANID, description)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"prefix": subnet,
				"name":   name,
				"error":  err,
			}).Error("Failed to create prefix")
			stats.Errors++
			e.state.RecordError("prefix", subnet, "create", err)
			continue
		}
		e.state.RecordAPICall("netbox")

		if prefix.GetId() > 0 {
			stats.Created++
		}

		logger.WithFields(map[string]interface{}{
			"prefix":  subnet,
			"name":    name,
			"vlan_id": vlanID,
		}).Debug("Processed prefix")
	}

	e.state.RecordObjectStats("prefixes", stats)
	logger.WithFields(map[string]interface{}{
		"discovered": stats.Discovered,
		"created":    stats.Created,
		"errors":     stats.Errors,
	}).Info("IP prefix synchronization complete")

	return nil
}
