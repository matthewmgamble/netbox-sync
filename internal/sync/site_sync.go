package sync

import (
	"strings"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	"github.com/matthewmgamble/netbox-sync/internal/state"
)

// syncSitesWithData synchronizes sites from Mist to NetBox using cached data
func (e *Engine) syncSitesWithData(mistSites []*models.Site) error {
	logger.Info("Starting site synchronization")

	stats := state.ObjectStats{}

	stats.Discovered = len(mistSites)
	logger.WithField("count", len(mistSites)).Info("Discovered sites from Mist")

	// TESTING: Limit to first 20 sites for iteration
	testLimit := 20
	if len(mistSites) > testLimit {
		logger.Warnf("TESTING MODE: Processing only first %d sites (total: %d)", testLimit, len(mistSites))
		mistSites = mistSites[:testLimit]
	}

	// Process each site
	for _, site := range mistSites {
		// Apply filters
		if e.shouldSkipSite(site.Name) {
			logger.WithField("site", site.Name).Debug("Skipping filtered site")
			continue
		}

		// Normalize: Apply mapping rules
		if err := config.ApplyMapping(site, e.mapping); err != nil {
			logger.WithFields(map[string]interface{}{
				"site":  site.Name,
				"error": err,
			}).Warn("Failed to apply mapping, using defaults")
			e.state.RecordError("site", site.SourceID, "apply_mapping", err)
			stats.Errors++
			continue
		}

		// Compute checksum for change detection
		checksum, err := computeChecksum(site)
		if err != nil {
			logger.WithField("site", site.Name).Warnf("Failed to compute checksum: %v", err)
		}

		// Compare: Check if site exists and has changed
		existingSite, err := e.netboxClient.FindSiteByName(site.Name)
		if err != nil {
			logger.WithField("site", site.Name).Warnf("Failed to check for existing site: %v", err)
			e.state.RecordError("site", site.SourceID, "find", err)
			stats.Errors++
			continue
		}

		e.state.RecordAPICall("netbox")

		if existingSite == nil {
			// Apply: Create new site
			created, err := e.netboxClient.CreateSite(site, e.tags)
			if err != nil {
				logger.WithField("site", site.Name).Errorf("Failed to create site: %v", err)
				e.state.RecordError("site", site.SourceID, "create", err)
				stats.Errors++
				continue
			}

			e.state.RecordAPICall("netbox")
			e.state.TrackObject("sites", site.SourceID, int64(created.GetId()), checksum)
			stats.Created++
		} else {
			// Check if update needed
			if e.state.IsChanged("sites", site.SourceID, checksum) {
				// Apply: Update existing site
				_, err := e.netboxClient.UpdateSite(int64(existingSite.GetId()), site, e.tags)
				if err != nil {
					logger.WithField("site", site.Name).Errorf("Failed to update site: %v", err)
					e.state.RecordError("site", site.SourceID, "update", err)
					stats.Errors++
					continue
				}

				e.state.RecordAPICall("netbox")
				e.state.TrackObject("sites", site.SourceID, int64(existingSite.GetId()), checksum)
				stats.Updated++
			} else {
				stats.Unchanged++
				if logger.Get().Level >= 4 { // Debug level
					logger.WithField("site", site.Name).Debug("Site unchanged")
				}
			}
		}
	}

	e.state.RecordObjectStats("sites", stats)
	logger.WithFields(map[string]interface{}{
		"created":   stats.Created,
		"updated":   stats.Updated,
		"unchanged": stats.Unchanged,
		"errors":    stats.Errors,
	}).Info("Site synchronization complete")

	return nil
}

// shouldSkipSite checks if a site should be skipped based on filters
func (e *Engine) shouldSkipSite(siteName string) bool {
	for _, excluded := range e.config.Filter.ExcludeSites {
		if strings.EqualFold(siteName, excluded) {
			return true
		}
	}
	return false
}
