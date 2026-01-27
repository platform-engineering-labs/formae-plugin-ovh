// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// VolumeAttachmentResourceType is the resource type for volume attachments.
const VolumeAttachmentResourceType = "OVH::Compute::VolumeAttachment"

// VolumeAttachment has a special pattern where both operations use POST:
// - Create: POST /cloud/project/{serviceName}/volume/{volumeId}/attach
// - Delete: POST /cloud/project/{serviceName}/volume/{volumeId}/detach
// - No Read, Update, or List support

// volumeAttachmentProvisioner handles attach/detach operations.
type volumeAttachmentProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &volumeAttachmentProvisioner{}

// Create attaches a volume to an instance.
// POST /cloud/project/{serviceName}/volume/{volumeId}/attach
// Body: {"instanceId": "..."}
func (p *volumeAttachmentProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("failed to parse properties: %v", err),
			},
		}, nil
	}

	// Extract required fields
	volumeID, _ := props["volume_id"].(string)
	instanceID, _ := props["instance_id"].(string)
	project := extractProject(request.TargetConfig)

	if volumeID == "" || instanceID == "" || project == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "volume_id, instance_id, and serviceName are required",
			},
		}, nil
	}

	// Build attach URL
	url := fmt.Sprintf("/cloud/project/%s/volume/%s/attach", project, volumeID)

	// Build request body - only instanceId is sent
	body := map[string]interface{}{
		"instanceId": instanceID,
	}

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.CreateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationCreate,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       ovhtransport.ToResourceErrorCode(transportErr.Code),
					StatusMessage:   transportErr.Message,
				},
			}, nil
		}
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Native ID format: project/volumeId/instanceId
	// We need both volumeId and instanceId for detach operation
	nativeID := fmt.Sprintf("%s/%s/%s", project, volumeID, instanceID)

	propsJSON, _ := json.Marshal(response.Body)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

// Read is not supported for volume attachments.
func (p *volumeAttachmentProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	return &resource.ReadResult{
		ErrorCode: resource.OperationErrorCodeNotFound,
	}, nil
}

// Update is not supported for volume attachments.
func (p *volumeAttachmentProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			NativeID:        request.NativeID,
		},
	}, nil
}

// Delete detaches a volume from an instance.
// POST /cloud/project/{serviceName}/volume/{volumeId}/detach
// Body: {"instanceId": "..."}
func (p *volumeAttachmentProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Parse native ID: project/volumeId/instanceId
	parts := strings.SplitN(request.NativeID, "/", 3)
	if len(parts) != 3 {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("invalid native ID format: %s", request.NativeID),
				NativeID:        request.NativeID,
			},
		}, nil
	}

	project := parts[0]
	volumeID := parts[1]
	instanceID := parts[2]

	// Build detach URL
	url := fmt.Sprintf("/cloud/project/%s/volume/%s/detach", project, volumeID)

	// Build request body
	body := map[string]interface{}{
		"instanceId": instanceID,
	}

	_, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			// 404 is success for delete (already detached)
			if transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.DeleteResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationDelete,
						OperationStatus: resource.OperationStatusSuccess,
						NativeID:        request.NativeID,
					},
				}, nil
			}
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       ovhtransport.ToResourceErrorCode(transportErr.Code),
					StatusMessage:   transportErr.Message,
					NativeID:        request.NativeID,
				},
			}, nil
		}
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   err.Error(),
				NativeID:        request.NativeID,
			},
		}, nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

// List is not supported for volume attachments.
func (p *volumeAttachmentProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return &resource.ListResult{
		NativeIDs: nil,
	}, nil
}

// Status returns success immediately (no async operations).
func (p *volumeAttachmentProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

// extractProject extracts the project/serviceName from target config.
func extractProject(targetConfig json.RawMessage) string {
	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}

	projectFields := []string{"ProjectId", "projectId", "ServiceName", "serviceName"}
	for _, field := range projectFields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func init() {
	// Register VolumeAttachment with custom provisioner
	registry.Register(
		VolumeAttachmentResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationDelete,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &volumeAttachmentProvisioner{client: client}
		},
	)
}
