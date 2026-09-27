package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete application configuration
type Config struct {
	Mist    MistConfig    `mapstructure:"mist"`
	NetBox  NetBoxConfig  `mapstructure:"netbox"`
	Sync    SyncConfig    `mapstructure:"sync"`
	Filter  FilterConfig  `mapstructure:"filter"`
	State   StateConfig   `mapstructure:"state"`
	Logging LoggingConfig `mapstructure:"logging"`
	Mapping MappingRef    `mapstructure:"mapping"`
}

// MistConfig holds Mist API configuration
type MistConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	APIToken       string `mapstructure:"api_token"`
	OrganizationID string `mapstructure:"organization_id"`
}

// NetBoxConfig holds NetBox API configuration
type NetBoxConfig struct {
	URL       string        `mapstructure:"url"`
	Token     string        `mapstructure:"token"`
	VerifySSL bool          `mapstructure:"verify_ssl"`
	Timeout   time.Duration `mapstructure:"timeout"`
	RateLimit int           `mapstructure:"rate_limit"`
}

// SyncConfig controls what gets synchronized
type SyncConfig struct {
	Enabled struct {
		Sites        bool `mapstructure:"sites"`
		DeviceTypes  bool `mapstructure:"device_types"`
		Devices      bool `mapstructure:"devices"`
		Interfaces   bool `mapstructure:"interfaces"`
		VLANs        bool `mapstructure:"vlans"`
		IPAddresses  bool `mapstructure:"ip_addresses"`
		Wireless     bool `mapstructure:"wireless"`
	} `mapstructure:"enabled"`

	DeleteOrphans       bool   `mapstructure:"delete_orphans"`
	ApplySourceTag      bool   `mapstructure:"apply_source_tag"`
	SourceTagName       string `mapstructure:"source_tag_name"`
	CreateCustomFields  bool   `mapstructure:"create_custom_fields"`
	CustomFieldPrefix   string `mapstructure:"custom_field_prefix"`
}

// FilterConfig controls what gets filtered out
type FilterConfig struct {
	IncludeSites        []string `mapstructure:"include_sites"`
	ExcludeSites        []string `mapstructure:"exclude_sites"`
	ExcludeDeviceModels []string `mapstructure:"exclude_device_models"`
}

// StateConfig controls state tracking
type StateConfig struct {
	File          string `mapstructure:"file"`
	RetentionDays int    `mapstructure:"retention_days"`
}

// LoggingConfig controls logging behavior
type LoggingConfig struct {
	Level         string `mapstructure:"level"`
	Format        string `mapstructure:"format"`
	Output        string `mapstructure:"output"`
	LogAPICalls   bool   `mapstructure:"log_api_calls"`
}

// MappingRef points to the mapping configuration file
type MappingRef struct {
	File string `mapstructure:"file"`
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Allow environment variable overrides
	v.AutomaticEnv()

	// Read the config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in config values
	expandEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Mist defaults
	v.SetDefault("mist.base_url", "https://api.mist.com")

	// NetBox defaults
	v.SetDefault("netbox.verify_ssl", true)
	v.SetDefault("netbox.timeout", "30s")
	v.SetDefault("netbox.rate_limit", 100)

	// Sync defaults
	v.SetDefault("sync.enabled.sites", true)
	v.SetDefault("sync.enabled.device_types", true)
	v.SetDefault("sync.enabled.devices", true)
	v.SetDefault("sync.delete_orphans", false)
	v.SetDefault("sync.apply_source_tag", true)
	v.SetDefault("sync.source_tag_name", "mist-synced")
	v.SetDefault("sync.create_custom_fields", true)
	v.SetDefault("sync.custom_field_prefix", "mist_")

	// State defaults
	v.SetDefault("state.file", "./data/sync-state.json")
	v.SetDefault("state.retention_days", 30)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.log_api_calls", false)

	// Mapping defaults
	v.SetDefault("mapping.file", "./configs/mapping.yaml")
}

// expandEnvVars expands environment variables in all string config values
func expandEnvVars(v *viper.Viper) {
	// Expand environment variables in sensitive fields
	if token := v.GetString("mist.api_token"); token != "" {
		v.Set("mist.api_token", os.ExpandEnv(token))
	}
	if token := v.GetString("netbox.token"); token != "" {
		v.Set("netbox.token", os.ExpandEnv(token))
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Mist config
	if c.Mist.APIToken == "" {
		return fmt.Errorf("mist.api_token is required")
	}
	if c.Mist.OrganizationID == "" {
		return fmt.Errorf("mist.organization_id is required")
	}

	// Validate NetBox config
	if c.NetBox.URL == "" {
		return fmt.Errorf("netbox.url is required")
	}
	if c.NetBox.Token == "" {
		return fmt.Errorf("netbox.token is required")
	}

	// Validate state file path
	if c.State.File == "" {
		return fmt.Errorf("state.file is required")
	}

	// Validate logging config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging.level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid logging.format: %s (must be json or text)", c.Logging.Format)
	}

	// Validate mapping file exists
	if c.Mapping.File == "" {
		return fmt.Errorf("mapping.file is required")
	}

	return nil
}
