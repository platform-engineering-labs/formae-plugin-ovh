// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/platform-engineering-labs/formae/pkg/model"
)

// Config holds OpenStack authentication configuration
// Note: Only AuthURL and Region are stored in the target config.
// Credentials (Username, Password, ProjectID, DomainName) are always
// read from environment variables to avoid storing secrets in the database.
type Config struct {
	// Stored in target config (non-sensitive)
	AuthURL string `json:"authURL"` // https://auth.cloud.ovh.net/v3
	Region  string `json:"region"`  // GRA11, SBG5, BHS5, US-EAST-VA, etc.

	// Read from environment variables only (never stored)
	Username   string `json:"-"` // From OS_USERNAME
	Password   string `json:"-"` // From OS_PASSWORD
	ProjectID  string `json:"-"` // From OS_PROJECT_ID
	DomainName string `json:"-"` // From OS_USER_DOMAIN_NAME
}

// FromTarget extracts OVH configuration from a Target
func FromTarget(target *model.Target) (*Config, error) {
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}
	return FromTargetConfig(target.Config)
}

// FromTargetConfig extracts OVH configuration from a TargetConfig JSON
// Only AuthURL and Region are read from the target config.
// Credentials are always read from environment variables.
func FromTargetConfig(targetConfig json.RawMessage) (*Config, error) {
	var cfg Config

	// Read non-sensitive config from target
	if len(targetConfig) > 0 {
		if err := json.Unmarshal(targetConfig, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal target config: %w", err)
		}
	}

	// AuthURL and Region can fall back to environment variables
	if cfg.AuthURL == "" {
		cfg.AuthURL = os.Getenv("OS_AUTH_URL")
	}
	if cfg.Region == "" {
		cfg.Region = os.Getenv("OS_REGION_NAME")
	}

	// Credentials are ALWAYS read from environment variables (never stored)
	cfg.Username = os.Getenv("OS_USERNAME")
	cfg.Password = os.Getenv("OS_PASSWORD")
	cfg.ProjectID = os.Getenv("OS_PROJECT_ID")
	cfg.DomainName = os.Getenv("OS_USER_DOMAIN_NAME")
	if cfg.DomainName == "" {
		cfg.DomainName = "Default" // OVH default
	}

	// Validate required fields
	if cfg.AuthURL == "" {
		return nil, fmt.Errorf("authURL is required (set OS_AUTH_URL or provide in target config)")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("region is required (set OS_REGION_NAME or provide in target config)")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("OS_USERNAME environment variable is required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("OS_PASSWORD environment variable is required")
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("OS_PROJECT_ID environment variable is required")
	}

	return &cfg, nil
}

// ToAuthOptions converts Config to gophercloud AuthOptions
func (c *Config) ToAuthOptions() gophercloud.AuthOptions {
	return gophercloud.AuthOptions{
		IdentityEndpoint: c.AuthURL,
		Username:         c.Username,
		Password:         c.Password,
		TenantID:         c.ProjectID,
		DomainName:       c.DomainName,
		AllowReauth:      true,
	}
}

// Authenticate creates an authenticated OpenStack provider client
func (c *Config) Authenticate() (*gophercloud.ProviderClient, error) {
	opts := c.ToAuthOptions()
	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenStack client: %w", err)
	}

	ctx := context.Background()
	err = openstack.Authenticate(ctx, provider, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with OpenStack: %w", err)
	}
	return provider, nil
}
