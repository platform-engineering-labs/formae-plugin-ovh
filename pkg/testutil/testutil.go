// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package testutil

import (
	"encoding/json"
	"os"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
)

var (
	// OVH OpenStack configuration - read from environment variables
	// Source your OpenStack credentials file before running tests:
	//   source ~/.ovh-openstack-credentials

	AuthURL    = getEnvOrDefault("OS_AUTH_URL", "https://auth.cloud.ovh.net/v3")
	Username   = os.Getenv("OS_USERNAME")
	Password   = os.Getenv("OS_PASSWORD")
	ProjectID  = os.Getenv("OS_PROJECT_ID")
	Region     = getEnvOrDefault("OS_REGION_NAME", "DE1")
	DomainName = getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default")

	// External network ID for floating IPs (OVH Ext-Net)
	// Can be overridden via OS_EXTERNAL_NETWORK_ID environment variable
	//ExternalNetworkID = getEnvOrDefault("OS_EXTERNAL_NETWORK_ID", "b347ed75-8603-4ce0-a40c-c6c98a8820fc") //TODO renable once REGION is fixed
	ExternalNetworkID = getEnvOrDefault("OS_EXTERNAL_NETWORK_ID", "ed0ab0c6-93ee-44f8-870b-d103065b1b34")

	// Compute test configuration
	// Override via environment variables if needed
	// TestFlavorID - Small flavor for testing (OVH d2-2: 2 vCPU, 2GB RAM)
	TestFlavorID = getEnvOrDefault("OS_TEST_FLAVOR_ID", "45ca263c-0373-4902-ab39-e5f0fc118190") // openstack flavor list --limit 20 2>&1 | head -30
	// TestImageID - Image for testing (set via OS_TEST_IMAGE_ID, no default as it varies by region)
	TestImageID = getEnvOrDefault("OS_TEST_IMAGE_ID", "6e9dbad6-03bb-49ee-a992-99e852f05381") // openstack image list --limit 10
	// TestNetworkID - Private network for instance testing (optional)
	TestNetworkID = getEnvOrDefault("OS_TEST_NETWORK_ID", "63dc373f-aaa9-4447-a4cd-5f6f28c2e7d7") // openstack network list --internal
	// TestSSHPublicKey - SSH public key for keypair testing (valid RSA 2048-bit key)
	TestSSHPublicKey = getEnvOrDefault("OS_TEST_SSH_PUBLIC_KEY", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCp/ZSLK1ks5D52nW/5iTsf6xWz1DOu94JB2C2KdN4earhMw4ej+HxANayQKD67pgSub0SnSH14/HxnnfnQ4g8r+o5KVZtZyBF7bJSc0qSyRYlQBWRWKUUlNFLFA5WQBRZyN+fBQvb+/9ndrFjPOiNcPjWI8EnF70dd4LGAIFuca+apsdeN125S9FaLZR3mKdrj8cp2nEMQvPUytqx6e3lZN0WhQ6ChztdijzUGdzi9ck/0spKwY1bMYgXmj1pWi3YDDnghc9a+MOdTGELVXhBou7qM5IxRdOR5T2BuHLgDipM+bKR4czEghKPNuOC8t+U+q/LShgkE/pFQg+pukp6z formae-integration-test")

	Config = &config.Config{
		AuthURL:    AuthURL,
		Region:     Region,
		Username:   Username,
		Password:   Password,
		ProjectID:  ProjectID,
		DomainName: DomainName,
	}

	// TargetConfig is a json.RawMessage containing the target configuration
	// Note: Only AuthURL and Region are stored in target config (credentials come from env)
	TargetConfig = func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"authURL": AuthURL,
			"region":  Region,
		})
		return b
	}()
)

// getEnvOrDefault returns the environment variable value or the default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsConfigured returns true if the required environment variables are set
func IsConfigured() bool {
	return Username != "" && Password != "" && ProjectID != ""
}

// SkipIfNotConfigured skips the test if required environment variables are not set
func SkipIfNotConfigured(t interface{ Skip(...any) }) {
	if !IsConfigured() {
		t.Skip("Skipping test: OVH credentials not configured. Set OS_USERNAME, OS_PASSWORD, and OS_PROJECT_ID environment variables.")
	}
}
