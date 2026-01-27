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

// VolumeSnapshotResourceType is the resource type for volume snapshots.
const VolumeSnapshotResourceType = "OVH::Compute::VolumeSnapshot"

// VolumeSnapshot has a special path structure:
// - Create: POST /cloud/project/{serviceName}/volume/{volumeId}/snapshot
// - Read:   GET /cloud/project/{serviceName}/volume/snapshot/{snapshotId}
// - Delete: DELETE /cloud/project/{serviceName}/volume/snapshot/{snapshotId}
// - List:   GET /cloud/project/{serviceName}/volume/snapshot

// volumeSnapshotPathBuilder handles the special case where Create is nested under volume.
func volumeSnapshotPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s", ctx.Project)

	// For Create (collection URL with ParentResource set): use volume/{volumeId}/snapshot path
	// POST /cloud/project/{serviceName}/volume/{volumeId}/snapshot
	if ctx.ResourceName == "" && ctx.ParentResource != "" {
		return path + fmt.Sprintf("/volume/%s/snapshot", ctx.ParentResource)
	}

	// For List (collection URL without ParentResource): use volume/snapshot path
	// GET /cloud/project/{serviceName}/volume/snapshot
	if ctx.ResourceName == "" {
		return path + "/volume/snapshot"
	}

	// For Read/Delete (resource URL): use volume/snapshot/{id} path
	// GET/DELETE /cloud/project/{serviceName}/volume/snapshot/{snapshotId}
	return path + fmt.Sprintf("/volume/snapshot/%s", ctx.ResourceName)
}

// VolumeSnapshotAPI defines API config for volume snapshots with custom path builder.
var VolumeSnapshotAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: volumeSnapshotPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// VolumeSnapshotOperations defines operation behavior for volume snapshots.
// Native ID format: project/snapshotId (no volume_id needed for Read/Delete).
var VolumeSnapshotOperations = base.OperationConfig{
	Synchronous: true,
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		if id, ok := response["id"].(string); ok {
			if ctx.Project != "" {
				return fmt.Sprintf("%s/%s", ctx.Project, id)
			}
			return id
		}
		return ""
	},
}

// volumeSnapshotRequestTransformer strips volume_id from the request body.
// volume_id is used in the URL path for Create operations.
type volumeSnapshotRequestTransformer struct{}

func (t *volumeSnapshotRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range props {
		// Strip volume_id - it's used in the URL path, not the body
		if k == "volume_id" {
			continue
		}
		result[k] = v
	}
	return result, nil
}

var volumeSnapshotTransformer = &volumeSnapshotRequestTransformer{}

// volumeSnapshotRegistry is separate from cloudComputeRegistry to use custom API config.
var volumeSnapshotRegistry *base.ResourceRegistry

func init() {
	volumeSnapshotRegistry = base.NewResourceRegistry(
		VolumeSnapshotAPI,
		VolumeSnapshotOperations,
		cloud.CloudNativeID,
	)

	// VolumeSnapshot (OVH Cloud Volume Snapshot)
	// Create: POST /cloud/project/{serviceName}/volume/{volumeId}/snapshot
	// Read:   GET /cloud/project/{serviceName}/volume/snapshot/{snapshotId}
	// Delete: DELETE /cloud/project/{serviceName}/volume/snapshot/{snapshotId}
	// List:   GET /cloud/project/{serviceName}/volume/snapshot
	err := volumeSnapshotRegistry.Register(base.ResourceDefinition{
		ResourceType: VolumeSnapshotResourceType,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "snapshot",
			Scope:        &base.ScopeConfig{Type: base.ScopeProject},
			ParentResource: &base.ParentResourceConfig{
				RequiresParent: true,
				ParentType:     "volume",
				PropertyName:   "volume_id",
			},
			SupportsUpdate: false, // Snapshots are immutable
		},
		RequestTransformer: volumeSnapshotTransformer,
		Operations: []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
	})

	if err != nil {
		panic(err)
	}
}
