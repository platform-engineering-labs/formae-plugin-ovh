// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package client

import (
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
)

// Client wraps gophercloud service clients for different OpenStack services
type Client struct {
	Config *config.Config

	// OpenStack service clients
	ComputeClient *gophercloud.ServiceClient // Nova - instances, keypairs, flavors
	NetworkClient *gophercloud.ServiceClient // Neutron - networks, subnets, security groups
	VolumeClient  *gophercloud.ServiceClient // Cinder - volumes, snapshots

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

	return &Client{
		Config:        cfg,
		ComputeClient: computeClient,
		NetworkClient: networkClient,
		VolumeClient:  volumeClient,
		provider:      provider,
	}, nil
}
