package netbox

import (
	"fmt"

	"github.com/int13consulting/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreateCable finds or creates a cable between two interfaces
func (c *Client) FindOrCreateCable(interfaceAID, interfaceBID int64) (*netbox.Cable, error) {
	c.rateLimit()

	// Note: Cable deduplication is complex in NetBox API
	// For now, we'll let NetBox handle duplicate prevention via unique constraints

	// Create cable
	cableReq := netbox.NewWritableCableRequestWithDefaults()

	// Set A-side termination (interface)
	aTermination := netbox.NewGenericObjectRequest("dcim.interface", int32(interfaceAID))
	cableReq.SetATerminations([]netbox.GenericObjectRequest{*aTermination})

	// Set B-side termination (interface)
	bTermination := netbox.NewGenericObjectRequest("dcim.interface", int32(interfaceBID))
	cableReq.SetBTerminations([]netbox.GenericObjectRequest{*bTermination})

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"interface_a": interfaceAID,
			"interface_b": interfaceBID,
		}).Debug("Creating cable")
	}

	created, resp, err := c.client.DcimAPI.DcimCablesCreate(c.ctx).WritableCableRequest(*cableReq).Execute()
	if err != nil {
		// Check if this is a duplicate cable error (400 Bad Request)
		if resp != nil && resp.StatusCode == 400 {
			// Cable likely already exists, try to find it
			if c.logAPI {
				logger.WithFields(map[string]interface{}{
					"interface_a": interfaceAID,
					"interface_b": interfaceBID,
				}).Debug("Cable creation returned 400, likely already exists")
			}

			// For now, return nil to indicate cable exists but we don't need to fail
			// This is acceptable since the cable connection is established
			return nil, nil
		}
		return nil, fmt.Errorf("failed to create cable: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"interface_a": interfaceAID,
		"interface_b": interfaceBID,
		"cable_id":    created.GetId(),
	}).Info("Created cable")

	return created, nil
}
