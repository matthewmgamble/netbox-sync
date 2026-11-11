package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/logger"
	"github.com/matthewmgamble/netbox-sync/internal/models"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureCustomFields ensures custom fields exist for sites and devices
func (c *Client) EnsureCustomFields(mapping *models.MappingConfig) error {
	logger.Info("Ensuring custom fields exist in NetBox")

	// Create site custom fields
	for _, cf := range mapping.CustomFields.Sites {
		if err := c.ensureCustomField(cf, []string{"dcim.site"}); err != nil {
			logger.WithFields(map[string]interface{}{
				"field": cf.Name,
				"error": err,
			}).Warn("Failed to create custom field for sites")
		}
	}

	// Create device custom fields
	for _, cf := range mapping.CustomFields.Devices {
		if err := c.ensureCustomField(cf, []string{"dcim.device"}); err != nil {
			logger.WithFields(map[string]interface{}{
				"field": cf.Name,
				"error": err,
			}).Warn("Failed to create custom field for devices")
		}
	}

	// Create interface custom fields
	for _, cf := range mapping.CustomFields.Interfaces {
		if err := c.ensureCustomField(cf, []string{"dcim.interface"}); err != nil {
			logger.WithFields(map[string]interface{}{
				"field": cf.Name,
				"error": err,
			}).Warn("Failed to create custom field for interfaces")
		}
	}

	return nil
}

// ensureCustomField ensures a single custom field exists with the correct object types
func (c *Client) ensureCustomField(cfDef models.CustomFieldDef, contentTypes []string) error {
	c.rateLimit()

	// Check if custom field exists
	req := c.client.ExtrasAPI.ExtrasCustomFieldsList(c.ctx).Name([]string{cfDef.Name})
	result, _, err := req.Execute()
	if err != nil {
		return fmt.Errorf("failed to query custom fields: %w", err)
	}

	if result.GetCount() > 0 {
		// Field exists - check if it has the correct object types
		results := result.GetResults()
		existingField := results[0]
		existingTypes := existingField.GetObjectTypes()

		// Check if all required content types are present
		missingTypes := []string{}
		for _, requiredType := range contentTypes {
			found := false
			for _, existingType := range existingTypes {
				if existingType == requiredType {
					found = true
					break
				}
			}
			if !found {
				missingTypes = append(missingTypes, requiredType)
			}
		}

		// If no missing types, we're good
		if len(missingTypes) == 0 {
			if c.logAPI {
				logger.WithField("custom_field", cfDef.Name).Debug("Custom field already exists with correct object types")
			}
			return nil
		}

		// Update the field to add missing object types
		updatedTypes := append(existingTypes, missingTypes...)
		patchReq := netbox.NewPatchedWritableCustomFieldRequestWithDefaults()
		patchReq.SetObjectTypes(updatedTypes)

		fieldID := existingField.GetId()
		_, _, err = c.client.ExtrasAPI.ExtrasCustomFieldsPartialUpdate(c.ctx, int32(fieldID)).PatchedWritableCustomFieldRequest(*patchReq).Execute()
		if err != nil {
			return fmt.Errorf("failed to update custom field object types: %w", err)
		}

		logger.WithFields(map[string]interface{}{
			"custom_field":   cfDef.Name,
			"added_types":    missingTypes,
			"all_types":      updatedTypes,
		}).Info("Updated custom field with additional object types")
		return nil
	}

	// Create custom field
	cfReq := netbox.NewWritableCustomFieldRequestWithDefaults()
	cfReq.SetName(cfDef.Name)
	cfReq.SetObjectTypes(contentTypes)
	cfReq.SetLabel(cfDef.Label)

	cfType := mapCustomFieldType(cfDef.Type)
	cfReq.SetType(cfType)

	if cfDef.Description != "" {
		cfReq.SetDescription(cfDef.Description)
	}

	if cfDef.Required {
		cfReq.SetRequired(true)
	}

	if c.logAPI {
		logger.WithField("custom_field", cfDef.Name).Debug("Creating custom field")
	}

	_, _, err = c.client.ExtrasAPI.ExtrasCustomFieldsCreate(c.ctx).WritableCustomFieldRequest(*cfReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to create custom field: %w", err)
	}

	logger.WithField("custom_field", cfDef.Name).Info("Created custom field")
	return nil
}

// mapCustomFieldType maps our type names to NetBox custom field types
func mapCustomFieldType(cfType string) netbox.PatchedWritableCustomFieldRequestType {
	typeMap := map[string]netbox.PatchedWritableCustomFieldRequestType{
		"text":        netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_TEXT,
		"longtext":    netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_LONGTEXT,
		"integer":     netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_INTEGER,
		"decimal":     netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_DECIMAL,
		"boolean":     netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_BOOLEAN,
		"date":        netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_DATE,
		"datetime":    netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_DATETIME,
		"url":         netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_URL,
		"json":        netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_JSON,
		"select":      netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_SELECT,
		"multiselect": netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_MULTISELECT,
	}

	if val, ok := typeMap[cfType]; ok {
		return val
	}

	// Default to text
	return netbox.PATCHEDWRITABLECUSTOMFIELDREQUESTTYPE_TEXT
}
