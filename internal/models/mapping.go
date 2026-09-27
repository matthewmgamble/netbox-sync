package models

// MappingConfig represents the complete mapping configuration
type MappingConfig struct {
	// Default mappings
	Defaults struct {
		Region    string                 `yaml:"region"`
		SiteGroup string                 `yaml:"site_group"`
		Status    string                 `yaml:"status"`
		CustomFields map[string]interface{} `yaml:"custom_fields"`
	} `yaml:"defaults"`

	// Pattern-based rules
	Rules []struct {
		Pattern      string                 `yaml:"pattern"`
		Region       string                 `yaml:"region"`
		SiteGroup    string                 `yaml:"site_group"`
		Status       string                 `yaml:"status"`
		CustomFields map[string]interface{} `yaml:"custom_fields"`
	} `yaml:"rules"`

	// Explicit site mappings
	Sites map[string]struct {
		NetBoxSiteName string                 `yaml:"netbox_site_name"`
		Region         string                 `yaml:"region"`
		SiteGroup      string                 `yaml:"site_group"`
		Status         string                 `yaml:"status"`
		CustomFields   map[string]interface{} `yaml:"custom_fields"`
	} `yaml:"sites"`

	// Region hierarchy
	Regions []RegionDef `yaml:"regions"`

	// Site group hierarchy
	SiteGroups []SiteGroupDef `yaml:"site_groups"`

	// Custom field definitions
	CustomFields struct {
		Sites   []CustomFieldDef `yaml:"sites"`
		Devices []CustomFieldDef `yaml:"devices"`
	} `yaml:"custom_fields"`
}

// RegionDef defines a region and its children
type RegionDef struct {
	Name     string      `yaml:"name"`
	Slug     string      `yaml:"slug"`
	Children []RegionDef `yaml:"children"`
}

// SiteGroupDef defines a site group and its children
type SiteGroupDef struct {
	Name     string         `yaml:"name"`
	Slug     string         `yaml:"slug"`
	Children []SiteGroupDef `yaml:"children"`
}

// CustomFieldDef defines a custom field
type CustomFieldDef struct {
	Name        string `yaml:"name"`
	Label       string `yaml:"label"`
	Type        string `yaml:"type"` // text, integer, boolean, date, url, json, select
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}
