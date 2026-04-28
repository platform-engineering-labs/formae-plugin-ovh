// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// engineClusterProvisioner manages a database cluster for a fixed engine
// (e.g. mysql, postgresql). The engine is baked into the URL and is not
// part of the native ID. Native ID format: "project/clusterId".
type engineClusterProvisioner struct {
	client *ovhtransport.Client
	engine string
}

var _ prov.Provisioner = &engineClusterProvisioner{}

func newEngineClusterProvisioner(client *ovhtransport.Client, engine string) *engineClusterProvisioner {
	return &engineClusterProvisioner{client: client, engine: engine}
}

func (p *engineClusterProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName is required"), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s", project, p.engine)

	body := filterProps(props, "serviceName", "id", "createdAt", "status", "networkType", "endpoints")
	normalizeServiceCreateBody(body)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return handleTransportError(err), nil
	}

	clusterID, _ := response.Body["id"].(string)
	if clusterID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			"no cluster ID in response"), nil
	}

	nativeID := fmt.Sprintf("%s/%s", project, clusterID)
	propsJSON, _ := json.Marshal(response.Body)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusInProgress,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *engineClusterProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, clusterID, err := parseEngineClusterNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s", project, p.engine, clusterID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: url})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}

	propsJSON, _ := json.Marshal(response.Body)
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

func (p *engineClusterProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, clusterID, err := parseEngineClusterNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s", project, p.engine, clusterID)
	body := serviceUpdateBody(props)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "PUT",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	propsJSON, _ := json.Marshal(response.Body)
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *engineClusterProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, clusterID, err := parseEngineClusterNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s", project, p.engine, clusterID)

	_, err = p.client.Do(ctx, ovhtransport.RequestOptions{Method: "DELETE", Path: url})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			if transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.DeleteResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationDelete,
						OperationStatus: resource.OperationStatusSuccess,
						NativeID:        request.NativeID,
					},
				}, nil
			}
			return deleteFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return deleteFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			RequestID:       deleteStatusRequestID(request.NativeID),
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *engineClusterProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	if project == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s", project, p.engine)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: url})
	if err != nil {
		return nil, fmt.Errorf("failed to list %s clusters: %w", p.engine, err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if id, ok := item.(string); ok {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s", project, id))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *engineClusterProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	project, clusterID, err := parseEngineClusterNativeID(request.NativeID)
	if err != nil {
		return statusFailure(request, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s", project, p.engine, clusterID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: url})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			if isDeleteStatusRequest(request) && transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.StatusResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationDelete,
						OperationStatus: resource.OperationStatusSuccess,
						RequestID:       request.RequestID,
						NativeID:        request.NativeID,
					},
				}, nil
			}
			return statusFailure(request, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return statusFailure(request, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	if isDeleteStatusRequest(request) {
		status, _ := response.Body["status"].(string)
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   fmt.Sprintf("Service deletion in progress; status: %s", status),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	status, _ := response.Body["status"].(string)
	if status != "READY" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   fmt.Sprintf("Service status: %s", status),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	propsJSON, _ := json.Marshal(response.Body)
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCheckStatus,
			OperationStatus:    resource.OperationStatusSuccess,
			RequestID:          request.RequestID,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

// parseEngineClusterNativeID parses "project/clusterId" format.
func parseEngineClusterNativeID(nativeID string) (project, clusterID string, err error) {
	parts := strings.SplitN(nativeID, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid cluster native ID: %s", nativeID)
	}
	return parts[0], parts[1], nil
}
