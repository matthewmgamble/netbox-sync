package netbox

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/logger"
	netbox "github.com/netbox-community/go-netbox/v4"
)

// Client wraps the NetBox API client
type Client struct {
	client  *netbox.APIClient
	ctx     context.Context
	logAPI  bool
	limiter *time.Ticker
}

// NewClient creates a new NetBox client
func NewClient(cfg *config.NetBoxConfig, logAPI bool) (*Client, error) {
	// Create HTTP client with optional SSL verification
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	if !cfg.VerifySSL {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		logger.Warn("SSL verification disabled for NetBox")
	}

	// Parse URL to extract host and scheme
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid NetBox URL: %w", err)
	}

	// Create NetBox client configuration
	netboxConfig := netbox.NewConfiguration()
	netboxConfig.Host = parsedURL.Host
	netboxConfig.Scheme = parsedURL.Scheme
	if netboxConfig.Scheme == "" {
		netboxConfig.Scheme = "https"
	}
	netboxConfig.HTTPClient = httpClient
	netboxConfig.AddDefaultHeader("Authorization", fmt.Sprintf("Token %s", cfg.Token))

	// Create client
	client := netbox.NewAPIClient(netboxConfig)

	// Test connection by getting a simple endpoint
	ctx := context.Background()
	_, _, err = client.StatusAPI.StatusRetrieve(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NetBox: %w", err)
	}

	logger.WithField("url", cfg.URL).Info("Connected to NetBox API")

	// Set up rate limiter if configured
	var limiter *time.Ticker
	if cfg.RateLimit > 0 {
		// Convert requests per minute to duration between requests
		interval := time.Minute / time.Duration(cfg.RateLimit)
		limiter = time.NewTicker(interval)
	}

	return &Client{
		client:  client,
		ctx:     ctx,
		logAPI:  logAPI,
		limiter: limiter,
	}, nil
}

// Close cleans up resources
func (c *Client) Close() {
	if c.limiter != nil {
		c.limiter.Stop()
	}
}

// rateLimit applies rate limiting if configured
func (c *Client) rateLimit() {
	if c.limiter != nil {
		<-c.limiter.C
	}
}

// GetContext returns the client's context
func (c *Client) GetContext() context.Context {
	return c.ctx
}

// GetClient returns the underlying NetBox API client
func (c *Client) GetClient() *netbox.APIClient {
	return c.client
}
