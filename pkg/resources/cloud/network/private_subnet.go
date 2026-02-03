// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// SubnetPrivate uses the private network API:
// - Create: POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
// - Read: GET /cloud/project/{serviceName}/network/private/{networkId}/subnet (list and filter)
// - Delete: DELETE /cloud/project/{serviceName}/network/private/{networkId}/subnet/{subnetId}

// subnetPathBuilder builds paths for the private network subnet API.
func subnetPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s/network/private", ctx.Project)

	// Add network ID
	if ctx.ParentResource != "" {
		path += "/" + ctx.ParentResource + "/subnet"
	}

	// Add subnet ID for Delete
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

// SubnetAPI defines API config for subnets with custom path builder.
var SubnetAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: subnetPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// SubnetOperations defines operation behavior for subnets.
// Native ID format: project/networkId/subnetId (nested resource).
var SubnetOperations = base.OperationConfig{
	Synchronous: true, // Subnet creation is synchronous
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		// Extract the subnet ID from response
		if id, ok := response["id"].(string); ok {
			if ctx.Project != "" && ctx.ParentResource != "" {
				return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.ParentResource, id)
			}
			if ctx.Project != "" {
				return fmt.Sprintf("%s/%s", ctx.Project, id)
			}
			return id
		}
		return ""
	},
}

// SubnetNativeID defines native ID format for subnets: "project/networkId/subnetId"
var SubnetNativeID = base.NativeIDConfig{
	Format: base.ProjectNestedFormat,
}

// PrivateSubnetProvisioner wraps BaseResource and adds custom Read via List+filter.
type PrivateSubnetProvisioner struct {
	base *base.BaseResource
}

// Create delegates to BaseResource
func (p *PrivateSubnetProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	return p.base.Create(ctx, request)
}

// Read implements Read by listing subnets and filtering by ID.
// Uses GET /cloud/project/{serviceName}/network/private/{networkId}/subnet
func (p *PrivateSubnetProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Parse native ID: project/networkId/subnetId
	parts := strings.Split(request.NativeID, "/")
	if len(parts) != 3 {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	project := parts[0]
	networkID := parts[1]
	subnetID := parts[2]

	// Build list URL: /cloud/project/{serviceName}/network/private/{networkId}/subnet
	listURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", project, networkID)

	// Call the OVH API to list subnets
	response, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   listURL,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeServiceInternalError,
		}, nil
	}

	// Response is an array of subnets (stored in BodyArray)
	subnets := response.BodyArray
	if subnets == nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeServiceInternalError,
		}, nil
	}

	// Find the subnet with matching ID
	for _, s := range subnets {
		subnet, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := subnet["id"].(string); ok && id == subnetID {
			// Found the subnet - add network_id to properties for consistency
			subnet["network_id"] = networkID

			propsJSON, err := json.Marshal(subnet)
			if err != nil {
				return &resource.ReadResult{
					ErrorCode: resource.OperationErrorCodeServiceInternalError,
				}, nil
			}
			return &resource.ReadResult{
				Properties: string(propsJSON),
			}, nil
		}
	}

	// Subnet not found
	return &resource.ReadResult{
		ErrorCode: resource.OperationErrorCodeNotFound,
	}, nil
}

// Update is not supported for PrivateSubnet
func (p *PrivateSubnetProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return p.base.Update(ctx, request)
}

// Delete delegates to BaseResource
func (p *PrivateSubnetProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.base.Delete(ctx, request)
}

// Status delegates to BaseResource
func (p *PrivateSubnetProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return p.base.Status(ctx, request)
}

// List is not implemented (no discovery for PrivateSubnet)
func (p *PrivateSubnetProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return &resource.ListResult{}, nil
}

// privateSubnetDefinition holds the resource definition for creating BaseResource instances
var privateSubnetDefinition *base.ResourceDefinition

func init() {
	// Define the resource configuration
	privateSubnetDefinition = &base.ResourceDefinition{
		ResourceType: PrivateSubnetResourceType,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "subnet", // Base type for path construction
			Scope:        &base.ScopeConfig{Type: base.ScopeProject},
			ParentResource: &base.ParentResourceConfig{
				RequiresParent: true,
				ParentType:     "network/private", // Used in URL path
				PropertyName:   "network_id",
			},
			SupportsUpdate: false, // OVH subnets are not updatable
		},
		// Strip network_id from request body (used in URL path)
		RequestTransformer: subnetPrivateTransformer,
		Operations: []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
		},
	}

	// Register with global registry using custom factory
	registry.Register(
		PrivateSubnetResourceType,
		privateSubnetDefinition.Operations,
		func(client *ovhtransport.Client) prov.Provisioner {
			// Create BaseResource directly for Create/Delete operations
			baseResource := &base.BaseResource{
				APIConfig:          SubnetAPI,
				OperationConfig:    SubnetOperations,
				ResourceConfig:     privateSubnetDefinition.ResourceConfig,
				NativeIDConfig:     SubnetNativeID,
				RequestTransformer: subnetPrivateTransformer,
				Client:             client,
			}
			return &PrivateSubnetProvisioner{
				base: baseResource,
			}
		},
	)
}
