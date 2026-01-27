// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package registry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// UserResourceType is the resource type for registry users.
const UserResourceType = "OVH::Registry::User"

// userProvisioner handles registry user operations.
// Path: /cloud/project/{project}/containerRegistry/{registryId}/users[/{userId}]
type userProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &userProvisioner{}

func (p *userProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	registryID := resolveString(props["registryId"])

	if project == "" || registryID == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName and registryId are required"), nil
	}

	// Build URL: POST /cloud/project/{project}/containerRegistry/{registryId}/users
	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/users", project, registryID)

	// Strip serviceName and registryId from body
	body := filterProps(props, "serviceName", "registryId")

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return handleTransportError(err), nil
	}

	// Extract user ID
	userID, _ := response.Body["id"].(string)
	if userID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			"no user ID in response"), nil
	}

	// Native ID: project/registryId/userId
	nativeID := fmt.Sprintf("%s/%s/%s", project, registryID, userID)

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

func (p *userProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, registryID, userID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/users/%s", project, registryID, userID)

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

func (p *userProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Users are not updatable
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *userProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, registryID, userID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/users/%s", project, registryID, userID)

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

func (p *userProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	registryID := request.AdditionalProperties["registryId"]

	if project == "" || registryID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/users", project, registryID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if m, ok := item.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok {
				nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, registryID, id))
			}
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *userProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

func init() {
	registry.Register(
		UserResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &userProvisioner{client: client}
		},
	)
}
