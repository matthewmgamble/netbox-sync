package models

import "time"

// Device represents a vendor-agnostic network device
type Device struct {
	// Source identifiers
	SourceID   string // Vendor-specific ID (e.g., Mist device UUID)
	SourceType string // "mist", "meraki", etc.

	// Basic information
	Name         string
	Description  string
	Serial       string
	AssetTag     string

	// Type and classification
	DeviceType   string // Device type name
	Manufacturer string // Manufacturer name
	Model        string // Model number/name
	Role         string // Device role (e.g., "Access Point", "Switch")
	Platform     string // Platform/OS (e.g., "Mist 0.11.x")

	// Site assignment
	Site         string // Site name this device belongs to

	// Status
	Status       string // "active", "planned", "failed", etc.

	// Hardware details
	Firmware     string
	MAC          string

	// Tags
	Tags         []string

	// Custom fields
	CustomFields map[string]interface{}

	// Metadata
	LastSynced time.Time
	Checksum   string // For change detection
}

// DeviceType represents a vendor-agnostic device type
type DeviceType struct {
	Manufacturer string
	Model        string
	Slug         string
	PartNumber   string
	Height       float64 // U height (1.0 for 1U, 0.0 for non-rack devices)
	IsFullDepth  bool
	Description  string
}

// Manufacturer represents a hardware manufacturer
type Manufacturer struct {
	Name        string
	Slug        string
	Description string
}

// DeviceRole represents a device role/function
type DeviceRole struct {
	Name        string
	Slug        string
	Color       string // Hex color code
	VMRole      bool   // Can be assigned to VMs
	Description string
}

// Platform represents an operating system/platform
type Platform struct {
	Name         string
	Slug         string
	Manufacturer string
	Description  string
}
