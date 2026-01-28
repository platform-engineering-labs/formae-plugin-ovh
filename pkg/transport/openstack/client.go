// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package openstack

import (
	"context"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
)

// Client wraps gophercloud clients for OpenStack services
type Client struct {
	Provider      *gophercloud.ProviderClient
	NetworkClient *gophercloud.ServiceClient
	ComputeClient *gophercloud.ServiceClient
}

// Config holds OpenStack authentication configuration
type Config struct {
	AuthURL         string
	Username        string
	Password        string
	ProjectID       string
	UserDomainName  string
	ProjectDomainID string
	Region          string
}

// ConfigFromEnv creates a Config from environment variables
func ConfigFromEnv() *Config {
	return &Config{
		AuthURL:         os.Getenv("OS_AUTH_URL"),
		Username:        os.Getenv("OS_USERNAME"),
		Password:        os.Getenv("OS_PASSWORD"),
		ProjectID:       os.Getenv("OS_PROJECT_ID"),
		UserDomainName:  getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default"),
		ProjectDomainID: getEnvOrDefault("OS_PROJECT_DOMAIN_ID", "default"),
		Region:          os.Getenv("OS_REGION_NAME"),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// NewClient creates a new OpenStack client from config
func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.AuthURL,
		Username:         cfg.Username,
		Password:         cfg.Password,
		TenantID:         cfg.ProjectID,
		DomainName:       cfg.UserDomainName,
	}

	provider, err := openstack.AuthenticatedClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	endpointOpts := gophercloud.EndpointOpts{
		Region: cfg.Region,
	}

	networkClient, err := openstack.NewNetworkV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	computeClient, err := openstack.NewComputeV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	return &Client{
		Provider:      provider,
		NetworkClient: networkClient,
		ComputeClient: computeClient,
	}, nil
}
