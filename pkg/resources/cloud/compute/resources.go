// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"

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

// instanceStatusChecker verifies the instance has reached ACTIVE status.
// OVH instances go through BUILD -> ACTIVE (or ERROR) states. ERROR is
// terminal - return an error so polling stops with an actionable message
// instead of looping until the framework times out.
func instanceStatusChecker(resourceData map[string]interface{}) (bool, error) {
	status, ok := resourceData["status"].(string)
	if !ok {
		return false, nil
	}
	switch status {
	case "ACTIVE":
		return true, nil
	case "ERROR":
		name, _ := resourceData["name"].(string)
		id, _ := resourceData["id"].(string)
		return false, fmt.Errorf("instance %q (id=%s) entered ERROR state", name, id)
	default:
		return false, nil
	}
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
				ResourceType:   "instance",
				Scope:          &base.ScopeConfig{Type: base.ScopeProject},
				SupportsUpdate:         true,
				UpdateMethod:           base.UpdateMethodPut,
				WaitUntilGone:          true,
				DeletionTimeoutSeconds: 30,
				DeletingStatuses:       []string{"DELETING", "DELETED"},
				// OVH's instance API lags behind the network and subnet APIs:
				// after a private network/subnet is created, the instance
				// endpoint can still report "network ... not found" for tens
				// of seconds. Retry rather than failing the apply.
				CreateRetryOnInvalidInputContains: []string{"not found"},
				CreateRetryAttempts:               4,
				CreateRetryBackoffSeconds:         15,
			},
			RequestTransformer:  instanceRequestTransformer_,
			ResponseTransformer: instanceResponseTransformer_,
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
