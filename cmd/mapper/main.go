package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/matthewmgamble/netbox-sync/internal/client/mist"
	"github.com/matthewmgamble/netbox-sync/internal/client/netbox"
	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/mapper"
)

func main() {
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	mappingPath := flag.String("mapping", "./configs/mapping.yaml", "Path to mapping file")
	listenAddr := flag.String("listen", ":8080", "Listen address")
	flag.Parse()

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

	// Initialize Mist client with no site filter — mapper needs to see all sites
	noFilter := &config.FilterConfig{}
	mistClient, err := mist.NewClient(&cfg.Mist, noFilter, cfg.Logging.LogAPICalls)
	if err != nil {
		log.Fatalf("Failed to initialize Mist client: %v", err)
	}

	// Initialize NetBox client
	netboxClient, err := netbox.NewClient(&cfg.NetBox, cfg.Logging.LogAPICalls)
	if err != nil {
		log.Fatalf("Failed to initialize NetBox client: %v", err)
	}
	defer netboxClient.Close()

	// Create and start server
	srv, err := mapper.NewServer(mistClient, netboxClient, *mappingPath, *listenAddr)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Printf("Site Mapping Wizard running at http://localhost%s", *listenAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
