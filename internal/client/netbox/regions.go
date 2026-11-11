package netbox

import (
	"fmt"
	"io"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureRegionHierarchy ensures all regions in the hierarchy exist
func (c *Client) EnsureRegionHierarchy(regions []models.RegionDef) error {
	logger.Info("Ensuring region hierarchy exists in NetBox")

	// Create regions recursively
	for _, region := range regions {
		if err := c.ensureRegion(region, nil); err != nil {
			return err
		}
	}

	return nil
}

// ensureRegion creates a region and its children recursively
func (c *Client) ensureRegion(regionDef models.RegionDef, parentID *int64) error {
	c.rateLimit()

	// Check if region exists by name (NetBox validates uniqueness by name, not slug)
	existing, err := c.findRegionByName(regionDef.Name)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"region": regionDef.Name,
			"error":  err,
		}).Warn("Failed to check if region exists")
		return err
	}

	var regionID int64
	if existing != nil {
		regionID = int64(existing.GetId())
		if c.logAPI {
			logger.WithField("region", regionDef.Name).Debug("Region already exists")
		}
	} else {
		// Create region
		region, err := c.createRegion(regionDef.Name, regionDef.Slug, parentID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"region": regionDef.Name,
				"error":  err,
			}).Warn("Failed to create region, skipping children")
			// Return error but don't create children
			return fmt.Errorf("failed to create region %s: %w", regionDef.Name, err)
		}
		regionID = int64(region.GetId())
		logger.WithField("region", regionDef.Name).Info("Created region")
	}

	// Create children (continue even if one fails)
	for _, child := range regionDef.Children {
		if err := c.ensureRegion(child, &regionID); err != nil {
			logger.WithFields(map[string]interface{}{
				"parent": regionDef.Name,
				"child":  child.Name,
				"error":  err,
			}).Warn("Failed to create child region, continuing with siblings")
			// Continue with other children
		}
	}

	return nil
}

// findRegionByName finds a region by its name
func (c *Client) findRegionByName(name string) (*netbox.Region, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimRegionsList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query regions: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	}

	return nil, nil
}

// findRegionBySlug finds a region by its slug
func (c *Client) findRegionBySlug(slug string) (*netbox.Region, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimRegionsList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query regions: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	}

	return nil, nil
}

// createRegion creates a new region
func (c *Client) createRegion(name, slug string, parentID *int64) (*netbox.Region, error) {
	c.rateLimit()

	req := netbox.NewWritableRegionRequestWithDefaults()
	req.SetName(name)
	req.SetSlug(slug)

	if parentID != nil {
		req.SetParent(int32(*parentID))
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"name": name,
			"slug": slug,
		}).Debug("Creating region")
	}

	result, httpResp, err := c.client.DcimAPI.DcimRegionsCreate(c.ctx).WritableRegionRequest(*req).Execute()
	if err != nil {
		// Try to get more details from the HTTP response
		if httpResp != nil && httpResp.Body != nil {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			return nil, fmt.Errorf("failed to create region (HTTP %d): %s - %w", httpResp.StatusCode, string(bodyBytes), err)
		}
		return nil, err
	}

	return result, nil
}

// GetRegionByName finds a region by name (supports hierarchical names like "Canada/Ontario")
func (c *Client) GetRegionByName(name string) (*netbox.Region, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimRegionsList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query regions: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, fmt.Errorf("region not found: %s", name)
	}

	results := result.GetResults()
	return &results[0], nil
}
