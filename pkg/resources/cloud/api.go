// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloud

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
)

// CloudAPI defines the API configuration for OVH Cloud
var CloudAPI = base.APIConfig{
	BaseURL:     "", // go-ovh handles endpoint
	APIVersion:  "1.0",
	PathBuilder: cloudPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// CloudOperations defines operation behavior for cloud resources
var CloudOperations = base.OperationConfig{
	Synchronous: false, // Cloud operations may be async
	// OperationIDExtractor extracts the operation ID from an async response
	// OVH Cloud async operations have an "action" field
	OperationIDExtractor: func(response map[string]interface{}) string {
		// Only extract if this is an operation response (has "action" field)
		if _, hasAction := response["action"]; !hasAction {
			return ""
		}
		if id, ok := response["id"].(string); ok {
			return id
		}
		return ""
	},
	// OperationURLBuilder builds the URL to poll operation status
	OperationURLBuilder: func(ctx base.PathContext, operationID string) string {
		return fmt.Sprintf("/cloud/project/%s/operation/%s", ctx.Project, operationID)
	},
	// OperationStatusChecker checks if the operation is complete
	// Returns done=true when status is "completed" or "error"
	OperationStatusChecker: func(response map[string]interface{}) (done bool, err error) {
		status, _ := response["status"].(string)
		switch status {
		case "completed":
			return true, nil
		case "error":
			message, _ := response["message"].(string)
			if message == "" {
				message = "operation failed"
			}
			return true, fmt.Errorf("operation failed: %s", message)
		default:
			// "created", "in-progress", etc.
			return false, nil
		}
	},
	// NativeIDExtractor extracts the resource ID and builds a native ID.
	// For nested resources: project/parentId/resourceId
	// For top-level resources: project/resourceId
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		var resourceID string

		// Check if this is a completed operation (has resourceId)
		if rid, ok := response["resourceId"].(string); ok && rid != "" {
			resourceID = rid
		} else if id, ok := response["id"].(string); ok {
			// Fall back to direct "id" field for sync responses
			resourceID = id
		}

		if resourceID == "" {
			return ""
		}

		// For nested resources (e.g., Subnet), include parent in native ID
		// Format: project/parentId/resourceId
		if ctx.Project != "" && ctx.ParentResource != "" {
			return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.ParentResource, resourceID)
		}

		// For top-level resources: project/resourceId
		if ctx.Project != "" {
			return fmt.Sprintf("%s/%s", ctx.Project, resourceID)
		}

		return resourceID
	},
}

// CloudNativeID defines native ID format for cloud resources: "project/resourceId"
var CloudNativeID = base.NativeIDConfig{
	Format: base.ProjectHierarchicalFormat,
}

// cloudPathBuilder builds paths for cloud resources
// Supports:
// - Project-scoped resources: /cloud/project/{serviceName}/{resourceType}
// - Regional resources: /cloud/project/{serviceName}/region/{regionName}/{resourceType}
// - Nested resources under parent: /cloud/project/{serviceName}/{parentType}/{parentId}/{resourceType}
// - Regional nested resources: /cloud/project/{serviceName}/region/{regionName}/{parentType}/{parentId}/{resourceType}
func cloudPathBuilder(ctx base.PathContext) string {
	// Base path: /cloud/project/{serviceName}
	path := fmt.Sprintf("/cloud/project/%s", ctx.Project)

	// Add region for regional resources
	if ctx.Region != "" {
		path += fmt.Sprintf("/region/%s", ctx.Region)
	}

	// For nested resources: add parent path segment
	// Example: /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet
	// Example: /cloud/project/{serviceName}/network/private/{networkId}/subnet
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s/%s", ctx.ParentType, ctx.ParentResource, ctx.ResourceType)
	} else {
		// Top-level resources: /cloud/project/{serviceName}/{resourceType}
		// Or regional: /cloud/project/{serviceName}/region/{regionName}/{resourceType}
		path += "/" + ctx.ResourceType
	}

	// Add resource ID for single resource operations
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}
