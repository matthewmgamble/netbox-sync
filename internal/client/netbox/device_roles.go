package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// DeviceRoleDef defines a device role to create
type DeviceRoleDef struct {
	Name  string
	Slug  string
	Color string // 6-character hex color code
}

// EnsureDeviceRoles ensures all required device roles exist in NetBox
func (c *Client) EnsureDeviceRoles() error {
	logger.Info("Ensuring device roles exist in NetBox")

	// Define required device roles
	roles := []DeviceRoleDef{
		{Name: "Access Point", Slug: "access-point", Color: "2196f3"},   // Blue
		{Name: "Switch", Slug: "switch", Color: "4caf50"},                // Green
		{Name: "Gateway", Slug: "gateway", Color: "ff9800"},              // Orange
		{Name: "Firewall", Slug: "firewall", Color: "f44336"},            // Red
		{Name: "Network Device", Slug: "network-device", Color: "9e9e9e"}, // Gray
	}

	for _, role := range roles {
		if err := c.ensureDeviceRole(role); err != nil {
			logger.WithFields(map[string]interface{}{
				"role":  role.Name,
				"error": err,
			}).Warn("Failed to create device role")
			return err
		}
	}

	return nil
}

// ensureDeviceRole ensures a single device role exists
func (c *Client) ensureDeviceRole(roleDef DeviceRoleDef) error {
	c.rateLimit()

	// Check if role exists
	req := c.client.DcimAPI.DcimDeviceRolesList(c.ctx).Slug([]string{roleDef.Slug})
	result, _, err := req.Execute()
	if err != nil {
		return fmt.Errorf("failed to query device roles: %w", err)
	}

	if result.GetCount() > 0 {
		if c.logAPI {
			logger.WithField("role", roleDef.Name).Debug("Device role already exists")
		}
		return nil
	}

	// Create device role
	roleReq := netbox.NewWritableDeviceRoleRequestWithDefaults()
	roleReq.SetName(roleDef.Name)
	roleReq.SetSlug(roleDef.Slug)
	roleReq.SetColor(roleDef.Color)

	if c.logAPI {
		logger.WithField("role", roleDef.Name).Debug("Creating device role")
	}

	_, _, err = c.client.DcimAPI.DcimDeviceRolesCreate(c.ctx).WritableDeviceRoleRequest(*roleReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to create device role: %w", err)
	}

	logger.WithField("role", roleDef.Name).Info("Created device role")
	return nil
}

// GetDeviceRoleByName finds a device role by name
func (c *Client) GetDeviceRoleByName(name string) (*netbox.DeviceRole, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimDeviceRolesList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query device roles: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, fmt.Errorf("device role not found: %s", name)
	}

	results := result.GetResults()
	return &results[0], nil
}
