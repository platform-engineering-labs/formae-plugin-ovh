// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/registry"

	// Import resources to trigger init() registration
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/compute"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/network"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/volume"
)

// Plugin implements the Formae ResourcePlugin interface.
// The SDK automatically provides identity methods (Name, Version, Namespace)
// and schema methods (SupportedResources, SchemaForResourceType) by reading
// formae-plugin.pkl and schema/pkl/ at startup.
type Plugin struct{}

// Compile-time check: Plugin must satisfy ResourcePlugin interface.
var _ plugin.ResourcePlugin = &Plugin{}

// RateLimit returns the rate limit configuration for this plugin
func (p *Plugin) RateLimit() plugin.RateLimitConfig {
	return plugin.RateLimitConfig{
		Scope:                            plugin.RateLimitScopeNamespace,
		MaxRequestsPerSecondForNamespace: 10, // Conservative rate limit for OpenStack APIs
	}
}

// DiscoveryFilters returns declarative filters for discovery.
// OVH doesn't need any special filters currently.
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter {
	return nil
}

// LabelConfig returns the label extraction configuration for discovered OVH resources.
// Most OpenStack resources have a "name" property, with FloatingIP being the exception.
func (p *Plugin) LabelConfig() plugin.LabelConfig {
	return plugin.LabelConfig{
		DefaultQuery: "$.name",
		ResourceOverrides: map[string]string{
			// FloatingIP doesn't have a name, use the IP address
			"OVH::Network::FloatingIP": "$.floating_ip_address",
		},
	}
}

func (p *Plugin) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.Create(ctx, request)
}

func (p *Plugin) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.Read(ctx, request)
}

func (p *Plugin) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.Update(ctx, request)
}

func (p *Plugin) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.Delete(ctx, request)
}

func (p *Plugin) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.Status(ctx, request)
}

func (p *Plugin) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// Extract config from target
	cfg, err := config.FromTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from target: %w", err)
	}

	// Create OVH client
	ovhClient, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}

	// Check if resource type is supported
	if !registry.HasProvisioner(request.ResourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", request.ResourceType)
	}

	// Get provisioner and execute
	provisioner := registry.Get(request.ResourceType, ovhClient, cfg)
	return provisioner.List(ctx, request)
}
