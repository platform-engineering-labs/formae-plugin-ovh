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

// OidcResourceType is the resource type for registry OIDC configuration.
const OidcResourceType = "OVH::Registry::Oidc"

// oidcProvisioner handles registry OIDC configuration operations.
// This is a singleton resource per registry.
// Path: /cloud/project/{project}/containerRegistry/{registryId}/openIdConnect
type oidcProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &oidcProvisioner{}

func (p *oidcProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
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

	// Build URL: POST /cloud/project/{project}/containerRegistry/{registryId}/openIdConnect
	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/openIdConnect", project, registryID)

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

	// Native ID: project/registryId (singleton)
	nativeID := fmt.Sprintf("%s/%s", project, registryID)

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

func (p *oidcProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, registryID, err := parseRegistryNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/openIdConnect", project, registryID)

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

func (p *oidcProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, registryID, err := parseRegistryNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/openIdConnect", project, registryID)

	// Strip immutable fields
	body := filterProps(props, "serviceName", "registryId")

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

func (p *oidcProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, registryID, err := parseRegistryNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/openIdConnect", project, registryID)

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

func (p *oidcProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// OIDC is a singleton, List checks if it exists
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	registryID := request.AdditionalProperties["registryId"]

	if project == "" || registryID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	// Check if OIDC exists by reading it
	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/openIdConnect", project, registryID)

	_, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		// No OIDC configured
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	// OIDC exists
	return &resource.ListResult{
		NativeIDs: []string{fmt.Sprintf("%s/%s", project, registryID)},
	}, nil
}

func (p *oidcProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
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
		OidcResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &oidcProvisioner{client: client}
		},
	)
}
