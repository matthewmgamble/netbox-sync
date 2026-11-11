package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureManufacturer ensures a manufacturer exists in NetBox
func (c *Client) EnsureManufacturer(mfr *models.Manufacturer) (*netbox.Manufacturer, error) {
	c.rateLimit()

	// Check if manufacturer exists
	existing, err := c.findManufacturerBySlug(mfr.Slug)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if c.logAPI {
			logger.WithField("manufacturer", mfr.Name).Debug("Manufacturer already exists")
		}
		return existing, nil
	}

	// Create manufacturer
	return c.createManufacturer(mfr)
}

// findManufacturerBySlug finds a manufacturer by slug
func (c *Client) findManufacturerBySlug(slug string) (*netbox.Manufacturer, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimManufacturersList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query manufacturers: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// createManufacturer creates a new manufacturer
func (c *Client) createManufacturer(mfr *models.Manufacturer) (*netbox.Manufacturer, error) {
	c.rateLimit()

	req := netbox.NewManufacturerRequestWithDefaults()
	req.SetName(mfr.Name)
	req.SetSlug(mfr.Slug)

	if mfr.Description != "" {
		req.SetDescription(mfr.Description)
	}

	if c.logAPI {
		logger.WithField("manufacturer", mfr.Name).Debug("Creating manufacturer")
	}

	result, _, err := c.client.DcimAPI.DcimManufacturersCreate(c.ctx).ManufacturerRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create manufacturer: %w", err)
	}

	logger.WithField("manufacturer", mfr.Name).Info("Created manufacturer")
	return result, nil
}

// FindManufacturerByName finds a manufacturer by name
func (c *Client) FindManufacturerByName(name string) (*netbox.Manufacturer, error) {
	slug := config.NormalizeSlug(name)
	return c.findManufacturerBySlug(slug)
}
