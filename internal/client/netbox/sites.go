package netbox

import (
	"fmt"
	"io"
	"strings"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// FindSiteByName finds a site by its name
func (c *Client) FindSiteByName(name string) (*netbox.Site, error) {
	c.rateLimit()

	req := c.client.DcimAPI.DcimSitesList(c.ctx).Name([]string{name})
	result, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query sites: %w", err)
	}

	if result.GetCount() == 0 {
		return nil, nil
	}

	results := result.GetResults()
	return &results[0], nil
}

// CreateSite creates a new site in NetBox
func (c *Client) CreateSite(site *models.Site, tags []string) (*netbox.Site, error) {
	c.rateLimit()

	req := netbox.NewWritableSiteRequestWithDefaults()
	req.SetName(site.Name)
	req.SetSlug(config.NormalizeSlug(site.Name))

	// Set description if available
	if site.Description != "" {
		req.SetDescription(site.Description)
	}

	// Set physical address
	if site.Address != "" {
		req.SetPhysicalAddress(site.Address)
	}

	// Set timezone
	if site.Timezone != "" {
		req.SetTimeZone(site.Timezone)
	}

	// Set latitude/longitude
	if site.Latitude != nil {
		req.SetLatitude(*site.Latitude)
	}
	if site.Longitude != nil {
		req.SetLongitude(*site.Longitude)
	}

	// Set status
	if site.Status != "" {
		status := c.mapSiteStatus(site.Status)
		req.SetStatus(status)
	}

	// Set region if specified
	if site.Region != "" {
		regionID, err := c.getRegionIDByPath(site.Region)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site":   site.Name,
				"region": site.Region,
			}).Warnf("Failed to find region: %v", err)
		} else if regionID != nil {
			regionUnion := netbox.Int32AsPatchedWritableSiteRequestRegion(regionID)
			req.SetRegion(regionUnion)
		}
	}

	// Set site group if specified
	if site.SiteGroup != "" {
		groupID, err := c.getSiteGroupIDByPath(site.SiteGroup)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site":       site.Name,
				"site_group": site.SiteGroup,
			}).Warnf("Failed to find site group: %v", err)
		} else if groupID != nil {
			groupUnion := netbox.Int32AsPatchedWritableSiteRequestGroup(groupID)
			req.SetGroup(groupUnion)
		}
	}

	// Set tags
	if len(tags) > 0 {
		tagRequests := make([]netbox.NestedTagRequest, len(tags))
		for i, tag := range tags {
			tagRequests[i] = *netbox.NewNestedTagRequest(tag, config.NormalizeSlug(tag))
		}
		req.SetTags(tagRequests)
	}

	// Set custom fields
	if len(site.CustomFields) > 0 {
		req.SetCustomFields(site.CustomFields)
	}

	if c.logAPI {
		logger.WithField("site", site.Name).Debug("Creating site")
	}

	result, httpResp, err := c.client.DcimAPI.DcimSitesCreate(c.ctx).WritableSiteRequest(*req).Execute()
	if err != nil {
		// Try to get more details from the HTTP response
		if httpResp != nil && httpResp.Body != nil {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			return nil, fmt.Errorf("failed to create site (HTTP %d): %s - %w", httpResp.StatusCode, string(bodyBytes), err)
		}
		return nil, fmt.Errorf("failed to create site: %w", err)
	}

	logger.WithField("site", site.Name).Info("Created site")
	return result, nil
}

// UpdateSite updates an existing site in NetBox
func (c *Client) UpdateSite(siteID int64, site *models.Site, tags []string) (*netbox.Site, error) {
	c.rateLimit()

	req := netbox.NewPatchedWritableSiteRequestWithDefaults()

	// Update fields that may have changed
	req.SetDescription(site.Description)

	if site.Address != "" {
		req.SetPhysicalAddress(site.Address)
	}

	if site.Timezone != "" {
		req.SetTimeZone(site.Timezone)
	}

	if site.Latitude != nil {
		req.SetLatitude(*site.Latitude)
	}
	if site.Longitude != nil {
		req.SetLongitude(*site.Longitude)
	}

	if site.Status != "" {
		status := c.mapSiteStatus(site.Status)
		req.SetStatus(status)
	}

	// Update region
	if site.Region != "" {
		regionID, err := c.getRegionIDByPath(site.Region)
		if err == nil && regionID != nil {
			regionUnion := netbox.Int32AsPatchedWritableSiteRequestRegion(regionID)
			req.SetRegion(regionUnion)
		}
	}

	// Update site group
	if site.SiteGroup != "" {
		groupID, err := c.getSiteGroupIDByPath(site.SiteGroup)
		if err == nil && groupID != nil {
			groupUnion := netbox.Int32AsPatchedWritableSiteRequestGroup(groupID)
			req.SetGroup(groupUnion)
		}
	}

	// Update tags
	if len(tags) > 0 {
		tagRequests := make([]netbox.NestedTagRequest, len(tags))
		for i, tag := range tags {
			tagRequests[i] = *netbox.NewNestedTagRequest(tag, config.NormalizeSlug(tag))
		}
		req.SetTags(tagRequests)
	}

	// Update custom fields
	if len(site.CustomFields) > 0 {
		req.SetCustomFields(site.CustomFields)
	}

	if c.logAPI {
		logger.WithField("site", site.Name).Debug("Updating site")
	}

	result, _, err := c.client.DcimAPI.DcimSitesPartialUpdate(c.ctx, int32(siteID)).PatchedWritableSiteRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to update site: %w", err)
	}

	logger.WithField("site", site.Name).Info("Updated site")
	return result, nil
}

// getRegionIDByPath finds a region ID by its path (e.g., "Canada/Ontario")
func (c *Client) getRegionIDByPath(path string) (*int32, error) {
	parts := strings.Split(path, "/")
	leafName := parts[len(parts)-1]

	// Search for the region
	c.rateLimit()
	req := c.client.DcimAPI.DcimRegionsList(c.ctx).Name([]string{leafName})
	result, _, err := req.Execute()
	if err != nil {
		return nil, err
	}

	if result.GetCount() == 0 {
		return nil, fmt.Errorf("region not found: %s", path)
	}

	results := result.GetResults()
	region := results[0]
	id := int32(region.GetId())

	return &id, nil
}

// getSiteGroupIDByPath finds a site group ID by its path
func (c *Client) getSiteGroupIDByPath(path string) (*int32, error) {
	parts := strings.Split(path, "/")
	leafName := parts[len(parts)-1]

	// Search for the site group
	c.rateLimit()
	req := c.client.DcimAPI.DcimSiteGroupsList(c.ctx).Name([]string{leafName})
	result, _, err := req.Execute()
	if err != nil {
		return nil, err
	}

	if result.GetCount() == 0 {
		return nil, fmt.Errorf("site group not found: %s", path)
	}

	results := result.GetResults()
	group := results[0]
	id := int32(group.GetId())

	return &id, nil
}

// mapSiteStatus maps our status to NetBox status value
func (c *Client) mapSiteStatus(status string) netbox.LocationStatusValue {
	// NetBox site statuses: active, planned, retired, staging
	statusMap := map[string]netbox.LocationStatusValue{
		"active":  netbox.LOCATIONSTATUSVALUE_ACTIVE,
		"planned": netbox.LOCATIONSTATUSVALUE_PLANNED,
		"retired": netbox.LOCATIONSTATUSVALUE_RETIRED,
		"staging": netbox.LOCATIONSTATUSVALUE_STAGING,
	}

	if val, ok := statusMap[status]; ok {
		return val
	}

	// Default to active
	return netbox.LOCATIONSTATUSVALUE_ACTIVE
}
