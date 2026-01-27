// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// SubnetPrivate uses the private network API:
// - Create: POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
// - Delete: DELETE /cloud/project/{serviceName}/network/private/{networkId}/subnet/{subnetId}
// Note: No Read or List operations available on this API.

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

// subnetPrivateRegistry is separate from cloudNetworkRegistry to use custom API config.
var subnetPrivateRegistry *base.ResourceRegistry

func init() {
	// Create a separate registry for SubnetPrivate with custom path builder
	subnetPrivateRegistry = base.NewResourceRegistry(
		SubnetAPI,
		SubnetOperations,
		SubnetNativeID, // Uses project/region/networkId/subnetId format
	)

	// SubnetPrivate (OVH Cloud Private Network Subnet)
	// Create: POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
	// Delete: DELETE /cloud/project/{serviceName}/network/private/{networkId}/subnet/{subnetId}
	// Note: No Read or List operations available on this API.
	err := subnetPrivateRegistry.Register(base.ResourceDefinition{
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
			resource.OperationDelete,
		},
	})

	if err != nil {
		panic(err)
	}
}
