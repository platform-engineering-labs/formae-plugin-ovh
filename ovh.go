// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	openstacktransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/openstack"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	// Import OVH REST API resources to trigger init() registration
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/compute"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/database"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/dns"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/kube"

	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/network"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/registry"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud/storage"
	_ "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/openstack/resources/network"
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
		MaxRequestsPerSecondForNamespace: 2, // Conservative rate limit for APIs
	}
}

// DiscoveryFilters returns declarative filters for discovery.
// OVH doesn't need any special filters currently.
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter {
	return nil
}

// LabelConfig returns the label extraction configuration for discovered OVH resources.
// Most resources have a "name" property, with FloatingIP being the exception.
func (p *Plugin) LabelConfig() plugin.LabelConfig {
	return plugin.LabelConfig{
		DefaultQuery: "$.name",
		ResourceOverrides: map[string]string{
			// FloatingIP doesn't have a name, use the IP address
			"OVH::Network::FloatingIP": "$.floating_ip_address",
		},
	}
}

// augmentTargetConfig injects CloudProjectID from environment into target config.
// This ensures serviceName (CloudProjectID) flows through to API calls via
// extractProjectFromTargetConfig in base_resource.go.
func (p *Plugin) augmentTargetConfig(targetConfig []byte, cfg *config.Config) ([]byte, error) {
	var configMap map[string]interface{}
	if len(targetConfig) > 0 {
		if err := json.Unmarshal(targetConfig, &configMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal target config: %w", err)
		}
	} else {
		configMap = make(map[string]interface{})
	}

	// Inject CloudProjectID as serviceName for API path building
	if cfg.CloudProjectID != "" {
		configMap["serviceName"] = cfg.CloudProjectID
	}

	return json.Marshal(configMap)
}

// getProvisioner returns the appropriate provisioner for a resource type.
func (p *Plugin) getProvisioner(ctx context.Context, resourceType string, targetConfig []byte) (prov.Provisioner, error) {
	if !registry.HasProvisioner(resourceType) {
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	transportType := registry.GetTransportType(resourceType)

	switch transportType {
	case registry.TransportOVH:
		// Create OVH REST API client (go-ovh)
		cfg, err := config.FromTargetConfig(targetConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to extract config: %w", err)
		}
		ovhClient, err := ovhtransport.NewClient(&ovhtransport.OVHConfig{
			Endpoint:          cfg.OVHEndpoint,
			ApplicationKey:    cfg.ApplicationKey,
			ApplicationSecret: cfg.ApplicationSecret,
			ConsumerKey:       cfg.ConsumerKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create OVH REST API client: %w", err)
		}
		factory, _ := registry.GetOVHFactory(resourceType)
		return factory(ovhClient), nil

	case registry.TransportOpenStack:
		// Create OpenStack client (gophercloud)
		openstackCfg := openstacktransport.ConfigFromEnv()
		openstackClient, err := openstacktransport.NewClient(ctx, openstackCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenStack client: %w", err)
		}
		factory, _ := registry.GetOpenStackFactory(resourceType)
		return factory(openstackClient, openstackCfg), nil

	default:
		return nil, fmt.Errorf("unsupported transport type for resource: %s", resourceType)
	}
}

// prepareTargetConfig extracts config from target config bytes and returns an
// augmented target config with CloudProjectID injected as serviceName.
func (p *Plugin) prepareTargetConfig(targetConfig []byte) ([]byte, error) {
	cfg, err := config.FromTargetConfig(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config: %w", err)
	}
	return p.augmentTargetConfig(targetConfig, cfg)
}

func (p *Plugin) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.Create(ctx, request)
}

func (p *Plugin) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.Read(ctx, request)
}

func (p *Plugin) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.Update(ctx, request)
}

func (p *Plugin) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.Delete(ctx, request)
}

func (p *Plugin) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.Status(ctx, request)
}

func (p *Plugin) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	augmentedConfig, err := p.prepareTargetConfig(request.TargetConfig)
	if err != nil {
		return nil, err
	}
	request.TargetConfig = augmentedConfig

	provisioner, err := p.getProvisioner(ctx, request.ResourceType, request.TargetConfig)
	if err != nil {
		return nil, err
	}
	return provisioner.List(ctx, request)
}
