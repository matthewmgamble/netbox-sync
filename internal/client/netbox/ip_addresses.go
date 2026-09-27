package netbox

import (
	"fmt"
	"io"
	"net/netip"

	"github.com/int13consulting/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindOrCreateIPAddress finds an existing IP address or creates a new one
// If interfaceID is provided, assigns the IP to that interface
func (c *Client) FindOrCreateIPAddress(ipStr string, interfaceID *int64) (*netbox.IPAddress, error) {
	c.rateLimit()

	// Parse IP to validate format
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return nil, fmt.Errorf("invalid IP address format: %w", err)
	}

	// Determine prefix length (32 for IPv4, 128 for IPv6)
	prefixLen := 32
	if addr.Is6() {
		prefixLen = 128
	}
	ipWithPrefix := fmt.Sprintf("%s/%d", ipStr, prefixLen)

	// Check if IP already exists
	req := c.client.IpamAPI.IpamIpAddressesList(c.ctx).Address([]string{ipWithPrefix})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query IP addresses: %w", err)
	}

	if result.GetCount() > 0 {
		results := result.GetResults()
		existing := &results[0]

		if c.logAPI {
			logger.WithFields(map[string]interface{}{
				"ip": ipStr,
				"id": existing.GetId(),
			}).Debug("IP address already exists")
		}

		return existing, nil
	}

	// Create new IP address
	ipReq := netbox.NewWritableIPAddressRequestWithDefaults()
	ipReq.SetAddress(ipWithPrefix)

	// Assign to interface if provided
	if interfaceID != nil && *interfaceID > 0 {
		ipReq.SetAssignedObjectType("dcim.interface")
		ipReq.SetAssignedObjectId(*interfaceID)
	}

	if c.logAPI {
		logger.WithField("ip", ipStr).Debug("Creating IP address")
	}

	created, _, err := c.client.IpamAPI.IpamIpAddressesCreate(c.ctx).WritableIPAddressRequest(*ipReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create IP address: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"ip": ipStr,
		"id": created.GetId(),
	}).Info("Created IP address")

	return created, nil
}

// SetDevicePrimaryIPs sets the primary IPv4 and/or IPv6 addresses for a device
func (c *Client) SetDevicePrimaryIPs(deviceID int64, primaryIPv4ID, primaryIPv6ID *int64) error {
	c.rateLimit()

	req := netbox.NewPatchedWritableDeviceWithConfigContextRequestWithDefaults()

	if primaryIPv4ID != nil && *primaryIPv4ID > 0 {
		ipv4ID := int32(*primaryIPv4ID)
		ipv4Union := netbox.Int32AsDeviceWithConfigContextRequestPrimaryIp4(&ipv4ID)
		req.SetPrimaryIp4(ipv4Union)
	}

	if primaryIPv6ID != nil && *primaryIPv6ID > 0 {
		ipv6ID := int32(*primaryIPv6ID)
		ipv6Union := netbox.Int32AsDeviceWithConfigContextRequestPrimaryIp4(&ipv6ID)
		req.SetPrimaryIp6(ipv6Union)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"device_id":   deviceID,
			"primary_ip4": primaryIPv4ID,
			"primary_ip6": primaryIPv6ID,
		}).Debug("Setting device primary IPs")
	}

	result, resp, err := c.client.DcimAPI.DcimDevicesPartialUpdate(c.ctx, int32(deviceID)).PatchedWritableDeviceWithConfigContextRequest(*req).Execute()
	if err != nil {
		// Try to read the response body for detailed error
		var responseBody string
		if resp != nil && resp.Body != nil {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				responseBody = string(bodyBytes)
			}
		}

		logger.WithFields(map[string]interface{}{
			"device_id":     deviceID,
			"http_status":   resp.StatusCode,
			"error":         err.Error(),
			"response_body": responseBody,
		}).Error("Failed to set primary IPs - NetBox API error")

		return fmt.Errorf("failed to set primary IPs: %w", err)
	}

	// Log success with result
	if result != nil {
		logger.WithFields(map[string]interface{}{
			"device_id": deviceID,
			"device":    result.GetName(),
		}).Debug("Primary IPs set successfully")
	}

	logger.WithField("device_id", deviceID).Info("Set device primary IPs")
	return nil
}

// AssignIPToInterface assigns an existing IP address to an interface
func (c *Client) AssignIPToInterface(ipID, interfaceID int64) error {
	c.rateLimit()

	req := netbox.NewPatchedWritableIPAddressRequestWithDefaults()
	req.SetAssignedObjectType("dcim.interface")
	req.SetAssignedObjectId(interfaceID)

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"ip_id":        ipID,
			"interface_id": interfaceID,
		}).Debug("Assigning IP to interface")
	}

	_, _, err := c.client.IpamAPI.IpamIpAddressesPartialUpdate(c.ctx, int32(ipID)).PatchedWritableIPAddressRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("failed to assign IP to interface: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"ip_id":        ipID,
		"interface_id": interfaceID,
	}).Info("Assigned IP to interface")
	return nil
}
