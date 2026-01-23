// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package client

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
)

// Client wraps gophercloud service clients for different OpenStack services
type Client struct {
	Config *config.Config

	// OpenStack service clients
	ComputeClient       *gophercloud.ServiceClient // Nova - instances, keypairs, flavors
	NetworkClient       *gophercloud.ServiceClient // Neutron - networks, subnets, security groups
	VolumeClient        *gophercloud.ServiceClient // Cinder - volumes, snapshots
	ObjectStorageClient *gophercloud.ServiceClient // Swift - containers, objects (may be nil if not available)

	// Provider client (for token refresh, etc.)
	provider *gophercloud.ProviderClient
}

// NewClient creates a new OVH OpenStack client with authenticated service clients
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// Authenticate and get provider client
	provider, err := cfg.Authenticate()
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	// Create compute service client (Nova)
	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	// Create network service client (Neutron)
	networkClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	// Create volume service client (Cinder)
	volumeClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create volume client: %w", err)
	}

	client := &Client{
		Config:        cfg,
		ComputeClient: computeClient,
		NetworkClient: networkClient,
		VolumeClient:  volumeClient,
		provider:      provider,
	}

	// Try to create object storage service client (Swift) - may not be available in all regions
	// Map region to Swift region format (e.g., DE1 -> DE, GRA11 -> GRA)
	swiftRegion := mapRegionToSwiftRegion(cfg.Region)
	objectStorageClient, err := openstack.NewObjectStorageV1(provider, gophercloud.EndpointOpts{
		Region: swiftRegion,
	})
	if err == nil {
		client.ObjectStorageClient = objectStorageClient
	}

	return client, nil
}

// HasSwift returns true if Swift object storage client is available
func (c *Client) HasSwift() bool {
	return c.ObjectStorageClient != nil
}

// EnsureSwift returns the Swift client or an error if not available
func (c *Client) EnsureSwift(ctx context.Context) (*gophercloud.ServiceClient, error) {
	if c.ObjectStorageClient == nil {
		return nil, fmt.Errorf("swift object storage not available in this region")
	}
	return c.ObjectStorageClient, nil
}

// mapRegionToSwiftRegion converts OpenStack region codes to Swift region codes
// Swift uses region names without version suffixes (e.g., DE1 -> DE, GRA11 -> GRA)
func mapRegionToSwiftRegion(region string) string {
	// Map of OpenStack regions to Swift regions
	regionMap := map[string]string{
		// Germany / Frankfurt
		"DE1": "DE",
		// France / Gravelines
		"GRA1":  "GRA",
		"GRA3":  "GRA",
		"GRA5":  "GRA",
		"GRA7":  "GRA",
		"GRA9":  "GRA",
		"GRA11": "GRA",
		// France / Strasbourg
		"SBG1": "SBG",
		"SBG3": "SBG",
		"SBG5": "SBG",
		// Canada / Beauharnois
		"BHS1": "BHS",
		"BHS3": "BHS",
		"BHS5": "BHS",
		// UK / London
		"UK1": "UK",
		// Poland / Warsaw
		"WAW1": "WAW",
		// US / Virginia
		"US-EAST-VA-1": "US-EAST-VA",
	}

	if swiftRegion, ok := regionMap[region]; ok {
		return swiftRegion
	}
	// If not found, return as-is (might already be correct format)
	return region
}
