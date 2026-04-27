// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants for cloud network resources.
const (
	//NetworkResourceType        = "OVH::Network::Network"
	PrivateNetworkResourceType = "OVH::Network::PrivateNetwork"
	//SubnetResourceType         = "OVH::Network::Subnet"
	PrivateSubnetResourceType = "OVH::Network::PrivateSubnet"
	FloatingIPResourceType    = "OVH::Network::FloatingIP"
	SecurityGroupResourceType = "OVH::Network::SecurityGroup"
	GatewayResourceType       = "OVH::Network::Gateway"
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

// privateNetworkStatusChecker verifies the network is fully provisioned.
// cloud.network.Network has TWO status fields:
//   - top-level `status` (NetworkStatusEnum: ACTIVE | BUILDING | DELETING |
//     ERROR) — overall network state.
//   - `regions[].status` (NetworkRegionStatusEnum) — per-region rollout.
// Dependents (subnet, instance) can race the network if we only gate on
// regions[].status reaching ACTIVE — the instance API consults the
// top-level status and rejects with "network not found" until it flips to
// ACTIVE. Each region also needs an openstackId before its compute layer
// can resolve the network. ERROR is terminal.
func privateNetworkStatusChecker(resourceData map[string]interface{}) (bool, error) {
	if topStatus, ok := resourceData["status"].(string); ok {
		switch topStatus {
		case "ACTIVE":
			// continue to per-region checks
		case "ERROR":
			id, _ := resourceData["id"].(string)
			return false, fmt.Errorf("private network %s entered ERROR state", id)
		default:
			return false, nil
		}
	}

	regions, ok := resourceData["regions"].([]interface{})
	if !ok {
		return true, nil
	}

	for _, r := range regions {
		region, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		status, _ := region["status"].(string)
		if status != "ACTIVE" {
			return false, nil
		}
		openstackID, _ := region["openstackId"].(string)
		if openstackID == "" {
			return false, nil
		}
	}

	return true, nil
}

// privateNetworkReadinessProbe verifies the network is visible on the
// surfaces that dependent resources actually call:
//   - /network/private/{id}/subnet  — subnets are created here.
//   - /region/{r}/network/{id}      — the regional/Neutron view that the
//     instance API consults; staying on /network/private alone leads to
//     "network <id> not found" responses from POST /instance even after
//     the network reports status=ACTIVE with an openstackId.
// We re-fetch the network to learn its regions, then probe each one.
func privateNetworkReadinessProbe(ctx context.Context, client base.TransportClient, pathCtx base.PathContext) (bool, error) {
	subnetURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", pathCtx.Project, pathCtx.ResourceName)
	if _, err := client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: subnetURL}); err != nil {
		return false, nil
	}

	netURL := fmt.Sprintf("/cloud/project/%s/network/private/%s", pathCtx.Project, pathCtx.ResourceName)
	netResp, err := client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: netURL})
	if err != nil {
		return false, nil
	}

	regionsRaw, _ := netResp.Body["regions"].([]interface{})
	if len(regionsRaw) == 0 {
		return false, nil
	}
	for _, r := range regionsRaw {
		regionObj, ok := r.(map[string]interface{})
		if !ok {
			return false, nil
		}
		regionName, _ := regionObj["region"].(string)
		openstackID, _ := regionObj["openstackId"].(string)
		if regionName == "" || openstackID == "" {
			return false, nil
		}
		// The regional endpoint expects the OpenStack UUID, not the OVH
		// pn-XXX ID — using the latter returns 400 invalid uuid.
		regionalURL := fmt.Sprintf("/cloud/project/%s/region/%s/network/%s", pathCtx.Project, regionName, openstackID)
		if _, err := client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: regionalURL}); err != nil {
			return false, nil
		}
	}
	return true, nil
}

func init() {
	cloudNetworkRegistry = base.NewResourceRegistry(cloud.CloudAPI, cloud.CloudOperations, cloud.CloudNativeID)

	err := cloudNetworkRegistry.RegisterAll([]base.ResourceDefinition{
		// Network (with embedded subnet and optional gateway)
		// Path: /cloud/project/{serviceName}/region/{regionName}/network
		// Region is obtained from target config
		//{
		//	ResourceType: NetworkResourceType,
		//	ResourceConfig: base.ResourceConfig{
		//		ResourceType:   "network",
		//		Scope:          &base.ScopeConfig{Type: base.ScopeRegional},
		//		SupportsUpdate: false, // OVH networks don't support direct PUT/PATCH
		//	},
		//	Operations: []resource.Operation{
		//		resource.OperationCreate,
		//		resource.OperationRead,
		//		resource.OperationDelete,
		//		resource.OperationList,
		//	},
		//},
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
			StatusChecker:  privateNetworkStatusChecker,
			ReadinessProbe: privateNetworkReadinessProbe,
		},
		// Subnet (nested under region-based network)
		// Path: /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet
		// Region is obtained from target config
		//{
		//	ResourceType: SubnetResourceType,
		//	ResourceConfig: base.ResourceConfig{
		//		ResourceType: "subnet",
		//		Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
		//		ParentResource: &base.ParentResourceConfig{
		//			RequiresParent: true,
		//			ParentType:     "network",
		//			PropertyName:   "network_id",
		//		},
		//		SupportsUpdate: false, // OVH subnets are createOnly
		//	},
		//	// Native ID format: project/networkId/subnetId (includes parent for Read)
		//	NativeIDConfig: base.NativeIDConfig{Format: base.ProjectNestedFormat},
		//	// Strip network_id from body (it's used in URL path)
		//	RequestTransformer: subnetRegionalTransformer,
		//	Operations: []resource.Operation{
		//		resource.OperationCreate,
		//		resource.OperationRead,
		//		resource.OperationDelete,
		//		resource.OperationList,
		//	},
		//},
		// NOTE: SubnetPrivate is registered separately in subnet.go with custom path builder.
		// Only Create and Delete operations are supported:
		// - Create: POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
		// - Delete: DELETE /cloud/project/{serviceName}/network/private/{networkId}/subnet/{subnetId}

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
		// NOTE: Gateway is registered separately in gateway.go with custom path builder
		// because Create path differs from Read/Delete/List path:
		// - Create: POST /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet/{subnetId}/gateway
		// - Read:   GET /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
		// - Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
		// - List:   GET /cloud/project/{serviceName}/region/{regionName}/gateway
		//
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
