// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/cloud"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants for cfres-based cloud storage resources.
const (
	ContainerResourceType = "OVH::Storage::Container"
)

var cloudStorageRegistry *base.ResourceRegistry

func init() {
	cloudStorageRegistry = base.NewResourceRegistry(cloud.CloudAPI, cloud.CloudOperations, cloud.CloudNativeID)

	err := cloudStorageRegistry.RegisterAll([]base.ResourceDefinition{
		// Container (OVH Cloud Object Storage Container)
		// List:   GET /cloud/project/{serviceName}/storage
		// Create: POST /cloud/project/{serviceName}/storage
		// Read:   GET /cloud/project/{serviceName}/storage/{containerId}
		// Update: PUT /cloud/project/{serviceName}/storage/{containerId}
		// Delete: DELETE /cloud/project/{serviceName}/storage/{containerId}
		{
			ResourceType: ContainerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "storage",
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
