package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/int13consulting/netbox-sync/internal/client/mist"
	"github.com/int13consulting/netbox-sync/internal/client/netbox"
	"github.com/int13consulting/netbox-sync/internal/config"
	"github.com/int13consulting/netbox-sync/internal/logger"
	"github.com/int13consulting/netbox-sync/internal/state"
	"github.com/int13consulting/netbox-sync/internal/sync"
)

const version = "0.1.0"

func main() {
	// Command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("netbox-sync version %s\n", version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(&cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("version", version).Info("NetBox Sync starting")

	// Load mapping configuration
	mappingCfg, err := config.LoadMapping(cfg.Mapping.File)
	if err != nil {
		logger.Fatalf("Failed to load mapping configuration: %v", err)
	}

	logger.Info("Configuration loaded successfully")

	// Initialize Mist client
	mistClient, err := mist.NewClient(&cfg.Mist, &cfg.Filter, cfg.Logging.LogAPICalls)
	if err != nil {
		logger.Fatalf("Failed to initialize Mist client: %v", err)
	}

	// Get organization name for logging
	orgName, err := mistClient.GetOrganizationInfo()
	if err != nil {
		logger.Warnf("Failed to get organization name: %v", err)
	} else {
		logger.WithField("organization", orgName).Info("Connected to Mist organization")
	}

	// Initialize NetBox client
	netboxClient, err := netbox.NewClient(&cfg.NetBox, cfg.Logging.LogAPICalls)
	if err != nil {
		logger.Fatalf("Failed to initialize NetBox client: %v", err)
	}
	defer netboxClient.Close()

	// Initialize state manager
	stateMgr := state.NewManager(cfg.State.File)
	if err := stateMgr.Load(); err != nil {
		logger.Warnf("Failed to load previous state: %v", err)
	}

	// Create sync engine
	engine := sync.NewEngine(mistClient, netboxClient, cfg, mappingCfg, stateMgr)

	// Run synchronization
	if err := engine.Run(); err != nil {
		logger.Errorf("Synchronization failed: %v", err)
		os.Exit(1)
	}

	logger.Info("NetBox Sync completed successfully")
}
