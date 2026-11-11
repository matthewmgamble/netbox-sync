package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsurePlatform ensures a platform exists in NetBox
func (c *Client) EnsurePlatform(platform *models.Platform) (*netbox.Platform, error) {
	c.rateLimit()

	// Check if platform exists
	existing, err := c.findPlatformBySlug(platform.Slug)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if c.logAPI {
			logger.WithField("platform", platform.Name).Debug("Platform already exists")
		}
		return existing, nil
	}

	// Create platform
	return c.createPlatform(platform)
}

// findPlatformBySlug finds a platform by slug
func (c *Client) findPlatformBySlug(slug string) (*netbox.Platform, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimPlatformsList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query platforms: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// createPlatform creates a new platform
func (c *Client) createPlatform(platform *models.Platform) (*netbox.Platform, error) {
	c.rateLimit()

	req := netbox.NewPlatformRequestWithDefaults()
	req.SetName(platform.Name)
	req.SetSlug(platform.Slug)

	if platform.Description != "" {
		req.SetDescription(platform.Description)
	}

	// Set manufacturer if specified (optional, so we skip it for now)
	// if platform.Manufacturer != "" {
	// 	mfr, err := c.FindManufacturerByName(platform.Manufacturer)
	// 	if err == nil && mfr != nil {
	// 		// Manufacturer is optional for platforms
	// 	}
	// }

	if c.logAPI {
		logger.WithField("platform", platform.Name).Debug("Creating platform")
	}

	result, _, err := c.client.DcimAPI.DcimPlatformsCreate(c.ctx).PlatformRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create platform: %w", err)
	}

	logger.WithField("platform", platform.Name).Info("Created platform")
	return result, nil
}

// FindPlatformByName finds a platform by name
func (c *Client) FindPlatformByName(name string) (*netbox.Platform, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimPlatformsList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query platforms: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}
