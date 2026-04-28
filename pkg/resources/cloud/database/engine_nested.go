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

// EngineNestedConfig configures a nested resource within an engine-specific cluster.
type EngineNestedConfig struct {
	Engine         string   // e.g. "mysql", "postgresql"
	PathSegment    string   // e.g. "user", "database", "ipRestriction", "integration"
	IDField        string   // default "id"
	SupportsUpdate bool
	StripFields    []string // default {"serviceName", "clusterId"}
}

// engineNestedProvisioner manages nested resources under a specific engine cluster.
// Native ID format: "project/clusterId/resourceId".
type engineNestedProvisioner struct {
	client *ovhtransport.Client
	config EngineNestedConfig
}

var _ prov.Provisioner = &engineNestedProvisioner{}

func newEngineNestedProvisioner(client *ovhtransport.Client, config EngineNestedConfig) *engineNestedProvisioner {
	if config.IDField == "" {
		config.IDField = "id"
	}
	if config.StripFields == nil {
		config.StripFields = []string{"serviceName", "clusterId"}
	}
	return &engineNestedProvisioner{client: client, config: config}
}

func (p *engineNestedProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	clusterID := resolveString(props["clusterId"])

	if project == "" || clusterID == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName and clusterId are required"), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s",
		project, p.config.Engine, clusterID, p.config.PathSegment)

	body := filterProps(props, p.config.StripFields...)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return handleTransportError(err), nil
	}

	resourceID := resolveString(response.Body[p.config.IDField])
	if resourceID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			fmt.Sprintf("no %s in response", p.config.IDField)), nil
	}

	nativeID := fmt.Sprintf("%s/%s/%s", project, clusterID, resourceID)
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

func (p *engineNestedProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, clusterID, resourceID, err := parseEngineNestedNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, p.config.Engine, clusterID, p.config.PathSegment, resourceID)

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

func (p *engineNestedProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
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

	project, clusterID, resourceID, err := parseEngineNestedNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, p.config.Engine, clusterID, p.config.PathSegment, resourceID)

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

func (p *engineNestedProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, clusterID, resourceID, err := parseEngineNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s/%s",
		project, p.config.Engine, clusterID, p.config.PathSegment, resourceID)

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
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *engineNestedProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	clusterID := request.AdditionalProperties["clusterId"]

	if project == "" || clusterID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/database/%s/%s/%s",
		project, p.config.Engine, clusterID, p.config.PathSegment)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: url})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if id, ok := item.(string); ok {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, clusterID, id))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *engineNestedProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

// parseEngineNestedNativeID parses "project/clusterId/resourceId" format.
func parseEngineNestedNativeID(nativeID string) (project, clusterID, resourceID string, err error) {
	parts := strings.SplitN(nativeID, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid nested native ID: %s", nativeID)
	}
	return parts[0], parts[1], parts[2], nil
}
