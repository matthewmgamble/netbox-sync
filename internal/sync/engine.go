package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/int13consulting/netbox-sync/internal/client/mist"
	"github.com/int13consulting/netbox-sync/internal/client/netbox"
	"github.com/int13consulting/netbox-sync/internal/config"
	"github.com/int13consulting/netbox-sync/internal/logger"
	"github.com/int13consulting/netbox-sync/internal/models"
	"github.com/int13consulting/netbox-sync/internal/state"
)

// Engine orchestrates the synchronization process
type Engine struct {
	mistClient   *mist.Client
	netboxClient *netbox.Client
	config       *config.Config
	mapping      *models.MappingConfig
	state        *state.Manager
	tags         []string
}

// NewEngine creates a new synchronization engine
func NewEngine(
	mistClient *mist.Client,
	netboxClient *netbox.Client,
	cfg *config.Config,
	mapping *models.MappingConfig,
	stateMgr *state.Manager,
) *Engine {
	tags := []string{}
	if cfg.Sync.ApplySourceTag {
		tags = append(tags, cfg.Sync.SourceTagName)
	}

	return &Engine{
		mistClient:   mistClient,
		netboxClient: netboxClient,
		config:       cfg,
		mapping:      mapping,
		state:        stateMgr,
		tags:         tags,
	}
}

// Run executes the full synchronization
func (e *Engine) Run() error {
	syncID := fmt.Sprintf("sync-%d", time.Now().Unix())
	logger.WithField("sync_id", syncID).Info("Starting synchronization")

	e.state.StartSync(syncID)
	defer func() {
		if err := e.state.Save(); err != nil {
			logger.Errorf("Failed to save state: %v", err)
		}
	}()

	var syncErr error

	// Phase 1: Ensure NetBox infrastructure exists
	if err := e.ensureInfrastructure(); err != nil {
		e.state.RecordError("infrastructure", "", "ensure", err)
		logger.Errorf("Failed to ensure infrastructure: %v", err)
		syncErr = err
	}

	// Phase 2: Fetch data from Mist once (cache for all phases)
	logger.Info("Fetching data from Mist API")
	mistSites, err := e.mistClient.GetSites()
	if err != nil {
		logger.Errorf("Failed to fetch sites from Mist: %v", err)
		syncErr = err
	}
	e.state.RecordAPICall("mist")

	var mistDevices []*models.Device
	if e.config.Sync.Enabled.DeviceTypes || e.config.Sync.Enabled.Devices {
		mistDevices, err = e.mistClient.GetDevicesWithSites(mistSites)
		if err != nil {
			logger.Errorf("Failed to fetch devices from Mist: %v", err)
			syncErr = err
		}
		e.state.RecordAPICall("mist")
	}

	// Phase 3: Sync sites
	if e.config.Sync.Enabled.Sites && mistSites != nil {
		if err := e.syncSitesWithData(mistSites); err != nil {
			logger.Errorf("Site sync failed: %v", err)
			syncErr = err
		}
	}

	// Phase 4: Sync device types and manufacturers
	if e.config.Sync.Enabled.DeviceTypes && mistDevices != nil {
		if err := e.syncDeviceTypesWithData(mistDevices); err != nil {
			logger.Errorf("Device type sync failed: %v", err)
			syncErr = err
		}
	}

	// Phase 5: Sync devices
	if e.config.Sync.Enabled.Devices && mistDevices != nil && mistSites != nil {
		if err := e.syncDevicesWithData(mistDevices, mistSites); err != nil {
			logger.Errorf("Device sync failed: %v", err)
			syncErr = err
		}
	}

	// Phase 6: Sync interfaces and cables
	if e.config.Sync.Enabled.Interfaces && mistSites != nil {
		if err := e.syncInterfacesWithData(mistSites); err != nil {
			logger.Errorf("Interface sync failed: %v", err)
			syncErr = err
		}
	}

	// Phase 7: Sync VLANs
	if e.config.Sync.Enabled.VLANs {
		if err := e.syncVLANsWithData(); err != nil {
			logger.Errorf("VLAN sync failed: %v", err)
			syncErr = err
		}
	}

	// Phase 8: Sync wireless LANs
	if e.config.Sync.Enabled.Wireless {
		if err := e.syncWirelessLANsWithData(); err != nil {
			logger.Errorf("Wireless LAN sync failed: %v", err)
			syncErr = err
		}
	}

	// Mark sync as complete
	e.state.EndSync(syncErr == nil)

	// Print summary
	summary := e.state.GetSummary()
	fmt.Println("\n" + summary)

	if syncErr != nil {
		return fmt.Errorf("synchronization completed with errors")
	}

	logger.Info("Synchronization completed successfully")
	return nil
}

// ensureInfrastructure ensures regions, site groups, and custom fields exist
func (e *Engine) ensureInfrastructure() error {
	logger.Info("Ensuring NetBox infrastructure")

	// Create regions (log errors but continue)
	if err := e.netboxClient.EnsureRegionHierarchy(e.mapping.Regions); err != nil {
		logger.Warnf("Failed to create all regions: %v (continuing with sync)", err)
		e.state.RecordError("infrastructure", "regions", "ensure", err)
	}

	// Create site groups (log errors but continue)
	if err := e.netboxClient.EnsureSiteGroupHierarchy(e.mapping.SiteGroups); err != nil {
		logger.Warnf("Failed to create all site groups: %v (continuing with sync)", err)
		e.state.RecordError("infrastructure", "site_groups", "ensure", err)
	}

	// Create tags - critical for sync
	if e.config.Sync.ApplySourceTag {
		if err := e.netboxClient.EnsureTag(e.config.Sync.SourceTagName); err != nil {
			e.state.RecordError("infrastructure", "tags", "ensure", err)
			return fmt.Errorf("failed to create source tag '%s': %w", e.config.Sync.SourceTagName, err)
		}
	}

	// Create custom fields - critical for sync
	if e.config.Sync.CreateCustomFields {
		if err := e.netboxClient.EnsureCustomFields(e.mapping); err != nil {
			e.state.RecordError("infrastructure", "custom_fields", "ensure", err)
			return fmt.Errorf("failed to create custom fields: %w", err)
		}
	}

	// Create device roles - critical for device sync
	if e.config.Sync.Enabled.Devices {
		if err := e.netboxClient.EnsureDeviceRoles(); err != nil {
			e.state.RecordError("infrastructure", "device_roles", "ensure", err)
			return fmt.Errorf("failed to create device roles: %w", err)
		}
	}

	logger.Info("Infrastructure setup complete")
	return nil
}

// computeChecksum computes a SHA256 checksum of an object for change detection
func computeChecksum(obj interface{}) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// testDiscoveredSwitches fetches and logs discovered switches data for analysis
func (e *Engine) testDiscoveredSwitches(sites []*models.Site) {
	logger.Info("=== DISCOVERED SWITCHES TEST ===")

	for _, site := range sites {
		logger.WithField("site", site.Name).Info("Fetching discovered switches for site")

		switches, err := e.mistClient.GetDiscoveredSwitches(site.SourceID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site":  site.Name,
				"error": err,
			}).Warn("Failed to fetch discovered switches")
			continue
		}
		e.state.RecordAPICall("mist")

		if len(switches) == 0 {
			logger.WithField("site", site.Name).Info("No discovered switches for this site")
			continue
		}

		// Pretty print the JSON data
		for i, switchData := range switches {
			prettyJSON, err := json.MarshalIndent(switchData, "", "  ")
			if err != nil {
				logger.WithField("error", err).Warn("Failed to marshal switch data")
				continue
			}

			logger.WithFields(map[string]interface{}{
				"site":        site.Name,
				"switch_num":  i + 1,
				"total":       len(switches),
			}).Info("Discovered switch data:")

			// Log the pretty JSON
			fmt.Printf("\n=== Switch %d/%d in site %s ===\n%s\n\n", i+1, len(switches), site.Name, string(prettyJSON))
		}
	}

	logger.Info("=== END DISCOVERED SWITCHES TEST ===")
}

// testNetworksAndWLANs fetches and logs networks and WLAN data for analysis
func (e *Engine) testNetworksAndWLANs(sites []*models.Site) {
	logger.Info("=== NETWORKS AND WLANs TEST ===")

	// Fetch org-level networks (VLANs)
	logger.Info("Fetching organization networks (VLANs)")
	networks, err := e.mistClient.GetOrgNetworks()
	if err != nil {
		logger.WithField("error", err).Warn("Failed to fetch org networks")
	} else {
		e.state.RecordAPICall("mist")

		if len(networks) == 0 {
			logger.Info("No organization networks found")
		} else {
			for i, network := range networks {
				prettyJSON, err := json.MarshalIndent(network, "", "  ")
				if err != nil {
					logger.WithField("error", err).Warn("Failed to marshal network data")
					continue
				}

				fmt.Printf("\n=== Network %d/%d ===\n%s\n\n", i+1, len(networks), string(prettyJSON))
			}
		}
	}

	// Fetch org-level WLANs (templates)
	logger.Info("Fetching organization WLANs (SSID templates)")
	orgWLANs, err := e.mistClient.GetOrgWLANs()
	if err != nil {
		logger.WithField("error", err).Warn("Failed to fetch org WLANs")
	} else {
		e.state.RecordAPICall("mist")

		if len(orgWLANs) == 0 {
			logger.Info("No organization WLANs found")
		} else {
			logger.WithField("count", len(orgWLANs)).Info("Found organization WLANs")

			for i, wlan := range orgWLANs {
				prettyJSON, err := json.MarshalIndent(wlan, "", "  ")
				if err != nil {
					logger.WithField("error", err).Warn("Failed to marshal WLAN data")
					continue
				}

				fmt.Printf("\n=== Org WLAN %d/%d ===\n%s\n\n", i+1, len(orgWLANs), string(prettyJSON))
			}
		}
	}

	// Fetch site WLANs
	for _, site := range sites {
		logger.WithField("site", site.Name).Info("Fetching WLANs for site")

		wlans, err := e.mistClient.GetSiteWLANs(site.SourceID)
		if err != nil {
			logger.WithFields(map[string]interface{}{
				"site":  site.Name,
				"error": err,
			}).Warn("Failed to fetch site WLANs")
			continue
		}
		e.state.RecordAPICall("mist")

		if len(wlans) == 0 {
			logger.WithField("site", site.Name).Info("No WLANs for this site")
			continue
		}

		logger.WithFields(map[string]interface{}{
			"site":  site.Name,
			"count": len(wlans),
		}).Info("Found WLANs")

		for i, wlan := range wlans {
			prettyJSON, err := json.MarshalIndent(wlan, "", "  ")
			if err != nil {
				logger.WithField("error", err).Warn("Failed to marshal WLAN data")
				continue
			}

			fmt.Printf("\n=== WLAN %d/%d in site %s ===\n%s\n\n", i+1, len(wlans), site.Name, string(prettyJSON))
		}
	}

	logger.Info("=== END NETWORKS AND WLANs TEST ===")
}
