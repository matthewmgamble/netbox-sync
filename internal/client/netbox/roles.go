package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureDeviceRole ensures a device role exists in NetBox
func (c *Client) EnsureDeviceRole(role *models.DeviceRole) (*netbox.DeviceRole, error) {
	c.rateLimit()

	// Check if role exists
	existing, err := c.findDeviceRoleBySlug(role.Slug)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if c.logAPI {
			logger.WithField("role", role.Name).Debug("Device role already exists")
		}
		return existing, nil
	}

	// Create role
	return c.createDeviceRole(role)
}

// findDeviceRoleBySlug finds a device role by slug
func (c *Client) findDeviceRoleBySlug(slug string) (*netbox.DeviceRole, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimDeviceRolesList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query device roles: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// createDeviceRole creates a new device role
func (c *Client) createDeviceRole(role *models.DeviceRole) (*netbox.DeviceRole, error) {
	c.rateLimit()

	req := netbox.NewWritableDeviceRoleRequestWithDefaults()
	req.SetName(role.Name)
	req.SetSlug(role.Slug)

	if role.Color != "" {
		req.SetColor(role.Color)
	} else {
		// Default colors for common roles
		req.SetColor(c.getDefaultRoleColor(role.Name))
	}

	if role.Description != "" {
		req.SetDescription(role.Description)
	}

	if c.logAPI {
		logger.WithField("role", role.Name).Debug("Creating device role")
	}

	result, _, err := c.client.DcimAPI.DcimDeviceRolesCreate(c.ctx).WritableDeviceRoleRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create device role: %w", err)
	}

	logger.WithField("role", role.Name).Info("Created device role")
	return result, nil
}

// FindDeviceRoleByName finds a device role by name
func (c *Client) FindDeviceRoleByName(name string) (*netbox.DeviceRole, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimDeviceRolesList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query device roles: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// getDefaultRoleColor returns a default color for common role names
func (c *Client) getDefaultRoleColor(roleName string) string {
	colors := map[string]string{
		"Access Point":   "2196f3", // Blue
		"Switch":         "4caf50", // Green
		"Gateway":        "ff9800", // Orange
		"Firewall":       "f44336", // Red
		"Network Device": "9e9e9e", // Grey
	}

	if color, ok := colors[roleName]; ok {
		return color
	}

	return "607d8b" // Blue Grey default
}
