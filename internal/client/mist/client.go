package mist

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gregwight/mistclient"
	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
)

// Client wraps the Mist API client
type Client struct {
	client       *mistclient.APIClient
	baseURL      string
	apiKey       string
	orgID        string
	logAPI       bool
	includeSites []string // Development filter: only sync these sites
}

// rawSite is a lenient version of Site that can handle string coordinates
type rawSite struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Timezone string                 `json:"timezone"`
	Address  string                 `json:"address"`
	LatLng   map[string]interface{} `json:"latlng"` // Can be float or string
}

// rawDeviceStat is a lenient version of DeviceStat that handles the essential fields
type rawDeviceStat struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	Serial  string `json:"serial"`
	Mac     string `json:"mac"`
	SiteID  string `json:"site_id"`
	Type    string `json:"type"`
	Version string `json:"version"` // Firmware version
	IP      string `json:"ip"`
	ExtIP   string `json:"ext_ip"`
}

// NewClient creates a new Mist client
func NewClient(cfg *config.MistConfig, filter *config.FilterConfig, logAPI bool) (*Client, error) {
	// Create Mist config
	mistCfg := &mistclient.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIToken,
		Timeout: 30 * time.Second,
	}

	// Create logger for Mist client
	mistLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn, // Only show warnings/errors from Mist client
	}))

	// Create Mist client
	client, err := mistclient.New(mistCfg, mistLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mist client: %w", err)
	}

	// Test authentication by getting self info
	_, err = client.GetSelf()
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with Mist: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"org_id": cfg.OrganizationID,
	}).Info("Connected to Mist API")

	return &Client{
		client:       client,
		baseURL:      cfg.BaseURL,
		apiKey:       cfg.APIToken,
		orgID:        cfg.OrganizationID,
		logAPI:       logAPI,
		includeSites: filter.IncludeSites,
	}, nil
}

// GetSites retrieves all sites for the organization
// Uses custom HTTP call to handle malformed latlng data
func (c *Client) GetSites() ([]*models.Site, error) {
	if c.logAPI {
		logger.Debug("Fetching sites from Mist")
	}

	// Make direct HTTP call to handle malformed coordinates
	url := fmt.Sprintf("%s/api/v1/orgs/%s/sites", c.baseURL, c.orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get sites: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse with lenient handling
	var rawSites []rawSite
	if err := json.NewDecoder(resp.Body).Decode(&rawSites); err != nil {
		return nil, fmt.Errorf("failed to parse sites: %w", err)
	}

	result := make([]*models.Site, 0, len(rawSites))
	for _, mistSite := range rawSites {
		now := time.Now()
		site := &models.Site{
			SourceID:   mistSite.ID,
			SourceType: "mist",
			Name:       mistSite.Name,
			Address:    mistSite.Address,
			Timezone:   mistSite.Timezone,
			CustomFields: map[string]interface{}{
				"mist_site_id": mistSite.ID,
				"mist_org_id":  c.orgID,
				"last_synced":  now.Format("2006-01-02"), // NetBox date format
			},
			LastSynced: now,
		}

		// Add latitude/longitude if available, handling both float and string types
		if mistSite.LatLng != nil {
			if latVal, ok := mistSite.LatLng["lat"]; ok {
				var lat64 float64
				switch v := latVal.(type) {
				case float64:
					lat64 = v
				case float32:
					lat64 = float64(v)
				case string:
					// Skip string coordinates - data quality issue
					if c.logAPI {
						logger.WithField("site", mistSite.Name).Debug("Skipping string latitude")
					}
				default:
					if c.logAPI {
						logger.WithFields(map[string]interface{}{
							"site": mistSite.Name,
							"type": fmt.Sprintf("%T", v),
						}).Debug("Unknown latitude type")
					}
				}
				if lat64 != 0 {
					// Round to 6 decimal places for NetBox validation
					rounded := roundCoordinate(lat64, 6)
					site.Latitude = &rounded
				}
			}
			if lngVal, ok := mistSite.LatLng["lng"]; ok {
				var lng64 float64
				switch v := lngVal.(type) {
				case float64:
					lng64 = v
				case float32:
					lng64 = float64(v)
				case string:
					// Skip string coordinates
					if c.logAPI {
						logger.WithField("site", mistSite.Name).Debug("Skipping string longitude")
					}
				default:
					if c.logAPI {
						logger.WithFields(map[string]interface{}{
							"site": mistSite.Name,
							"type": fmt.Sprintf("%T", v),
						}).Debug("Unknown longitude type")
					}
				}
				if lng64 != 0 {
					// Round to 6 decimal places for NetBox validation
					rounded := roundCoordinate(lng64, 6)
					site.Longitude = &rounded
				}
			}
		}

		result = append(result, site)
	}

	// Apply include_sites filter if specified (for development)
	if len(c.includeSites) > 0 {
		filtered := make([]*models.Site, 0, len(c.includeSites))
		includedMap := make(map[string]bool)
		for _, name := range c.includeSites {
			includedMap[name] = true
		}

		for _, site := range result {
			if includedMap[site.Name] {
				filtered = append(filtered, site)
			}
		}

		if len(filtered) < len(c.includeSites) {
			logger.WithFields(map[string]interface{}{
				"requested": len(c.includeSites),
				"found":     len(filtered),
			}).Warn("Some sites from include_sites filter were not found")
		}

		logger.WithFields(map[string]interface{}{
			"total":    len(result),
			"filtered": len(filtered),
		}).Info("Applied include_sites filter (development mode)")
		result = filtered
	}

	logger.WithField("count", len(result)).Info("Retrieved sites from Mist")
	return result, nil
}

// getSiteDeviceStatsRaw fetches device stats using direct HTTP call to handle parsing issues
// Calls /stats/devices with type filter to get all device types (ap, switch, gateway)
func (c *Client) getSiteDeviceStatsRaw(siteID string) ([]rawDeviceStat, error) {
	var allDevices []rawDeviceStat

	// Fetch each device type separately since the API defaults type filter to "ap"
	deviceTypes := []string{"ap", "switch", "gateway"}

	for _, deviceType := range deviceTypes {
		devices, err := c.fetchDevicesByType(siteID, deviceType)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site_id": siteID,
				"type":    deviceType,
				"error":   err,
			}).Warn("Failed to fetch devices of type")
		} else {
			allDevices = append(allDevices, devices...)
		}
	}

	return allDevices, nil
}

// fetchDevicesByType fetches devices from /stats/devices with a specific type filter
func (c *Client) fetchDevicesByType(siteID, deviceType string) ([]rawDeviceStat, error) {
	url := fmt.Sprintf("%s/api/v1/sites/%s/stats/devices?type=%s", c.baseURL, siteID, deviceType)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get device stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var devices []rawDeviceStat
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, fmt.Errorf("failed to parse device stats: %w", err)
	}

	return devices, nil
}

// GetDevices retrieves all devices for the organization
func (c *Client) GetDevices() ([]*models.Device, error) {
	if c.logAPI {
		logger.Debug("Fetching devices from Mist")
	}

	// Get all sites first using our custom method (handles latlng parsing issues)
	sites, err := c.GetSites()
	if err != nil {
		return nil, fmt.Errorf("failed to get sites: %w", err)
	}

	return c.GetDevicesWithSites(sites)
}

// GetDevicesWithSites retrieves all devices using cached site data
func (c *Client) GetDevicesWithSites(sites []*models.Site) ([]*models.Device, error) {
	if c.logAPI {
		logger.Debug("Fetching devices from Mist with cached sites")
	}

	// Collect device stats from all sites using direct HTTP calls (includes firmware version)
	var allDevices []rawDeviceStat
	for _, site := range sites {
		deviceStats, err := c.getSiteDeviceStatsRaw(site.SourceID)
		if err != nil {
			logger.WithField("site", site.Name).Warnf("Failed to get device stats: %v", err)
			continue
		}
		allDevices = append(allDevices, deviceStats...)
	}

	result := make([]*models.Device, 0, len(allDevices))
	deviceTypeCounts := make(map[string]int)

	for _, rawDevice := range allDevices {
		deviceTypeCounts[rawDevice.Type]++
		device := c.convertRawDeviceStat(&rawDevice, rawDevice.SiteID)
		if device != nil {
			result = append(result, device)
		}
	}

	logger.WithFields(map[string]interface{}{
		"total":       len(result),
		"type_counts": deviceTypeCounts,
	}).Info("Retrieved devices from Mist")
	return result, nil
}

// GetSiteDevices retrieves devices for a specific site
func (c *Client) GetSiteDevices(siteID string) ([]*models.Device, error) {
	if c.logAPI {
		logger.WithField("site_id", siteID).Debug("Fetching devices for site")
	}

	// Get device stats by site (includes firmware version)
	deviceStats, err := c.client.GetSiteDeviceStats(siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get site device stats: %w", err)
	}

	result := make([]*models.Device, 0, len(deviceStats))
	for _, mistDeviceStat := range deviceStats {
		device := c.convertMistDeviceStat(&mistDeviceStat, siteID)
		if device != nil {
			result = append(result, device)
		}
	}

	return result, nil
}

// convertMistDevice converts a Mist site device to internal model
func (c *Client) convertMistDevice(mistDevice *mistclient.Device, siteID string) *models.Device {
	if mistDevice == nil {
		return nil
	}

	// Determine device role based on type
	role := c.determineDeviceRole(mistDevice.Type.String())

	now := time.Now()
	device := &models.Device{
		SourceID:     mistDevice.ID,
		SourceType:   "mist",
		Name:         mistDevice.Name,
		Serial:       mistDevice.Serial,
		Model:        mistDevice.Model,
		Manufacturer: "Juniper",
		Role:         role,
		Status:       "active",
		CustomFields: map[string]interface{}{
			"mist_device_id": mistDevice.ID,
			"mist_site_id":   siteID,
			"mist_model":     mistDevice.Model,
			"last_synced":    now.Format("2006-01-02"), // NetBox date format
		},
		LastSynced: now,
	}

	// Set device type
	device.DeviceType = mistDevice.Model

	// Add MAC if available (note: Mac not MAC)
	if mistDevice.Mac != "" {
		device.MAC = mistDevice.Mac
		device.CustomFields["mist_mac"] = mistDevice.Mac
	}

	return device
}

// convertMistDeviceStat converts a Mist DeviceStat to internal model (includes firmware version)
func (c *Client) convertMistDeviceStat(mistDeviceStat *mistclient.DeviceStat, siteID string) *models.Device {
	if mistDeviceStat == nil {
		return nil
	}

	// Determine device role based on type
	role := c.determineDeviceRole(mistDeviceStat.Type.String())

	now := time.Now()
	device := &models.Device{
		SourceID:     mistDeviceStat.ID,
		SourceType:   "mist",
		Name:         mistDeviceStat.Name,
		Serial:       mistDeviceStat.Serial,
		Model:        mistDeviceStat.Model,
		Manufacturer: "Juniper",
		Role:         role,
		Status:       "active",
		CustomFields: map[string]interface{}{
			"mist_device_id": mistDeviceStat.ID,
			"mist_site_id":   siteID,
			"mist_model":     mistDeviceStat.Model,
			"last_synced":    now.Format("2006-01-02"), // NetBox date format
		},
		LastSynced: now,
	}

	// Set device type
	device.DeviceType = mistDeviceStat.Model

	// Add MAC if available
	if mistDeviceStat.Mac != "" {
		device.MAC = mistDeviceStat.Mac
		device.CustomFields["mist_mac"] = mistDeviceStat.Mac
	}

	// Add firmware version if available
	if mistDeviceStat.Version != "" {
		device.Firmware = mistDeviceStat.Version
		device.CustomFields["firmware_version"] = mistDeviceStat.Version
	}

	// Add management IP if available
	if mistDeviceStat.IP.IsValid() {
		device.CustomFields["management_ip"] = mistDeviceStat.IP.String()
	}

	// Add external IP if available
	if mistDeviceStat.ExtIP.IsValid() {
		device.CustomFields["external_ip"] = mistDeviceStat.ExtIP.String()
	}

	return device
}

// convertRawDeviceStat converts a raw DeviceStat to internal model
func (c *Client) convertRawDeviceStat(rawDevice *rawDeviceStat, siteID string) *models.Device {
	if rawDevice == nil {
		return nil
	}

	// Determine device role based on type
	role := c.determineDeviceRole(rawDevice.Type)

	now := time.Now()
	device := &models.Device{
		SourceID:     rawDevice.ID,
		SourceType:   "mist",
		Name:         rawDevice.Name,
		Serial:       rawDevice.Serial,
		Model:        rawDevice.Model,
		Manufacturer: "Juniper",
		Role:         role,
		Status:       "active",
		CustomFields: map[string]interface{}{
			"mist_device_id": rawDevice.ID,
			"mist_site_id":   siteID,
			"mist_model":     rawDevice.Model,
			"last_synced":    now.Format("2006-01-02"), // NetBox date format
		},
		LastSynced: now,
	}

	// Set device type
	device.DeviceType = rawDevice.Model

	// Add MAC if available
	if rawDevice.Mac != "" {
		device.MAC = rawDevice.Mac
		device.CustomFields["mist_mac"] = rawDevice.Mac
	}

	// Add firmware version if available
	if rawDevice.Version != "" {
		device.Firmware = rawDevice.Version
		device.CustomFields["firmware_version"] = rawDevice.Version
	}

	// Add management IP if available
	if rawDevice.IP != "" {
		device.CustomFields["management_ip"] = rawDevice.IP
	}

	// Add external IP if available
	if rawDevice.ExtIP != "" {
		device.CustomFields["external_ip"] = rawDevice.ExtIP
	}

	return device
}

// determineDeviceRole maps Mist device type to NetBox role
func (c *Client) determineDeviceRole(deviceType string) string {
	switch deviceType {
	case "ap":
		return "Access Point"
	case "switch":
		return "Switch"
	case "gateway":
		return "Gateway"
	case "srx":
		return "Firewall"
	default:
		return "Network Device"
	}
}

// GetOrganizationInfo gets information about the organization
func (c *Client) GetOrganizationInfo() (string, error) {
	// The mistclient doesn't have a direct GetOrganization method
	// Return the org ID as the name for now
	return c.orgID, nil
}

// GetDiscoveredSwitches retrieves discovered switch information including port connectivity
func (c *Client) GetDiscoveredSwitches(siteID string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/sites/%s/stats/discovered_switches/search", c.baseURL, siteID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovered switches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Search endpoints return a wrapper object with results
	var response struct {
		Results []map[string]interface{} `json:"results"`
		Total   int                      `json:"total"`
		Limit   int                      `json:"limit"`
		Offset  int                      `json:"offset"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse discovered switches: %w", err)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"site_id": siteID,
			"count":   len(response.Results),
			"total":   response.Total,
		}).Debug("Retrieved discovered switches")
	}

	return response.Results, nil
}

// GetOrgNetworks retrieves organization-level networks (VLANs)
func (c *Client) GetOrgNetworks() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/networks", c.baseURL, c.orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get org networks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var networks []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&networks); err != nil {
		return nil, fmt.Errorf("failed to parse org networks: %w", err)
	}

	if c.logAPI {
		logger.WithField("count", len(networks)).Debug("Retrieved org networks")
	}

	return networks, nil
}

// GetOrgWLANs retrieves organization-level WLANs (SSID templates)
func (c *Client) GetOrgWLANs() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/wlans", c.baseURL, c.orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get org WLANs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var wlans []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&wlans); err != nil {
		return nil, fmt.Errorf("failed to parse org WLANs: %w", err)
	}

	if c.logAPI {
		logger.WithField("count", len(wlans)).Debug("Retrieved org WLANs")
	}

	return wlans, nil
}

// GetSiteWLANs retrieves WLANs (SSIDs) for a site
func (c *Client) GetSiteWLANs(siteID string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/sites/%s/wlans", c.baseURL, siteID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get site WLANs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var wlans []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&wlans); err != nil {
		return nil, fmt.Errorf("failed to parse site WLANs: %w", err)
	}

	if c.logAPI {
		logger.WithFields(map[string]interface{}{
			"site_id": siteID,
			"count":   len(wlans),
		}).Debug("Retrieved site WLANs")
	}

	return wlans, nil
}

// roundCoordinate rounds a coordinate to the specified number of decimal places
// NetBox requires: latitude max 8 digits total, longitude max 6 decimal places
func roundCoordinate(coord float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(coord*multiplier) / multiplier
}
