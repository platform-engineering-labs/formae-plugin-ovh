// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants for cfres-based cloud compute resources.
const (
	InstanceResourceType = "OVH::Compute::Instance"
	SSHKeyResourceType   = "OVH::Compute::SSHKey"
	VolumeResourceType   = "OVH::Compute::Volume"
)

var cloudComputeRegistry *base.ResourceRegistry

// instanceStatusChecker verifies the instance has reached ACTIVE status.
// OVH instances go through BUILD -> ACTIVE (or ERROR) states.
func instanceStatusChecker(resourceData map[string]interface{}) (bool, error) {
	status, ok := resourceData["status"].(string)
	if !ok {
		// No status field - consider not ready
		return false, nil
	}
	return status == "ACTIVE", nil
}

func init() {
	cloudComputeRegistry = base.NewResourceRegistry(cloud.CloudAPI, cloud.CloudOperations, cloud.CloudNativeID)

	err := cloudComputeRegistry.RegisterAll([]base.ResourceDefinition{
		// Instance (OVH Cloud Compute Instance)
		// List:   GET /cloud/project/{serviceName}/instance
		// Create: POST /cloud/project/{serviceName}/instance
		// Read:   GET /cloud/project/{serviceName}/instance/{instanceId}
		// Update: PUT /cloud/project/{serviceName}/instance/{instanceId}
		// Delete: DELETE /cloud/project/{serviceName}/instance/{instanceId}
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instance",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
			StatusChecker: instanceStatusChecker,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
		// SSH Key (OVH Cloud SSH Key)
		// List:   GET /cloud/project/{serviceName}/sshkey
		// Create: POST /cloud/project/{serviceName}/sshkey
		// Read:   GET /cloud/project/{serviceName}/sshkey/{keyId}
		// Delete: DELETE /cloud/project/{serviceName}/sshkey/{keyId}
		// No Update support
		{
			ResourceType: SSHKeyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "sshkey",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate: false,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
		// Volume (OVH Cloud Block Storage Volume)
		// Create: POST /cloud/project/{serviceName}/volume
		// List:   GET /cloud/project/{serviceName}/volume
		// Read:   GET /cloud/project/{serviceName}/volume/{volumeId}
		// Update: PUT /cloud/project/{serviceName}/volume/{volumeId}
		// Delete: DELETE /cloud/project/{serviceName}/volume/{volumeId}
		{
			ResourceType: VolumeResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "volume",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
			},
		},
	})

	if err != nil {
		panic(err)
	}
}
