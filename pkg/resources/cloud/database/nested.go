// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// NestedResourceConfig configures a nested database resource.
type NestedResourceConfig struct {
	// PathSegment is the resource type in the URL (e.g., "database", "user", "topic")
	PathSegment string
	// IDField is the field name for the resource ID in responses (default "id")
	IDField string
	// FixedEngine restricts this resource to a specific engine (e.g., "kafka", "postgresql")
	FixedEngine string
	// SupportsUpdate indicates if PUT is supported
	SupportsUpdate bool
	// StripFields are fields to remove from request body (in URL path)
	StripFields []string
}

// nestedProvisioner handles nested database resource operations.
// Path: /cloud/project/{project}/database/{engine}/{clusterId}/{resourceType}[/{resourceId}]
type nestedProvisioner struct {
	client *ovhtransport.Client
	config NestedResourceConfig
}

var _ prov.Provisioner = &nestedProvisioner{}

func newNestedProvisioner(client *ovhtransport.Client, config NestedResourceConfig) *nestedProvisioner {
	if config.IDField == "" {
		config.IDField = "id"
	}
	if config.StripFields == nil {
		config.StripFields = []string{"serviceName", "engine", "clusterId"}
	}
	return &nestedProvisioner{client: client, config: config}
}

func (p *nestedProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	engine := p.getEngine(props)
	clusterID := resolveString(props["clusterId"])

	if project == "" || engine == "" || clusterID == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName, engine, and clusterId are required"), nil
	}

	// Build URL: POST /cloud/project/{project}/database/{engine}/{clusterId}/{resourceType}
	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s",
		project, engine, clusterID, p.config.PathSegment)

	body := filterProps(props, p.config.StripFields...)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return handleTransportError(err), nil
	}

	// Extract resource ID
	resourceID := resolveString(response.Body[p.config.IDField])
	if resourceID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			fmt.Sprintf("no %s in response", p.config.IDField)), nil
	}

	// Native ID: project/engine/clusterId/resourceId
	nativeID := fmt.Sprintf("%s/%s/%s/%s", project, engine, clusterID, resourceID)

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

func (p *nestedProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, engine, clusterID, resourceID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, engine, clusterID, p.config.PathSegment, resourceID)

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

func (p *nestedProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	if !p.config.SupportsUpdate {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeNotUpdatable,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, engine, clusterID, resourceID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, engine, clusterID, p.config.PathSegment, resourceID)

	body := filterProps(props, p.config.StripFields...)

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

func (p *nestedProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, engine, clusterID, resourceID, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, engine, clusterID, p.config.PathSegment, resourceID)

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

func (p *nestedProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	engine := p.getEngineFromAdditional(request.AdditionalProperties)
	clusterID := request.AdditionalProperties["clusterId"]

	if project == "" || engine == "" || clusterID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s",
		project, engine, clusterID, p.config.PathSegment)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if id, ok := item.(string); ok {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s/%s", project, engine, clusterID, id))
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *nestedProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// Nested resources don't need status polling by default
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

// getEngine returns the engine from props or the fixed engine
func (p *nestedProvisioner) getEngine(props map[string]interface{}) string {
	if p.config.FixedEngine != "" {
		return p.config.FixedEngine
	}
	return resolveString(props["engine"])
}

// getEngineFromAdditional returns the engine from additional props or the fixed engine
func (p *nestedProvisioner) getEngineFromAdditional(additionalProps map[string]string) string {
	if p.config.FixedEngine != "" {
		return p.config.FixedEngine
	}
	return additionalProps["engine"]
}

// resolveString converts interface{} to string
func resolveString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
