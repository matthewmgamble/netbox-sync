package netbox

import (
	"fmt"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// EnsureTag ensures a tag exists in NetBox
func (c *Client) EnsureTag(name string) error {
	c.rateLimit()

	slug := config.NormalizeSlug(name)

	// Check if tag exists
	req := c.client.ExtrasAPI.ExtrasTagsList(c.ctx).Slug([]string{slug})
	result, _, err := req.Execute()
	if err != nil {
		return fmt.Errorf("failed to query tags: %w", err)
	}

	if result.GetCount() > 0 {
		if c.logAPI {
			logger.WithField("tag", name).Debug("Tag already exists")
		}
		return nil
	}

	// Create tag
	tagReq := netbox.NewTagRequestWithDefaults()
	tagReq.SetName(name)
	tagReq.SetSlug(slug)

	if c.logAPI {
		logger.WithField("tag", name).Debug("Creating tag")
	}

	_, _, err = c.client.ExtrasAPI.ExtrasTagsCreate(c.ctx).TagRequest(*tagReq).Execute()
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	logger.WithField("tag", name).Info("Created tag")
	return nil
}
