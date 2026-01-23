// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources"
)

const (
	ResourceTypeSubnet = "OVH::Network::Subnet"
)

// Subnet schema and descriptor
var (
	SubnetDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeSubnet,
		Discoverable: true,
	}

	SubnetSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "network_id", "cidr", "ip_version", "gateway_ip", "enable_dhcp", "dns_nameservers", "allocation_pools", "tags"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: false,
			},
			"description": {
				Required: false,
			},
			"network_id": {
				Required:   true,
				CreateOnly: true,
			},
			"cidr": {
				Required:   true,
				CreateOnly: true,
			},
			"ip_version": {
				Required:   false,
				CreateOnly: true,
			},
			"gateway_ip": {
				Required: false,
			},
			"enable_dhcp": {
				Required: false,
			},
			"dns_nameservers": {
				Required: false,
			},
			"allocation_pools": {
				Required:   false,
				CreateOnly: true,
			},
			"tags": {
				Required: false,
			},
		},
	}
)

// Subnet provisioner
type Subnet struct {
	Client *client.Client
	Config *config.Config
}

// subnetToProperties converts an OpenStack subnet to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func subnetToProperties(subnet *subnets.Subnet) map[string]interface{} {
	props := map[string]interface{}{
		"id":          subnet.ID,
		"network_id":  subnet.NetworkID,
		"name":        subnet.Name,
		"cidr":        subnet.CIDR,
		"ip_version":  subnet.IPVersion,
		"gateway_ip":  subnet.GatewayIP,
		"enable_dhcp": subnet.EnableDHCP,
	}

	// Add description if present
	if subnet.Description != "" {
		props["description"] = subnet.Description
	}

	// Always include dns_nameservers - if OpenStack returns empty/nil, include empty array
	// This ensures the property exists for comparison with expected state
	if subnet.DNSNameservers != nil {
		props["dns_nameservers"] = subnet.DNSNameservers
	} else {
		props["dns_nameservers"] = []string{}
	}

	// Include allocation_pools if present
	if len(subnet.AllocationPools) > 0 {
		props["allocation_pools"] = subnet.AllocationPools
	}

	// Add tags if present
	if len(subnet.Tags) > 0 {
		props["tags"] = subnet.Tags
	}

	return props
}

// Register the Subnet resource type
func init() {
	registry.Register(
		ResourceTypeSubnet,
		SubnetDescriptor,
		SubnetSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Subnet{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new subnet
func (s *Subnet) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Build create options - NetworkID and CIDR are required
	networkID, ok := props["network_id"].(string)
	if !ok || networkID == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, "", "network_id is required"),
		}, nil
	}

	cidr, ok := props["cidr"].(string)
	if !ok || cidr == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, "", "cidr is required"),
		}, nil
	}

	createOpts := subnets.CreateOpts{
		NetworkID: networkID,
		CIDR:      cidr,
	}

	// Add optional name
	if name, ok := props["name"].(string); ok && name != "" {
		createOpts.Name = name
	}

	// Add optional description
	if description, ok := props["description"].(string); ok {
		createOpts.Description = description
	}

	// Add optional ip_version (defaults to 4 if not specified)
	if ipVersion, ok := props["ip_version"]; ok {
		// Handle both int and float64 from JSON unmarshaling
		switch v := ipVersion.(type) {
		case int:
			createOpts.IPVersion = gophercloud.IPVersion(v)
		case float64:
			createOpts.IPVersion = gophercloud.IPVersion(int(v))
		}
	} else {
		createOpts.IPVersion = gophercloud.IPv4
	}

	// Add optional gateway_ip
	if gatewayIP, ok := props["gateway_ip"].(string); ok {
		createOpts.GatewayIP = &gatewayIP
	}

	// Add optional enable_dhcp (defaults to true if not specified)
	if enableDHCP, ok := props["enable_dhcp"].(bool); ok {
		createOpts.EnableDHCP = &enableDHCP
	}

	// Add optional dns_nameservers
	if dnsServers, ok := props["dns_nameservers"].([]interface{}); ok {
		nameservers := make([]string, len(dnsServers))
		for i, dns := range dnsServers {
			if dnsStr, ok := dns.(string); ok {
				nameservers[i] = dnsStr
			}
		}
		createOpts.DNSNameservers = nameservers
	}

	// Add optional allocation_pools
	if pools, ok := props["allocation_pools"].([]interface{}); ok {
		allocationPools := make([]subnets.AllocationPool, 0, len(pools))
		for _, pool := range pools {
			if poolMap, ok := pool.(map[string]interface{}); ok {
				start, startOk := poolMap["start"].(string)
				end, endOk := poolMap["end"].(string)
				if startOk && endOk {
					allocationPools = append(allocationPools, subnets.AllocationPool{
						Start: start,
						End:   end,
					})
				}
			}
		}
		if len(allocationPools) > 0 {
			createOpts.AllocationPools = allocationPools
		}
	}

	// Create the subnet via OpenStack
	subnet, err := subnets.Create(ctx, s.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create subnet: %v", err),
			},
		}, nil
	}

	// Set tags if provided (must be done after creation via attributestags API)
	tags := resources.ParseTags(props["tags"])
	if len(tags) > 0 {
		_, err = attributestags.ReplaceAll(ctx, s.Client.NetworkClient, "subnets", subnet.ID, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - subnet was created successfully
			fmt.Printf("warning: failed to set tags on subnet %s: %v\n", subnet.ID, err)
		} else {
			subnet.Tags = tags
		}
	}

	// Convert subnet to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(subnetToProperties(subnet))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        subnet.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           subnet.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a subnet
func (s *Subnet) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the subnet ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the subnet from OpenStack
	subnet, err := subnets.Get(ctx, s.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read subnet: %w", err)
	}

	// Convert subnet to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(subnetToProperties(subnet))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing subnet
func (s *Subnet) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the subnet ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := subnets.UpdateOpts{}

	// Update mutable fields
	if name, ok := props["name"].(string); ok {
		updateOpts.Name = &name
	}

	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	if gatewayIP, ok := props["gateway_ip"].(string); ok {
		updateOpts.GatewayIP = &gatewayIP
	}

	if enableDHCP, ok := props["enable_dhcp"].(bool); ok {
		updateOpts.EnableDHCP = &enableDHCP
	}

	if dnsServers, ok := props["dns_nameservers"].([]interface{}); ok {
		nameservers := make([]string, len(dnsServers))
		for i, dns := range dnsServers {
			if dnsStr, ok := dns.(string); ok {
				nameservers[i] = dnsStr
			}
		}
		updateOpts.DNSNameservers = &nameservers
	}

	// Update the subnet via OpenStack
	subnet, err := subnets.Update(ctx, s.Client.NetworkClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update subnet: %v", err),
			},
		}, nil
	}

	// Update tags if provided (via attributestags API)
	if _, hasTags := props["tags"]; hasTags {
		tags := resources.ParseTags(props["tags"])
		if tags == nil {
			tags = []string{} // Empty slice to clear all tags
		}
		updatedTags, err := attributestags.ReplaceAll(ctx, s.Client.NetworkClient, "subnets", id, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - subnet was updated successfully
			fmt.Printf("warning: failed to update tags on subnet %s: %v\n", id, err)
		} else {
			subnet.Tags = updatedTags
		}
	}

	// Convert subnet to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(subnetToProperties(subnet))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        subnet.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           subnet.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a subnet
func (s *Subnet) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the subnet ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeSubnet, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the subnet from OpenStack
	err := subnets.Delete(ctx, s.Client.NetworkClient, id).ExtractErr()
	if err != nil {
		// Check if the error is NotFound - if so, consider it a success (idempotent delete)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			// Resource already deleted - this is a success
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        id,
				},
			}, nil
		}

		// Other errors are actual failures
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
				StatusMessage:   fmt.Sprintf("failed to delete subnet: %v", err),
			},
		}, nil
	}

	// Return success
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        id,
		},
	}, nil
}

// Status checks the status of a long-running operation (subnets are synchronous, so not used)
func (s *Subnet) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers subnets
func (s *Subnet) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all subnets using pagination
	allPages, err := subnets.List(s.Client.NetworkClient, subnets.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list subnets: %w", err)
	}

	// Extract subnets from pages
	subnetList, err := subnets.ExtractSubnets(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract subnets: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(subnetList))
	for _, subnet := range subnetList {
		nativeIDs = append(nativeIDs, subnet.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
