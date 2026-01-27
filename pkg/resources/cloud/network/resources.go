// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants for cfres-based cloud network resources.
const (
	NetworkResourceType         = "OVH::Network::Network"
	PrivateNetworkResourceType  = "OVH::Network::PrivateNetwork"
	SubnetResourceType          = "OVH::Network::Subnet"
	SubnetPrivateResourceType   = "OVH::Network::SubnetPrivate"
	FloatingIPResourceType      = "OVH::Network::FloatingIP"
	SecurityGroupResourceType   = "OVH::Network::SecurityGroup"
	GatewayResourceType         = "OVH::Network::Gateway"
)

var cloudNetworkRegistry *base.ResourceRegistry

// gatewayStatusChecker verifies gateway has ACTIVE status.
// Gateway creation is async and we need to wait for ACTIVE status.
func gatewayStatusChecker(resourceData map[string]interface{}) (bool, error) {
	status, ok := resourceData["status"].(string)
	if !ok {
		// No status field - consider ready
		return true, nil
	}
	// Gateway is ready when status is ACTIVE
	return status == "ACTIVE", nil
}

// privateNetworkStatusChecker verifies all regions have ACTIVE status.
// OVH private networks require region activation before subnets can be created.
func privateNetworkStatusChecker(resourceData map[string]interface{}) (bool, error) {
	regions, ok := resourceData["regions"].([]interface{})
	if !ok {
		// No regions field or not an array - consider ready
		return true, nil
	}

	for _, r := range regions {
		region, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		status, _ := region["status"].(string)
		if status != "ACTIVE" {
			// At least one region is not yet active
			return false, nil
		}
	}

	// All regions are active
	return true, nil
}

func init() {
	cloudNetworkRegistry = base.NewResourceRegistry(cloud.CloudAPI, cloud.CloudOperations, cloud.CloudNativeID)

	err := cloudNetworkRegistry.RegisterAll([]base.ResourceDefinition{
		// Network (with embedded subnet and optional gateway)
		// Path: /cloud/project/{serviceName}/region/{regionName}/network
		// Region is obtained from target config
		{
			ResourceType: NetworkResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "network",
				Scope:          &base.ScopeConfig{Type: base.ScopeRegional},
				SupportsUpdate: false, // OVH networks don't support direct PUT/PATCH
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
		// Private Network (simple network without embedded subnet)
		// Path: /cloud/project/{serviceName}/network/private
		{
			ResourceType: PrivateNetworkResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "network/private",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate: false, // OVH private networks don't support direct PUT/PATCH
			},
			// Simplify regions from [{region: "DE1", ...}] to ["DE1"]
			ResponseTransformer: privateNetworkResponseTransformer_,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus, // Wait for region activation before dependent resources
			},
			// Check that all regions have ACTIVE status before allowing dependent resources
			StatusChecker: privateNetworkStatusChecker,
		},
		// Subnet (nested under region-based network)
		// Path: /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet
		// Region is obtained from target config
		{
			ResourceType: SubnetResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "subnet",
				Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
				ParentResource: &base.ParentResourceConfig{
					RequiresParent: true,
					ParentType:     "network",
					PropertyName:   "network_id",
				},
				SupportsUpdate: false, // OVH subnets are createOnly
			},
			// Native ID format: project/networkId/subnetId (includes parent for Read)
			NativeIDConfig: base.NativeIDConfig{Format: base.ProjectNestedFormat},
			// Strip network_id from body (it's used in URL path)
			RequestTransformer: subnetRegionalTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
		// SubnetPrivate (nested under private network) - uses OVH API field names directly
		// Path: /cloud/project/{serviceName}/network/private/{networkId}/subnet
		{
			ResourceType: SubnetPrivateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "subnet",
				Scope:        &base.ScopeConfig{Type: base.ScopeProject},
				ParentResource: &base.ParentResourceConfig{
					RequiresParent: true,
					ParentType:     "network/private",
					PropertyName:   "network_id",
				},
				SupportsUpdate: false, // OVH subnets are createOnly
			},
			// Only strip network_id from body (used in URL path)
			RequestTransformer: subnetPrivateTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
		// NOTE: FloatingIP is registered separately in floatingip.go with custom path builder
		// because Create path differs from Read/Delete/List path:
		// - Create: POST /cloud/project/{serviceName}/region/{regionName}/instance/{instanceId}/floatingIp
		// - Read:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
		// - Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
		// - List:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip

		// Security Group
		{
			ResourceType: SecurityGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instance/group",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate: true,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
		// Gateway (OVH Cloud Gateway for private networks)
		// Path: /cloud/project/{serviceName}/region/{regionName}/gateway
		// Region is obtained from target config
		{
			ResourceType: GatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "gateway",
				Scope:          &base.ScopeConfig{Type: base.ScopeRegional},
				SupportsUpdate: true, // Name and model can be updated
			},
			// Gateway creation is async - need to poll for status
			StatusChecker: gatewayStatusChecker,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
		// NOTE: The following resources require region or other special handling.
		// For now, they use the OpenStack transport via pkg/resources/network/:
		// - Router: requires region in path (/region/{region}/gateway)
		// - Port: requires network_id in path (/network/private/{networkId}/port)
		// - SecurityGroupRule: requires security_group_id in path (/instance/group/{sgId}/rule)
	})

	if err != nil {
		panic(err)
	}
}
