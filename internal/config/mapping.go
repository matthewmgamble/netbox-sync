package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/matthewmgamble/netbox-sync/internal/models"
	"gopkg.in/yaml.v3"
)

// LoadMapping loads the mapping configuration from a YAML file
func LoadMapping(mappingPath string) (*models.MappingConfig, error) {
	data, err := os.ReadFile(mappingPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read mapping file: %w", err)
	}

	var mapping models.MappingConfig
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse mapping file: %w", err)
	}

	// Validate mapping configuration
	if err := ValidateMapping(&mapping); err != nil {
		return nil, fmt.Errorf("invalid mapping configuration: %w", err)
	}

	return &mapping, nil
}

// ValidateMapping validates the mapping configuration
func ValidateMapping(m *models.MappingConfig) error {
	// Validate rules have valid regex patterns
	for i, rule := range m.Rules {
		if rule.Pattern == "" {
			return fmt.Errorf("rule %d: pattern is required", i)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("rule %d: invalid pattern %q: %w", i, rule.Pattern, err)
		}
	}

	// Validate region hierarchy
	if err := validateRegionHierarchy(m.Regions); err != nil {
		return fmt.Errorf("invalid region hierarchy: %w", err)
	}

	// Validate site group hierarchy
	if err := validateSiteGroupHierarchy(m.SiteGroups); err != nil {
		return fmt.Errorf("invalid site group hierarchy: %w", err)
	}

	return nil
}

// validateRegionHierarchy validates region definitions
func validateRegionHierarchy(regions []models.RegionDef) error {
	seen := make(map[string]bool)
	var validate func([]models.RegionDef, string) error

	validate = func(regions []models.RegionDef, parent string) error {
		for _, region := range regions {
			if region.Name == "" {
				return fmt.Errorf("region name is required")
			}
			if region.Slug == "" {
				return fmt.Errorf("region %q: slug is required", region.Name)
			}

			// Check for duplicates
			fullPath := region.Name
			if parent != "" {
				fullPath = parent + "/" + region.Name
			}
			if seen[fullPath] {
				return fmt.Errorf("duplicate region: %s", fullPath)
			}
			seen[fullPath] = true

			// Validate children
			if err := validate(region.Children, fullPath); err != nil {
				return err
			}
		}
		return nil
	}

	return validate(regions, "")
}

// validateSiteGroupHierarchy validates site group definitions
func validateSiteGroupHierarchy(groups []models.SiteGroupDef) error {
	seen := make(map[string]bool)
	var validate func([]models.SiteGroupDef, string) error

	validate = func(groups []models.SiteGroupDef, parent string) error {
		for _, group := range groups {
			if group.Name == "" {
				return fmt.Errorf("site group name is required")
			}
			if group.Slug == "" {
				return fmt.Errorf("site group %q: slug is required", group.Name)
			}

			// Check for duplicates
			fullPath := group.Name
			if parent != "" {
				fullPath = parent + "/" + group.Name
			}
			if seen[fullPath] {
				return fmt.Errorf("duplicate site group: %s", fullPath)
			}
			seen[fullPath] = true

			// Validate children
			if err := validate(group.Children, fullPath); err != nil {
				return err
			}
		}
		return nil
	}

	return validate(groups, "")
}

// ApplyMapping applies mapping rules to a site
func ApplyMapping(site *models.Site, mapping *models.MappingConfig) error {
	// Check for explicit mapping first
	if siteMapping, ok := mapping.Sites[site.SourceID]; ok {
		if siteMapping.NetBoxSiteName != "" {
			site.Name = siteMapping.NetBoxSiteName
		}
		if siteMapping.Region != "" {
			site.Region = siteMapping.Region
		}
		if siteMapping.SiteGroup != "" {
			site.SiteGroup = siteMapping.SiteGroup
		}
		if siteMapping.Status != "" {
			site.Status = siteMapping.Status
		}
		// Merge custom fields
		if site.CustomFields == nil {
			site.CustomFields = make(map[string]interface{})
		}
		for k, v := range siteMapping.CustomFields {
			site.CustomFields[k] = v
		}
		return nil
	}

	// Try pattern-based rules
	searchText := fmt.Sprintf("%s %s", site.Name, site.Address)
	for _, rule := range mapping.Rules {
		matched, err := regexp.MatchString(rule.Pattern, searchText)
		if err != nil {
			continue // Skip invalid patterns (should have been caught in validation)
		}
		if matched {
			if rule.Region != "" {
				site.Region = rule.Region
			}
			if rule.SiteGroup != "" {
				site.SiteGroup = rule.SiteGroup
			}
			if rule.Status != "" {
				site.Status = rule.Status
			}
			// Merge custom fields
			if site.CustomFields == nil {
				site.CustomFields = make(map[string]interface{})
			}
			for k, v := range rule.CustomFields {
				site.CustomFields[k] = v
			}
			return nil
		}
	}

	// Apply defaults
	if site.Region == "" && mapping.Defaults.Region != "" {
		site.Region = mapping.Defaults.Region
	}
	if site.SiteGroup == "" && mapping.Defaults.SiteGroup != "" {
		site.SiteGroup = mapping.Defaults.SiteGroup
	}
	if site.Status == "" && mapping.Defaults.Status != "" {
		site.Status = mapping.Defaults.Status
	}
	// Merge default custom fields
	if site.CustomFields == nil {
		site.CustomFields = make(map[string]interface{})
	}
	for k, v := range mapping.Defaults.CustomFields {
		if _, exists := site.CustomFields[k]; !exists {
			site.CustomFields[k] = v
		}
	}

	return nil
}

// GetRegionPath finds the full path to a region by name
func GetRegionPath(regionName string, regions []models.RegionDef) (string, *models.RegionDef, error) {
	var search func([]models.RegionDef, string) (string, *models.RegionDef)

	search = func(regions []models.RegionDef, parent string) (string, *models.RegionDef) {
		for _, region := range regions {
			fullPath := region.Name
			if parent != "" {
				fullPath = parent + "/" + region.Name
			}

			if region.Name == regionName || fullPath == regionName {
				return fullPath, &region
			}

			// Search children
			if path, found := search(region.Children, fullPath); found != nil {
				return path, found
			}
		}
		return "", nil
	}

	path, found := search(regions, "")
	if found == nil {
		return "", nil, fmt.Errorf("region not found: %s", regionName)
	}
	return path, found, nil
}

// GetSiteGroupPath finds the full path to a site group by name
func GetSiteGroupPath(groupName string, groups []models.SiteGroupDef) (string, *models.SiteGroupDef, error) {
	var search func([]models.SiteGroupDef, string) (string, *models.SiteGroupDef)

	search = func(groups []models.SiteGroupDef, parent string) (string, *models.SiteGroupDef) {
		for _, group := range groups {
			fullPath := group.Name
			if parent != "" {
				fullPath = parent + "/" + group.Name
			}

			if group.Name == groupName || fullPath == groupName {
				return fullPath, &group
			}

			// Search children
			if path, found := search(group.Children, fullPath); found != nil {
				return path, found
			}
		}
		return "", nil
	}

	path, found := search(groups, "")
	if found == nil {
		return "", nil, fmt.Errorf("site group not found: %s", groupName)
	}
	return path, found, nil
}

// NormalizeSlug creates a NetBox-compatible slug from a name
func NormalizeSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and special characters with hyphens
	slug = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(slug, "-")

	// Remove consecutive hyphens
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	// Limit length to 100 characters
	if len(slug) > 100 {
		slug = slug[:100]
	}

	return slug
}
