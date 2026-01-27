// pkg/cfres/base/base_resource.go
package base

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// TransportClient interface for API calls
type TransportClient interface {
	Do(ctx context.Context, opts ovhtransport.RequestOptions) (*ovhtransport.Response, error)
}

// BaseResource provides unified CRUD operations
type BaseResource struct {
	APIConfig           APIConfig
	OperationConfig     OperationConfig
	ResourceConfig      ResourceConfig
	NativeIDConfig      NativeIDConfig
	RequestTransformer  RequestTransformer
	ResponseTransformer ResponseTransformer
	StatusChecker       StatusChecker
	Client              TransportClient
}

// Create performs a CREATE operation
func (b *BaseResource) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	pathCtx := b.buildPathContext(request.TargetConfig, props)

	// Validate required path context fields
	if pathCtx.Project == "" {
		return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
			"project/serviceName is required but not found in target config or properties"), nil
	}
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent && pathCtx.ParentResource == "" {
		propName := b.ResourceConfig.ParentResource.PropertyName
		return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("parent resource ID required: property %q is empty or not a valid ID", propName)), nil
	}

	body := props
	if b.RequestTransformer != nil {
		transformCtx := b.buildTransformContext(ctx, pathCtx, resource.OperationCreate)
		var err error
		body, err = b.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	// Filter nil values - OVH API rejects null for optional fields
	filteredBody := filterNilValues(body)

	response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   filteredBody,
	})
	if err != nil {
		return b.handleTransportError(err, resource.OperationCreate, ""), nil
	}

	// Handle async operations if configured
	responseBody := response.Body
	if !b.OperationConfig.Synchronous && b.OperationConfig.OperationIDExtractor != nil {
		operationID := b.OperationConfig.OperationIDExtractor(response.Body)
		if operationID != "" {
			// This is an async operation - poll until complete
			completedOperation, err := b.pollOperation(ctx, pathCtx, operationID)
			if err != nil {
				return b.createFailureResult(resource.OperationErrorCodeServiceInternalError,
					fmt.Sprintf("operation failed: %v", err)), nil
			}

			// Extract the resource ID from the completed operation
			resourceID, _ := completedOperation["resourceId"].(string)
			if resourceID != "" {
				// Fetch the actual resource to get its properties
				resourceURL := b.APIConfig.PathBuilder(PathContext{
					Project:      pathCtx.Project,
					Region:       pathCtx.Region,
					ResourceType: pathCtx.ResourceType,
					ResourceName: resourceID,
				})
				resourceResponse, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
					Method: "GET",
					Path:   resourceURL,
				})
				if err == nil {
					responseBody = resourceResponse.Body
				} else {
					// Fall back to operation response if fetch fails
					responseBody = completedOperation
				}
			} else {
				// No resourceId, use operation response
				responseBody = completedOperation
			}
		}
	}

	// Extract native ID
	nativeID := ""
	if b.OperationConfig.NativeIDExtractor != nil {
		nativeID = b.OperationConfig.NativeIDExtractor(responseBody, pathCtx)
	} else if id, ok := responseBody["id"]; ok {
		nativeID = BuildNativeID(b.NativeIDConfig, PathContext{
			Zone:         pathCtx.Zone,
			Project:      pathCtx.Project,
			ResourceName: fmt.Sprintf("%v", id),
		})
	}

	// Execute post-mutation hook (e.g., zone refresh)
	if b.OperationConfig.PostMutationHook != nil {
		if err := b.OperationConfig.PostMutationHook(pathCtx); err != nil {
			// Log but don't fail - resource was created
		}
	}

	// Transform response
	responseProps := responseBody
	if b.ResponseTransformer != nil {
		transformCtx := b.buildTransformContext(ctx, pathCtx, resource.OperationCreate)
		responseProps = b.ResponseTransformer.Transform(responseProps, transformCtx)
	}

	propsJSON, _ := json.Marshal(responseProps)

	// If resource has a StatusChecker, return InProgress to trigger status polling
	// This allows async resources (like PrivateNetwork region activation) to complete
	operationStatus := resource.OperationStatusSuccess
	if b.StatusChecker != nil {
		operationStatus = resource.OperationStatusInProgress
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    operationStatus,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

// Read performs a READ operation
func (b *BaseResource) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Set parent type for nested resources (ParentResource already set by ParseNativeID)
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		pathCtx.ParentType = b.ResourceConfig.ParentResource.ParentType
	}

	// Extract project from target config if not already set (for SimpleNameFormat native IDs)
	if pathCtx.Project == "" && len(request.TargetConfig) > 0 {
		pathCtx.Project = extractProjectFromTargetConfig(request.TargetConfig)
	}

	// Extract region from target config for regional resources
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		if len(request.TargetConfig) > 0 {
			pathCtx.Region = extractRegionFromTargetConfig(request.TargetConfig)
		}
	}

	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeServiceInternalError,
		}, nil
	}

	responseProps := response.Body
	if b.ResponseTransformer != nil {
		transformCtx := b.buildTransformContext(ctx, pathCtx, resource.OperationRead)
		responseProps = b.ResponseTransformer.Transform(responseProps, transformCtx)
	}

	propsJSON, _ := json.Marshal(responseProps)

	return &resource.ReadResult{
		Properties: string(propsJSON),
	}, nil
}

// Update performs an UPDATE operation
func (b *BaseResource) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	if !b.ResourceConfig.SupportsUpdate {
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
		return b.updateFailureResult(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return b.updateFailureResult(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Set parent type for nested resources (ParentResource already set by ParseNativeID)
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		pathCtx.ParentType = b.ResourceConfig.ParentResource.ParentType
	}

	// Extract project from target config if not already set (for SimpleNameFormat native IDs)
	if pathCtx.Project == "" && len(request.TargetConfig) > 0 {
		pathCtx.Project = extractProjectFromTargetConfig(request.TargetConfig)
	}

	// Extract region from target config for regional resources
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		if len(request.TargetConfig) > 0 {
			pathCtx.Region = extractRegionFromTargetConfig(request.TargetConfig)
		}
	}

	body := props
	if b.RequestTransformer != nil {
		transformCtx := b.buildTransformContext(ctx, pathCtx, resource.OperationUpdate)
		body, err = b.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return b.updateFailureResult(request.NativeID, resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	method := "PUT"
	if b.ResourceConfig.UpdateMethod == UpdateMethodPatch {
		method = "PATCH"
	}

	// Filter nil values - OVH API rejects null for optional fields
	filteredBody := filterNilValues(body)

	response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: method,
		Path:   url,
		Body:   filteredBody,
	})
	if err != nil {
		return b.handleTransportErrorUpdate(err, request.NativeID), nil
	}

	// Execute post-mutation hook
	if b.OperationConfig.PostMutationHook != nil {
		_ = b.OperationConfig.PostMutationHook(pathCtx)
	}

	responseProps := response.Body
	if b.ResponseTransformer != nil {
		transformCtx := b.buildTransformContext(ctx, pathCtx, resource.OperationUpdate)
		responseProps = b.ResponseTransformer.Transform(responseProps, transformCtx)
	}

	propsJSON, _ := json.Marshal(responseProps)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

// Delete performs a DELETE operation
func (b *BaseResource) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return b.deleteFailureResult(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Set parent type for nested resources (ParentResource already set by ParseNativeID)
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		pathCtx.ParentType = b.ResourceConfig.ParentResource.ParentType
	}

	// Extract project from target config if not already set (for SimpleNameFormat native IDs)
	if pathCtx.Project == "" && len(request.TargetConfig) > 0 {
		pathCtx.Project = extractProjectFromTargetConfig(request.TargetConfig)
	}

	// Extract region from target config for regional resources
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		if len(request.TargetConfig) > 0 {
			pathCtx.Region = extractRegionFromTargetConfig(request.TargetConfig)
		}
	}

	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	_, err = b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "DELETE",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			// 404 is success for delete
			if transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.DeleteResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationDelete,
						OperationStatus: resource.OperationStatusSuccess,
						NativeID:        request.NativeID,
					},
				}, nil
			}
			return b.deleteFailureResult(request.NativeID,
				ovhtransport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
		}
		return b.deleteFailureResult(request.NativeID,
			resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Execute post-mutation hook
	if b.OperationConfig.PostMutationHook != nil {
		_ = b.OperationConfig.PostMutationHook(pathCtx)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

// List performs a LIST operation
func (b *BaseResource) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	pathCtx := b.buildPathContextFromAdditionalProps(request.TargetConfig, request.AdditionalProperties)
	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	// OVH API returns either array of IDs or array of objects for list operations
	var nativeIDs []string
	for _, item := range response.BodyArray {
		var id string
		switch v := item.(type) {
		case string:
			// Direct ID string
			id = v
		case map[string]interface{}:
			// Object with id field (e.g., SWIFT storage containers)
			if idVal, ok := v["id"].(string); ok {
				id = idVal
			} else {
				// Fallback to string representation
				id = fmt.Sprintf("%v", item)
			}
		default:
			id = fmt.Sprintf("%v", item)
		}
		nativeID := BuildNativeID(b.NativeIDConfig, PathContext{
			Zone:         pathCtx.Zone,
			Project:      pathCtx.Project,
			ResourceName: id,
		})
		nativeIDs = append(nativeIDs, nativeID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}

// Status checks operation status
func (b *BaseResource) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// If no StatusChecker is configured, resource is immediately ready
	if b.StatusChecker == nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusSuccess,
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	// Parse native ID to read the resource
	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("invalid native ID: %v", err),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Set parent type for nested resources (ParentResource already set by ParseNativeID)
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		pathCtx.ParentType = b.ResourceConfig.ParentResource.ParentType
	}

	// Extract project from target config if not already set (for SimpleNameFormat native IDs)
	if pathCtx.Project == "" && len(request.TargetConfig) > 0 {
		pathCtx.Project = extractProjectFromTargetConfig(request.TargetConfig)
	}

	// Extract region from target config for regional resources
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		if len(request.TargetConfig) > 0 {
			pathCtx.Region = extractRegionFromTargetConfig(request.TargetConfig)
		}
	}

	// Read the resource
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	// Validate URL to catch configuration issues early
	if url == "" || url == "/" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("invalid URL built from native ID %q: project=%q, parent=%q, name=%q", request.NativeID, pathCtx.Project, pathCtx.ParentResource, pathCtx.ResourceName),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationCheckStatus,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       ovhtransport.ToResourceErrorCode(transportErr.Code),
					StatusMessage:   transportErr.Message,
					RequestID:       request.RequestID,
					NativeID:        request.NativeID,
				},
			}, nil
		}
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   err.Error(),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	// Check if resource is ready using the StatusChecker
	ready, err := b.StatusChecker(response.Body)
	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   fmt.Sprintf("status check failed: %v", err),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	if !ready {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   "Resource is not yet ready",
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	// Resource is ready
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

// Helper methods

func (b *BaseResource) buildPathContext(targetConfig json.RawMessage, props map[string]interface{}) PathContext {
	ctx := PathContext{
		ResourceType: b.ResourceConfig.ResourceType,
	}

	// Extract zone from properties (OVH DNS resources)
	if zone, ok := props["zone"].(string); ok {
		ctx.Zone = zone
	}

	// Extract serviceName/project from properties (OVH Cloud resources)
	if serviceName, ok := props["serviceName"].(string); ok && serviceName != "" {
		ctx.Project = serviceName
	}

	// Try target config if not found in props
	if ctx.Project == "" && len(targetConfig) > 0 {
		ctx.Project = extractProjectFromTargetConfig(targetConfig)
	}

	// Extract region for regional resources
	// ScopeRegional resources need region in the URL path and native ID
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		// First try to get region from properties (e.g., SubnetPrivate specifies region directly)
		if region, ok := props["region"].(string); ok && region != "" {
			ctx.Region = region
		} else if len(targetConfig) > 0 {
			// Fall back to target config
			ctx.Region = extractRegionFromTargetConfig(targetConfig)
		}
	}

	// Extract parent resource ID if this is a nested resource
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		ctx.ParentType = b.ResourceConfig.ParentResource.ParentType
		propName := b.ResourceConfig.ParentResource.PropertyName
		if val := props[propName]; val != nil {
			switch v := val.(type) {
			case string:
				if v != "" {
					ctx.ParentResource = v
				}
			case float64:
				// JSON numbers are decoded as float64
				ctx.ParentResource = fmt.Sprintf("%.0f", v)
			default:
				// Fallback: convert to string representation
				ctx.ParentResource = fmt.Sprintf("%v", v)
			}
		}
	}

	// Extract custom segments for complex nested paths (e.g., /network/{networkId}/subnet/{subnetId}/gateway)
	if b.ResourceConfig.CustomSegmentsConfig != nil {
		for _, propName := range b.ResourceConfig.CustomSegmentsConfig.PropertyNames {
			if value, ok := props[propName].(string); ok && value != "" {
				ctx.CustomSegments = append(ctx.CustomSegments, value)
			}
		}
	}

	return ctx
}

func (b *BaseResource) buildPathContextFromAdditionalProps(targetConfig json.RawMessage, additionalProps map[string]string) PathContext {
	ctx := PathContext{
		ResourceType: b.ResourceConfig.ResourceType,
	}

	// Extract zone from additional properties (OVH DNS resources)
	if zone, ok := additionalProps["zone"]; ok {
		ctx.Zone = zone
	}

	// Extract serviceName/project from additional properties (OVH Cloud resources)
	if serviceName, ok := additionalProps["serviceName"]; ok && serviceName != "" {
		ctx.Project = serviceName
	}

	// Try target config if not found in additional props
	if ctx.Project == "" && len(targetConfig) > 0 {
		ctx.Project = extractProjectFromTargetConfig(targetConfig)
	}

	// Extract region from target config only for regional resources
	// ScopeRegional resources need region in the URL path
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional {
		if len(targetConfig) > 0 {
			ctx.Region = extractRegionFromTargetConfig(targetConfig)
		}
	}

	// Extract parent resource ID if this is a nested resource
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		ctx.ParentType = b.ResourceConfig.ParentResource.ParentType
		propName := b.ResourceConfig.ParentResource.PropertyName
		if parentID, ok := additionalProps[propName]; ok && parentID != "" {
			ctx.ParentResource = parentID
		}
	}

	return ctx
}

// filterNilValues removes nil values from a map recursively.
// OVH API rejects null values for optional fields - they should be omitted entirely.
func filterNilValues(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if v == nil {
			continue
		}
		// Recursively filter nested maps
		if nested, ok := v.(map[string]interface{}); ok {
			filtered := filterNilValues(nested)
			if len(filtered) > 0 {
				result[k] = filtered
			}
			continue
		}
		// Filter nil values from slices of maps
		if slice, ok := v.([]interface{}); ok {
			var filtered []interface{}
			for _, item := range slice {
				if item == nil {
					continue
				}
				if nested, ok := item.(map[string]interface{}); ok {
					filtered = append(filtered, filterNilValues(nested))
				} else {
					filtered = append(filtered, item)
				}
			}
			if len(filtered) > 0 {
				result[k] = filtered
			}
			continue
		}
		result[k] = v
	}
	return result
}

// extractProjectFromTargetConfig extracts project/serviceName from target config JSON.
// Checks multiple field names to support different naming conventions.
func extractProjectFromTargetConfig(targetConfig json.RawMessage) string {
	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}

	// Check various field names for project ID (PascalCase and camelCase)
	projectFields := []string{"ProjectId", "projectId", "ServiceName", "serviceName"}
	for _, field := range projectFields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

// extractRegionFromTargetConfig extracts region from target config JSON.
// Checks multiple field names to support different naming conventions.
func extractRegionFromTargetConfig(targetConfig json.RawMessage) string {
	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}

	// Check various field names for region (PascalCase and camelCase)
	regionFields := []string{"Region", "region", "RegionName", "regionName"}
	for _, field := range regionFields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

// pollOperation polls an async operation until completion
func (b *BaseResource) pollOperation(ctx context.Context, pathCtx PathContext, operationID string) (map[string]interface{}, error) {
	if b.OperationConfig.OperationURLBuilder == nil || b.OperationConfig.OperationStatusChecker == nil {
		return nil, fmt.Errorf("operation polling not configured")
	}

	operationURL := b.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	// Poll with exponential backoff: 2s, 4s, 8s, ... up to 30s, max 5 minutes total
	maxWait := 5 * time.Minute
	startTime := time.Now()
	pollInterval := 2 * time.Second

	for {
		if time.Since(startTime) > maxWait {
			return nil, fmt.Errorf("operation timed out after %v", maxWait)
		}

		time.Sleep(pollInterval)

		response, err := b.Client.Do(ctx, ovhtransport.RequestOptions{
			Method: "GET",
			Path:   operationURL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to poll operation: %w", err)
		}

		done, err := b.OperationConfig.OperationStatusChecker(response.Body)
		if err != nil {
			return nil, err
		}
		if done {
			return response.Body, nil
		}

		// Increase poll interval with exponential backoff, max 30s
		pollInterval = pollInterval * 2
		if pollInterval > 30*time.Second {
			pollInterval = 30 * time.Second
		}
	}
}

func (b *BaseResource) buildTransformContext(ctx context.Context, pathCtx PathContext, operation resource.Operation) TransformContext {
	return TransformContext{
		Project:      pathCtx.Project,
		Zone:         pathCtx.Zone,
		ResourceType: pathCtx.ResourceType,
		Operation:    operation,
		Client:       b.Client,
		Ctx:          ctx,
	}
}

func (b *BaseResource) createFailureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

func (b *BaseResource) updateFailureResult(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

func (b *BaseResource) deleteFailureResult(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

func (b *BaseResource) handleTransportError(err error, operation resource.Operation, nativeID string) *resource.CreateResult {
	if transportErr, ok := err.(*ovhtransport.Error); ok {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       operation,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       ovhtransport.ToResourceErrorCode(transportErr.Code),
				StatusMessage:   transportErr.Message,
				NativeID:        nativeID,
			},
		}
	}
	return b.createFailureResult(resource.OperationErrorCodeServiceInternalError, err.Error())
}

func (b *BaseResource) handleTransportErrorUpdate(err error, nativeID string) *resource.UpdateResult {
	if transportErr, ok := err.(*ovhtransport.Error); ok {
		return b.updateFailureResult(nativeID, ovhtransport.ToResourceErrorCode(transportErr.Code), transportErr.Message)
	}
	return b.updateFailureResult(nativeID, resource.OperationErrorCodeServiceInternalError, err.Error())
}
