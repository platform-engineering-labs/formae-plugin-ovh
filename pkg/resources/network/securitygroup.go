// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
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
	ResourceTypeSecurityGroup = "OVH::Network::SecurityGroup"
)

// SecurityGroup schema and descriptor
var (
	SecurityGroupDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeSecurityGroup,
		Discoverable: true,
	}

	SecurityGroupSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "tags"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required:   true,
				CreateOnly: true,
			},
			"description": {
				Required: false,
			},
			"tags": {
				Required: false,
			},
		},
	}
)

// SecurityGroup provisioner
type SecurityGroup struct {
	Client *client.Client
	Config *config.Config
}

// securityGroupToProperties converts an OpenStack security group to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func securityGroupToProperties(sg *groups.SecGroup) map[string]interface{} {
	props := map[string]interface{}{
		"id":          sg.ID,
		"name":        sg.Name,
		"description": sg.Description,
	}

	// Add tags if present
	if len(sg.Tags) > 0 {
		props["tags"] = sg.Tags
	}

	return props
}

// Register the SecurityGroup resource type
func init() {
	registry.Register(
		ResourceTypeSecurityGroup,
		SecurityGroupDescriptor,
		SecurityGroupSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &SecurityGroup{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new security group
func (s *SecurityGroup) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSecurityGroup, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Extract security group name (required)
	name, ok := props["name"].(string)
	if !ok || name == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "name is required",
			},
		}, nil
	}

	// Build create options
	createOpts := groups.CreateOpts{
		Name: name,
	}

	// Add optional description
	if description, ok := props["description"].(string); ok {
		createOpts.Description = description
	}

	// Create the security group via OpenStack
	sg, err := groups.Create(ctx, s.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create security group: %v", err),
			},
		}, nil
	}

	// Set tags if provided (must be done after creation via attributestags API)
	tags := resources.ParseTags(props["tags"])
	if len(tags) > 0 {
		_, err = attributestags.ReplaceAll(ctx, s.Client.NetworkClient, "security-groups", sg.ID, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - security group was created successfully
			fmt.Printf("warning: failed to set tags on security group %s: %v\n", sg.ID, err)
		} else {
			sg.Tags = tags
		}
	}

	// Convert security group to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(securityGroupToProperties(sg))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        sg.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           sg.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a security group
func (s *SecurityGroup) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the security group ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil // Don't return Go error for expected errors
	}

	// Get the security group from OpenStack
	sg, err := groups.Get(ctx, s.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, nil // Don't return Go error for expected errors like NotFound
	}

	// Convert security group to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(securityGroupToProperties(sg))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, nil // Don't return Go error for expected errors
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing security group
func (s *SecurityGroup) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the security group ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeSecurityGroup, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeSecurityGroup, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := groups.UpdateOpts{}

	// Update mutable fields
	if name, ok := props["name"].(string); ok && name != "" {
		updateOpts.Name = name
	}

	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	// Update the security group via OpenStack
	sg, err := groups.Update(ctx, s.Client.NetworkClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update security group: %v", err),
			},
		}, nil
	}

	// Update tags if provided (via attributestags API)
	if _, hasTags := props["tags"]; hasTags {
		tags := resources.ParseTags(props["tags"])
		if tags == nil {
			tags = []string{} // Empty slice to clear all tags
		}
		updatedTags, err := attributestags.ReplaceAll(ctx, s.Client.NetworkClient, "security-groups", id, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - security group was updated successfully
			fmt.Printf("warning: failed to update tags on security group %s: %v\n", id, err)
		} else {
			sg.Tags = updatedTags
		}
	}

	// Convert security group to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(securityGroupToProperties(sg))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        sg.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           sg.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a security group
func (s *SecurityGroup) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the security group ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeSecurityGroup, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the security group from OpenStack
	err := groups.Delete(ctx, s.Client.NetworkClient, id).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete security group: %v", err),
			},
		}, nil
	}

	// Return success
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        id,
		},
	}, nil
}

// Status checks the status of a long-running operation (security groups are synchronous, so not used)
func (s *SecurityGroup) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers security groups
func (s *SecurityGroup) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all security groups using pagination
	allPages, err := groups.List(s.Client.NetworkClient, groups.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list security groups: %w", err)
	}

	// Extract security groups from pages
	sgs, err := groups.ExtractGroups(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract security groups: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(sgs))
	for _, sg := range sgs {
		nativeIDs = append(nativeIDs, sg.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
