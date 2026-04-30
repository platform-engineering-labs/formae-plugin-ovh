// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants for cloud compute resources.
const (
	InstanceResourceType = "OVH::Compute::Instance"
	SSHKeyResourceType   = "OVH::Compute::SSHKey"
	VolumeResourceType   = "OVH::Compute::Volume"
)

var cloudComputeRegistry *base.ResourceRegistry

// instanceStatusChecker verifies the instance has reached ACTIVE status AND
// has at least one IP attached. OVH reports ACTIVE before DHCP allocation
// finishes, so polling Read at that point yields ipAddresses=[] and the
// derived `networks` field is empty. Waiting for ipAddresses to populate
// ensures the persisted `networks` matches the spec at the verify step.
//
// OVH instances go through BUILD -> ACTIVE (or ERROR) states.
func instanceStatusChecker(resourceData map[string]interface{}) (bool, error) {
	status, ok := resourceData["status"].(string)
	if !ok || status != "ACTIVE" {
		return false, nil
	}
	if addrs, present := resourceData["ipAddresses"].([]interface{}); present && len(addrs) == 0 {
		return false, nil
	}
	return true, nil
}

// volumeStatusChecker verifies the volume has reached "available" status.
// OVH volumes go through creating -> available (or deleting) states.
// This enables async delete polling: after DELETE, formae polls Status until
// the volume returns 404 (gone) or remains in a non-available state.
func volumeStatusChecker(resourceData map[string]interface{}) (bool, error) {
	status, ok := resourceData["status"].(string)
	if !ok {
		return false, nil
	}
	return status == "available", nil
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
				ResourceType:     "instance",
				Scope:            &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate:   true,
				UpdateMethod:     base.UpdateMethodPut,
				AsyncDelete:      true,
				DeletingStatuses: []string{"DELETED", "DELETING"},
			},
			ResponseTransformer: instanceTransformer,
			StatusChecker:       instanceStatusChecker,
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
				ResourceType:     "volume",
				Scope:            &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate:   true,
				UpdateMethod:     base.UpdateMethodPut,
				DeletingStatuses: []string{"deleting", "deleted"},
			},
			StatusChecker: volumeStatusChecker,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
	})

	if err != nil {
		panic(err)
	}
}
