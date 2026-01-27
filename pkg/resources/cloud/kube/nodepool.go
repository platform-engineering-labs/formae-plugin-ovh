// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// NodePoolResourceType is the resource type for Kubernetes node pools.
const NodePoolResourceType = "OVH::Kube::NodePool"

// nodePoolProvisioner handles Kubernetes node pool operations.
// Path: /cloud/project/{project}/kube/{kubeId}/nodepool[/{nodePoolId}]
type nodePoolProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &nodePoolProvisioner{}

func (p *nodePoolProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	kubeID := resolveString(props["kubeId"])

	if project == "" || kubeID == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName and kubeId are required"), nil
	}

	// Build URL: POST /cloud/project/{project}/kube/{kubeId}/nodepool
	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool", project, kubeID)

	// Strip serviceName and kubeId from body
	body := filterProps(props, "serviceName", "kubeId")

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return handleTransportError(err), nil
	}

	// Extract node pool ID
	nodePoolID, _ := response.Body["id"].(string)
	if nodePoolID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			"no node pool ID in response"), nil
	}

	// Native ID: project/kubeId/nodePoolId
	nativeID := fmt.Sprintf("%s/%s/%s", project, kubeID, nodePoolID)

	propsJSON, _ := json.Marshal(response.Body)

	// Return InProgress - node pool creation is async
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusInProgress,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *nodePoolProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, kubeID, nodePoolID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool/%s", project, kubeID, nodePoolID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
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

func (p *nodePoolProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, kubeID, nodePoolID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool/%s", project, kubeID, nodePoolID)

	// Strip immutable fields
	body := filterProps(props, "serviceName", "kubeId", "name", "flavorName",
		"antiAffinity", "monthlyBilled", "availabilityZones")

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

func (p *nodePoolProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, kubeID, nodePoolID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool/%s", project, kubeID, nodePoolID)

	_, err = p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "DELETE",
		Path:   url,
	})
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
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *nodePoolProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	kubeID := request.AdditionalProperties["kubeId"]

	if project == "" || kubeID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool", project, kubeID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list node pools: %w", err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if id, ok := item.(string); ok {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, kubeID, id))
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *nodePoolProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	project, kubeID, nodePoolID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return statusFailure(request, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/nodepool/%s", project, kubeID, nodePoolID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return statusFailure(request, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return statusFailure(request, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Check if node pool is READY
	status, _ := response.Body["status"].(string)
	if status != "READY" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   fmt.Sprintf("NodePool status: %s", status),
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

func init() {
	registry.Register(
		NodePoolResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &nodePoolProvisioner{client: client}
		},
	)
}
