package main

import (
	"fmt"
	"os"

	"github.com/int13consulting/netbox-sync/internal/client/netbox"
	"github.com/int13consulting/netbox-sync/internal/config"
	"github.com/int13consulting/netbox-sync/internal/logger"
)

func main() {
	// Load config
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(&cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Create NetBox client
	nbClient, err := netbox.NewClient(&cfg.NetBox, false)
	if err != nil {
		logger.Fatalf("Failed to create NetBox client: %v", err)
	}
	defer nbClient.Close()

	// Check devices that were created in the test run
	serials := []string{
		"A111022020520", // CTR-0112-MDF-AP-I-001
		"A111022020524", // CTR-0112-MDF-AP-I-007
		"A114721020904", // CTR-0112-MDF-AP-I-019
	}

	for _, serial := range serials {
		device, err := nbClient.FindDeviceBySerial(serial)
		if err != nil {
			logger.Warnf("Failed to find device %s: %v", serial, err)
			continue
		}

		if device == nil {
			logger.Warnf("Device %s not found", serial)
			continue
		}

		fmt.Printf("\n=== Device %s (ID %d) ===\n", device.GetName(), device.GetId())
		fmt.Printf("Serial: %s\n", device.GetSerial())

		cfMap := device.GetCustomFields()
		if len(cfMap) == 0 {
			fmt.Printf("Custom Fields: (none)\n")
		} else {
			fmt.Printf("Custom Fields:\n")
			for k, v := range cfMap {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}
}
