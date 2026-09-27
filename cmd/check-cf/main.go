package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/int13consulting/netbox-sync/internal/config"
	netbox "github.com/netbox-community/go-netbox/v4"
)

func main() {
	// Load config
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create NetBox client
	nbConfig := netbox.NewConfiguration()
	nbConfig.Servers = netbox.ServerConfigurations{
		{URL: cfg.NetBox.URL},
	}
	nbConfig.AddDefaultHeader("Authorization", "Token "+cfg.NetBox.Token)

	// Disable SSL verification if configured
	if !cfg.NetBox.VerifySSL {
		nbConfig.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	client := netbox.NewAPIClient(nbConfig)
	ctx := context.Background()

	// Query all custom fields
	fmt.Println("Fetching all custom fields from NetBox...")
	resp, _, err := client.ExtrasAPI.ExtrasCustomFieldsList(ctx).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query custom fields: %v\n", err)
		os.Exit(1)
	}

	results := resp.GetResults()
	fmt.Printf("\nFound %d custom fields:\n\n", len(results))

	// Group by object type
	deviceFields := []netbox.CustomField{}
	siteFields := []netbox.CustomField{}
	otherFields := []netbox.CustomField{}

	for _, cf := range results {
		objectTypes := cf.GetObjectTypes()
		hasDevice := false
		hasSite := false

		for _, ot := range objectTypes {
			if ot == "dcim.device" {
				hasDevice = true
			}
			if ot == "dcim.site" {
				hasSite = true
			}
		}

		if hasDevice {
			deviceFields = append(deviceFields, cf)
		} else if hasSite {
			siteFields = append(siteFields, cf)
		} else {
			otherFields = append(otherFields, cf)
		}
	}

	fmt.Printf("=== Device Custom Fields (dcim.device) ===\n")
	if len(deviceFields) == 0 {
		fmt.Println("  (none)")
	}
	for _, cf := range deviceFields {
		fmt.Printf("  - %s (%s) - Type: %s\n", cf.GetName(), cf.GetLabel(), cf.GetType())
		fmt.Printf("    Object Types: %v\n", cf.GetObjectTypes())
		fmt.Printf("    Required: %v\n", cf.GetRequired())
	}

	fmt.Printf("\n=== Site Custom Fields (dcim.site) ===\n")
	if len(siteFields) == 0 {
		fmt.Println("  (none)")
	}
	for _, cf := range siteFields {
		fmt.Printf("  - %s (%s) - Type: %s\n", cf.GetName(), cf.GetLabel(), cf.GetType())
		fmt.Printf("    Object Types: %v\n", cf.GetObjectTypes())
		fmt.Printf("    Required: %v\n", cf.GetRequired())
	}

	fmt.Printf("\n=== Other Custom Fields ===\n")
	if len(otherFields) == 0 {
		fmt.Println("  (none)")
	}
	for _, cf := range otherFields {
		fmt.Printf("  - %s (%s) - Type: %s\n", cf.GetName(), cf.GetLabel(), cf.GetType())
		fmt.Printf("    Object Types: %v\n", cf.GetObjectTypes())
	}

	fmt.Printf("\n=== Expected Device Custom Fields ===\n")
	expected := []string{"mist_device_id", "mist_site_id", "mist_model", "last_synced", "mist_mac", "firmware_version"}
	for _, name := range expected {
		found := false
		for _, cf := range deviceFields {
			if cf.GetName() == name {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("  ✓ %s - EXISTS\n", name)
		} else {
			fmt.Printf("  ✗ %s - MISSING\n", name)
		}
	}

	fmt.Printf("\n=== Expected Site Custom Fields ===\n")
	expectedSite := []string{"mist_site_id", "mist_org_id", "last_synced"}
	for _, name := range expectedSite {
		found := false
		for _, cf := range siteFields {
			if cf.GetName() == name {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("  ✓ %s - EXISTS\n", name)
		} else {
			fmt.Printf("  ✗ %s - MISSING\n", name)
		}
	}
}
