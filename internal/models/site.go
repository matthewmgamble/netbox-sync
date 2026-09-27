package models

import "time"

// Site represents a vendor-agnostic site/location
type Site struct {
	// Source identifiers
	SourceID   string // Vendor-specific ID (e.g., Mist site UUID)
	SourceType string // "mist", "meraki", etc.

	// Basic information
	Name        string
	Description string
	Address     string
	Timezone    string
	Latitude    *float64
	Longitude   *float64

	// NetBox-specific mappings
	Region      string // NetBox region name (hierarchical, e.g., "Canada/Ontario")
	SiteGroup   string // NetBox site group name
	Status      string // "active", "planned", "retired", etc.
	Tenant      string // Optional tenant assignment

	// Custom fields
	CustomFields map[string]interface{}

	// Metadata
	LastSynced time.Time
	Checksum   string // For change detection
}

// SiteMapping represents mapping configuration for a site
type SiteMapping struct {
	MistSiteID      string
	NetBoxSiteName  string                 // Override site name if specified
	Region          string                 // Target NetBox region
	SiteGroup       string                 // Target NetBox site group
	Status          string                 // NetBox status
	CustomFields    map[string]interface{} // Additional custom fields
}

// MappingRule represents a pattern-based mapping rule
type MappingRule struct {
	Pattern   string                 // Regex pattern to match against address/name
	Region    string                 // Target region if matched
	SiteGroup string                 // Target site group if matched
	Status    string                 // Target status if matched
	Priority  int                    // Lower number = higher priority
	CustomFields map[string]interface{}
}
