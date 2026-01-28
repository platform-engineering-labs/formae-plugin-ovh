// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/platform-engineering-labs/formae/pkg/model"
)

// Config holds OVH REST API authentication configuration.
// OVHEndpoint can be stored in target config (non-sensitive).
// Credentials (ApplicationKey, ApplicationSecret, ConsumerKey) are always
// read from environment variables to avoid storing secrets.
type Config struct {
	// Stored in target config (non-sensitive)
	OVHEndpoint string `json:"OVHEndpoint"` // ovh-eu, ovh-ca, ovh-us, etc.

	// Read from environment variables only (never stored)
	ApplicationKey    string `json:"-"` // From OVH_APPLICATION_KEY
	ApplicationSecret string `json:"-"` // From OVH_APPLICATION_SECRET
	ConsumerKey       string `json:"-"` // From OVH_CONSUMER_KEY
	CloudProjectID    string `json:"-"` // From OVH_CLOUD_PROJECT_ID
}

// FromTarget extracts OVH configuration from a Target
func FromTarget(target *model.Target) (*Config, error) {
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}
	return FromTargetConfig(target.Config)
}

// FromTargetConfig extracts OVH configuration from a TargetConfig JSON.
// Only OVHEndpoint is read from the target config.
// Credentials are always read from environment variables.
func FromTargetConfig(targetConfig json.RawMessage) (*Config, error) {
	var cfg Config

	// Read non-sensitive config from target
	if len(targetConfig) > 0 {
		if err := json.Unmarshal(targetConfig, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal target config: %w", err)
		}
	}

	// OVHEndpoint can fall back to environment variable
	if cfg.OVHEndpoint == "" {
		cfg.OVHEndpoint = os.Getenv("OVH_ENDPOINT")
	}
	// Default to ovh-eu if not specified
	if cfg.OVHEndpoint == "" {
		cfg.OVHEndpoint = "ovh-eu"
	}

	// Credentials are ALWAYS read from environment variables (never stored)
	cfg.ApplicationKey = os.Getenv("OVH_APPLICATION_KEY")
	cfg.ApplicationSecret = os.Getenv("OVH_APPLICATION_SECRET")
	cfg.ConsumerKey = os.Getenv("OVH_CONSUMER_KEY")
	cfg.CloudProjectID = os.Getenv("OVH_CLOUD_PROJECT_ID")

	return &cfg, nil
}

// Validate checks that required OVH REST API fields are set
func (c *Config) Validate() error {
	if c.ApplicationKey == "" {
		return fmt.Errorf("OVH_APPLICATION_KEY environment variable is required")
	}
	if c.ApplicationSecret == "" {
		return fmt.Errorf("OVH_APPLICATION_SECRET environment variable is required")
	}
	if c.ConsumerKey == "" {
		return fmt.Errorf("OVH_CONSUMER_KEY environment variable is required")
	}
	if c.CloudProjectID == "" {
		return fmt.Errorf("OVH_CLOUD_PROJECT_ID environment variable is required")
	}
	return nil
}

// IsConfigured returns true if all required credentials are set
func (c *Config) IsConfigured() bool {
	return c.ApplicationKey != "" && c.ApplicationSecret != "" && c.ConsumerKey != "" && c.CloudProjectID != ""
}
