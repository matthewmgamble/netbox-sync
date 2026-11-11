package main

import (
	"fmt"
	"os"
	"time"

	"github.com/matthewmgamble/netbox-sync/internal/client/netbox"
	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
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
	nbClient, err := netbox.NewClient(&cfg.NetBox, cfg.Logging.LogAPICalls)
	if err != nil {
		logger.Fatalf("Failed to create NetBox client: %v", err)
	}
	defer nbClient.Close()

	// Create a test device with custom fields
	testDevice := &models.Device{
		Name:         "TEST-CUSTOM-FIELDS-001",
		Serial:       "TEST-SERIAL-001",
		Site:         "CTR_0113",
		Role:         "Access Point",
		Manufacturer: "Juniper",
		Model:        "AP33",
		Status:       "active",
		CustomFields: map[string]interface{}{
			"mist_device_id": "test-device-uuid-123",
			"mist_site_id":   "test-site-uuid-456",
			"mist_model":     "AP33",
			"last_synced":    time.Now().Format("2006-01-02"),
		},
	}

	logger.Info("Testing device creation with custom fields...")
	logger.WithFields(map[string]interface{}{
		"device":        testDevice.Name,
		"custom_fields": testDevice.CustomFields,
	}).Info("Creating test device")

	result, err := nbClient.CreateDevice(testDevice, []string{"mist-synced", "test"})
	if err != nil {
		logger.Fatalf("Device creation failed: %v", err)
	}

	logger.WithFields(map[string]interface{}{
		"device_id":   result.GetId(),
		"device_name": result.GetName(),
	}).Info("✓ Device created successfully with custom fields!")

	// Verify custom fields were set
	cfMap := result.GetCustomFields()
	logger.WithFields(map[string]interface{}{
		"custom_fields": cfMap,
	}).Info("Custom fields on created device")

	fmt.Printf("\n✓ SUCCESS! Device created with ID %d\n", result.GetId())
	fmt.Printf("Custom fields were successfully set:\n")
	for k, v := range cfMap {
		fmt.Printf("  - %s: %v\n", k, v)
	}
}
