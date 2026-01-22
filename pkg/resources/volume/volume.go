// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package volume

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
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
	ResourceTypeVolume = "OVH::Volume::Volume"
)

// Volume schema and descriptor
var (
	VolumeDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeVolume,
		Discoverable: true,
	}

	VolumeSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "size", "availability_zone", "volume_type", "metadata", "image_ref", "snapshot_id", "source_vol_id"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: false,
			},
			"description": {
				Required: false,
			},
			"size": {
				Required:   true,
				CreateOnly: true, // Size cannot be changed after creation (would require replacement)
			},
			"availability_zone": {
				Required:   false,
				CreateOnly: true,
			},
			"volume_type": {
				Required:   false,
				CreateOnly: true,
			},
			"metadata": {
				Required: false,
			},
			"image_ref": {
				Required:   false,
				CreateOnly: true, // Create bootable volume from image
			},
			"snapshot_id": {
				Required:   false,
				CreateOnly: true, // Create volume from snapshot
			},
			"source_vol_id": {
				Required:   false,
				CreateOnly: true, // Clone existing volume
			},
		},
	}
)

// Volume provisioner
type Volume struct {
	Client *client.Client
	Config *config.Config
}

// Register the Volume resource type
func init() {
	registry.Register(
		ResourceTypeVolume,
		VolumeDescriptor,
		VolumeSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Volume{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new volume (async operation)
func (v *Volume) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeVolume, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Extract size (required)
	sizeFloat, ok := props["size"].(float64)
	if !ok {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "size is required and must be a number",
			},
		}, nil
	}
	size := int(sizeFloat)

	// Build create options
	createOpts := volumes.CreateOpts{
		Size: size,
	}

	// Add optional name
	if name, ok := props["name"].(string); ok && name != "" {
		createOpts.Name = name
	}

	// Add optional description
	if description, ok := props["description"].(string); ok && description != "" {
		createOpts.Description = description
	}

	// Add optional availability zone
	if az, ok := props["availability_zone"].(string); ok && az != "" {
		createOpts.AvailabilityZone = az
	}

	// Add optional volume type
	if volumeType, ok := props["volume_type"].(string); ok && volumeType != "" {
		createOpts.VolumeType = volumeType
	}

	// Add optional metadata
	if metadataRaw, ok := props["metadata"]; ok && metadataRaw != nil {
		metadata, ok := metadataRaw.(map[string]interface{})
		if ok {
			metadataStr := make(map[string]string, len(metadata))
			for k, v := range metadata {
				if vStr, ok := v.(string); ok {
					metadataStr[k] = vStr
				}
			}
			createOpts.Metadata = metadataStr
		}
	}

	// Add optional image reference (create bootable volume from image)
	if imageRef, ok := props["image_ref"].(string); ok && imageRef != "" {
		createOpts.ImageID = imageRef
	}

	// Add optional snapshot ID (create volume from snapshot)
	if snapshotID, ok := props["snapshot_id"].(string); ok && snapshotID != "" {
		createOpts.SnapshotID = snapshotID
	}

	// Add optional source volume ID (clone existing volume)
	if sourceVolID, ok := props["source_vol_id"].(string); ok && sourceVolID != "" {
		createOpts.SourceVolID = sourceVolID
	}

	// Create the volume via OpenStack
	vol, err := volumes.Create(ctx, v.Client.VolumeClient, createOpts, nil).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create volume: %v", err),
			},
		}, nil
	}

	// Return InProgress - volumes are created asynchronously
	// The caller will poll Status() to check when the volume is ready
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			RequestID:       vol.ID, // Use volume ID for polling
			NativeID:        vol.ID,
		},
	}, nil
}

// Read retrieves the current state of a volume
func (v *Volume) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the volume ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the volume from OpenStack
	vol, err := volumes.Get(ctx, v.Client.VolumeClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read volume: %w", err)
	}

	// Convert volume to properties
	props := map[string]interface{}{
		"id":          vol.ID,
		"name":        vol.Name,
		"description": vol.Description,
		"size":        vol.Size,
		"status":      vol.Status,
	}

	// Add optional fields if present
	if vol.AvailabilityZone != "" {
		props["availability_zone"] = vol.AvailabilityZone
	}
	if vol.VolumeType != "" {
		props["volume_type"] = vol.VolumeType
	}
	if len(vol.Metadata) > 0 {
		props["metadata"] = vol.Metadata
	}
	if vol.SnapshotID != "" {
		props["snapshot_id"] = vol.SnapshotID
	}
	if vol.SourceVolID != "" {
		props["source_vol_id"] = vol.SourceVolID
	}
	// Note: ImageID is not returned by Volume.Get, it's only used during creation

	// Marshal properties to JSON
	propsJSON, err := resources.MarshalProperties(props)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing volume (only mutable fields: name, description)
func (v *Volume) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the volume ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeVolume, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeVolume, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options - name, description, and metadata are mutable
	updateOpts := volumes.UpdateOpts{}

	if name, ok := props["name"].(string); ok {
		updateOpts.Name = &name
	}

	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	// Add metadata if provided
	if metadataRaw, ok := props["metadata"]; ok && metadataRaw != nil {
		metadata, ok := metadataRaw.(map[string]interface{})
		if ok {
			metadataStr := make(map[string]string, len(metadata))
			for k, val := range metadata {
				if vStr, ok := val.(string); ok {
					metadataStr[k] = vStr
				}
			}
			updateOpts.Metadata = metadataStr
		}
	}

	// Update the volume via OpenStack
	vol, err := volumes.Update(ctx, v.Client.VolumeClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update volume: %v", err),
			},
		}, nil
	}

	// Read the current properties to include in the result
	// This ensures Formae stores the actual state after update
	readResult, readErr := v.Read(ctx, &resource.ReadRequest{
		NativeID: vol.ID,
	})
	if readErr != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        vol.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to read volume after update: %v", readErr),
			},
		}, nil
	}

	// Return success - volume updates are synchronous
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           vol.ID,
			ResourceProperties: []byte(readResult.Properties),
		},
	}, nil
}

// Delete removes a volume (async operation)
func (v *Volume) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the volume ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeVolume, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the volume from OpenStack
	err := volumes.Delete(ctx, v.Client.VolumeClient, id, nil).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete volume: %v", err),
			},
		}, nil
	}

	// Check the volume status to see if deletion is in progress
	vol, err := volumes.Get(ctx, v.Client.VolumeClient, id).Extract()
	if err != nil {
		// If we get NotFound, deletion completed immediately
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        id,
				},
			}, nil
		}
		// Other errors during status check
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
				StatusMessage:   fmt.Sprintf("failed to check volume status after delete: %v", err),
			},
		}, nil
	}

	// If volume is in "deleting" state, return InProgress
	if vol.Status == "deleting" {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       id,
				NativeID:        id,
			},
		}, nil
	}

	// If volume is in error state
	if vol.Status == "error_deleting" {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("volume deletion failed with status: %s", vol.Status),
			},
		}, nil
	}

	// Otherwise, deletion is complete
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        id,
		},
	}, nil
}

// Status checks the status of a long-running operation (Create/Delete)
func (v *Volume) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// Get the volume by RequestID (which is the volume ID)
	if request.RequestID == "" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
			},
		}, fmt.Errorf("requestID is required")
	}

	vol, err := volumes.Get(ctx, v.Client.VolumeClient, request.RequestID).Extract()
	if err != nil {
		// NotFound means the volume was deleted (or never existed)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			// Volume is gone - this is success for a delete operation
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        request.RequestID,
				},
			}, nil
		}

		// Other errors
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
			},
		}, fmt.Errorf("failed to get volume status: %w", err)
	}

	// Map volume status to operation status
	// We infer the operation from the volume state
	switch vol.Status {
	case "available":
		// Volume is ready - this is success for a create operation
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        vol.ID,
			},
		}, nil

	case "creating", "downloading":
		// Volume creation is in progress
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       vol.ID,
				NativeID:        vol.ID,
			},
		}, nil

	case "deleting":
		// Volume deletion is in progress
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       vol.ID,
				NativeID:        vol.ID,
			},
		}, nil

	case "error", "error_deleting":
		// Determine operation from the error state
		operation := resource.OperationCreate
		if vol.Status == "error_deleting" {
			operation = resource.OperationDelete
		}
		// Volume operation failed
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       operation,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
			},
		}, fmt.Errorf("volume in error state: %s", vol.Status)

	default:
		// Unknown state - treat as in progress (likely create)
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       vol.ID,
				NativeID:        vol.ID,
			},
		}, nil
	}
}

// List discovers volumes
func (v *Volume) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all volumes using pagination
	allPages, err := volumes.List(v.Client.VolumeClient, volumes.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list volumes: %w", err)
	}

	// Extract volumes from pages
	vols, err := volumes.ExtractVolumes(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract volumes: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(vols))
	for _, vol := range vols {
		nativeIDs = append(nativeIDs, vol.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
