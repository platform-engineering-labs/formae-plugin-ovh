// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
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
	ResourceTypeContainer = "OVH::ObjectStorage::Container"
)

// Container schema and descriptor
var (
	ContainerDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeContainer,
		Discoverable: true,
	}

	ContainerSchema = model.Schema{
		Identifier:   "name",
		Discoverable: true,
		Fields:       []string{"name", "read_acl", "write_acl", "metadata", "storage_policy"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required:   true,
				CreateOnly: true, // Container names cannot be changed
			},
			"read_acl": {
				Required: false,
			},
			"write_acl": {
				Required: false,
			},
			"metadata": {
				Required: false,
			},
			"storage_policy": {
				Required:   false,
				CreateOnly: true, // Storage policy cannot be changed after creation
			},
		},
	}
)

// Container provisioner
type Container struct {
	Client *client.Client
	Config *config.Config
}

// Register the Container resource type
func init() {
	registry.Register(
		ResourceTypeContainer,
		ContainerDescriptor,
		ContainerSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Container{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// ============================================================================
// Create
// ============================================================================

// Create creates a new Swift container
func (c *Container) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeContainer, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	name, ok := props["name"].(string)
	if !ok || name == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeContainer, resource.OperationErrorCodeInvalidRequest, "", "container name is required"),
		}, nil
	}

	swiftClient, err := c.Client.EnsureSwift(ctx)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeContainer, resource.OperationErrorCodeGeneralServiceException, "", err.Error()),
		}, nil
	}

	createOpts := containers.CreateOpts{}

	if readACL, ok := props["read_acl"].(string); ok && readACL != "" {
		createOpts.ContainerRead = readACL
	}
	if writeACL, ok := props["write_acl"].(string); ok && writeACL != "" {
		createOpts.ContainerWrite = writeACL
	}
	if storagePolicy, ok := props["storage_policy"].(string); ok && storagePolicy != "" {
		createOpts.StoragePolicy = storagePolicy
	}
	if metadata, ok := props["metadata"].(map[string]interface{}); ok {
		metadataMap := make(map[string]string)
		for k, v := range metadata {
			if strVal, ok := v.(string); ok {
				metadataMap[k] = strVal
			}
		}
		if len(metadataMap) > 0 {
			createOpts.Metadata = metadataMap
		}
	}

	createResult := containers.Create(ctx, swiftClient, name, createOpts)
	_, err = createResult.Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create container: %v", err),
			},
		}, nil
	}

	// Get container details
	getResult := containers.Get(ctx, swiftClient, name, nil)
	header, err := getResult.Extract()
	if err != nil {
		basicProps := map[string]interface{}{"name": name}
		propsJSON, _ := resources.MarshalProperties(basicProps)
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCreate,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           name,
				ResourceProperties: []byte(propsJSON),
			},
		}, nil
	}

	metadata, _ := getResult.ExtractMetadata()
	propsJSON, err := resources.MarshalProperties(containerToProperties(name, header, metadata))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        name,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           name,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// ============================================================================
// Read
// ============================================================================

// Read retrieves the current state of a Swift container
func (c *Container) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	name := request.NativeID
	if name == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	swiftClient, err := c.Client.EnsureSwift(ctx)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, nil
	}

	getResult := containers.Get(ctx, swiftClient, name, nil)
	header, err := getResult.Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, nil
	}

	metadata, _ := getResult.ExtractMetadata()
	propsJSON, err := resources.MarshalProperties(containerToProperties(name, header, metadata))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, nil
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// ============================================================================
// Update
// ============================================================================

// Update updates an existing Swift container
func (c *Container) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeContainer, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	name := request.NativeID

	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeContainer, resource.OperationErrorCodeInvalidRequest, name, err.Error()),
		}, nil
	}

	swiftClient, err := c.Client.EnsureSwift(ctx)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeContainer, resource.OperationErrorCodeGeneralServiceException, name, err.Error()),
		}, nil
	}

	updateOpts := containers.UpdateOpts{}

	if readACL, ok := props["read_acl"].(string); ok {
		updateOpts.ContainerRead = &readACL
	}
	if writeACL, ok := props["write_acl"].(string); ok {
		updateOpts.ContainerWrite = &writeACL
	}
	if metadata, ok := props["metadata"].(map[string]interface{}); ok {
		metadataMap := make(map[string]string)
		for k, v := range metadata {
			if strVal, ok := v.(string); ok {
				metadataMap[k] = strVal
			}
		}
		updateOpts.Metadata = metadataMap
	}

	_, err = containers.Update(ctx, swiftClient, name, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update container: %v", err),
			},
		}, nil
	}

	getResult := containers.Get(ctx, swiftClient, name, nil)
	header, err := getResult.Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to read updated container: %v", err),
			},
		}, nil
	}

	metadata, _ := getResult.ExtractMetadata()
	propsJSON, err := resources.MarshalProperties(containerToProperties(name, header, metadata))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        name,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           name,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// ============================================================================
// Delete
// ============================================================================

// Delete removes a Swift container
func (c *Container) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeContainer, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	name := request.NativeID

	swiftClient, err := c.Client.EnsureSwift(ctx)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	_, err = containers.Delete(ctx, swiftClient, name).Extract()
	if err != nil {
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        name,
				},
			}, nil
		}

		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
				StatusMessage:   fmt.Sprintf("failed to delete container: %v", err),
			},
		}, nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        name,
		},
	}, nil
}

// ============================================================================
// Status & List
// ============================================================================

// Status checks the status of a long-running operation (containers are synchronous)
func (c *Container) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers Swift containers
func (c *Container) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	swiftClient, err := c.Client.EnsureSwift(ctx)
	if err != nil {
		return &resource.ListResult{
			NativeIDs: []string{},
		}, nil
	}

	allPages, err := containers.List(swiftClient, containers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{
			NativeIDs: []string{},
		}, nil
	}

	names, err := containers.ExtractNames(allPages)
	if err != nil {
		return &resource.ListResult{
			NativeIDs: []string{},
		}, nil
	}

	return &resource.ListResult{
		NativeIDs: names,
	}, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// containerToProperties converts Swift container metadata to a properties map
func containerToProperties(name string, header *containers.GetHeader, metadata map[string]string) map[string]interface{} {
	props := map[string]interface{}{
		"name": name,
	}

	if header != nil {
		props["bytes_used"] = header.BytesUsed
		props["object_count"] = header.ObjectCount

		if len(header.Read) > 0 {
			props["read_acl"] = header.Read
		}
		if len(header.Write) > 0 {
			props["write_acl"] = header.Write
		}
		if header.StoragePolicy != "" {
			props["storage_policy"] = header.StoragePolicy
		}
	}

	if len(metadata) > 0 {
		props["metadata"] = metadata
	}

	return props
}
