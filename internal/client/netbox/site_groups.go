package netbox

import (
	"fmt"
	"io"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureSiteGroupHierarchy ensures all site groups in the hierarchy exist
func (c *Client) EnsureSiteGroupHierarchy(groups []models.SiteGroupDef) error {
	logger.Info("Ensuring site group hierarchy exists in NetBox")

	// Create site groups recursively
	for _, group := range groups {
		if err := c.ensureSiteGroup(group, nil); err != nil {
			return err
		}
	}

	return nil
}

// ensureSiteGroup creates a site group and its children recursively
func (c *Client) ensureSiteGroup(groupDef models.SiteGroupDef, parentID *int64) error {
	c.rateLimit()

	// Check if site group exists by name (NetBox validates uniqueness by name, not slug)
	existing, err := c.findSiteGroupByName(groupDef.Name)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"site_group": groupDef.Name,
			"error":      err,
		}).Warn("Failed to check if site group exists")
		return err
	}

	var groupID int64
	if existing != nil {
		groupID = int64(existing.GetId())
		if c.logAPI {
			logger.WithField("site_group", groupDef.Name).Debug("Site group already exists")
		}
	} else {
		// Create site group
		group, err := c.createSiteGroup(groupDef.Name, groupDef.Slug, parentID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site_group": groupDef.Name,
				"error":      err,
			}).Warn("Failed to create site group, skipping children")
			return fmt.Errorf("failed to create site group %s: %w", groupDef.Name, err)
		}
		groupID = int64(group.GetId())
		logger.WithField("site_group", groupDef.Name).Info("Created site group")
	}

	// Create children (continue even if one fails)
	for _, child := range groupDef.Children {
		if err := c.ensureSiteGroup(child, &groupID); err != nil {
			logger.WithFields(map[string]interface{}{
				"parent": groupDef.Name,
				"child":  child.Name,
				"error":  err,
			}).Warn("Failed to create child site group, continuing with siblings")
			// Continue with other children
		}
	}

	return nil
}

// findSiteGroupByName finds a site group by its name
func (c *Client) findSiteGroupByName(name string) (*netbox.SiteGroup, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimSiteGroupsList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query site groups: %w", err)
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

// findSiteGroupBySlug finds a site group by its slug
func (c *Client) findSiteGroupBySlug(slug string) (*netbox.SiteGroup, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimSiteGroupsList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query site groups: %w", err)
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

// createSiteGroup creates a new site group
func (c *Client) createSiteGroup(name, slug string, parentID *int64) (*netbox.SiteGroup, error) {
	c.rateLimit()

	req := netbox.NewWritableSiteGroupRequestWithDefaults()
	req.SetName(name)
	req.SetSlug(slug)

	if parentID != nil {
		req.SetParent(int32(*parentID))
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"name": name,
			"slug": slug,
		}).Debug("Creating site group")
	}

	result, httpResp, err := c.client.DcimAPI.DcimSiteGroupsCreate(c.ctx).WritableSiteGroupRequest(*req).Execute()
	if err != nil {
		// Try to get more details from the HTTP response
		if httpResp != nil && httpResp.Body != nil {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			return nil, fmt.Errorf("failed to create site group (HTTP %d): %s - %w", httpResp.StatusCode, string(bodyBytes), err)
		}
		return nil, err
	}

	return result, nil
}

// GetSiteGroupByName finds a site group by name
func (c *Client) GetSiteGroupByName(name string) (*netbox.SiteGroup, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimSiteGroupsList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query site groups: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, fmt.Errorf("site group not found: %s", name)
	}

	results := result.GetResults()
	return &results[0], nil
}
