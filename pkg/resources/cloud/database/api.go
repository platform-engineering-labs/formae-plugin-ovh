// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
)

// DatabaseAPI defines the API configuration for OVH Database services.
// Database resources have a unique path structure with engine in the URL.
var DatabaseAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: databasePathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// DatabaseOperations defines operation behavior for database resources.
var DatabaseOperations = base.OperationConfig{
	Synchronous: true, // Database operations return synchronously (status polling via Status)
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		var resourceID string
		if id, ok := response["id"].(string); ok {
			resourceID = id
		}
		if resourceID == "" {
			return ""
		}

		// Format: project/engine/clusterId for Service
		// Format: project/engine/clusterId/resourceId for nested resources
		if ctx.Project != "" && ctx.Engine != "" && ctx.ParentResource != "" {
			return fmt.Sprintf("%s/%s/%s/%s", ctx.Project, ctx.Engine, ctx.ParentResource, resourceID)
		}
		if ctx.Project != "" && ctx.Engine != "" {
			return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.Engine, resourceID)
		}
		return resourceID
	},
}

// DatabaseNativeID defines native ID format for database Service: "project/engine/clusterId"
var DatabaseNativeID = base.NativeIDConfig{
	Format: base.SimpleNameFormat, // We use custom parser
	Parser: func(nativeID string) (base.PathContext, error) {
		// Expect "project/engine/clusterId" format
		parts := strings.SplitN(nativeID, "/", 3)
		if len(parts) != 3 {
			return base.PathContext{}, fmt.Errorf("invalid database service native ID: %s", nativeID)
		}
		return base.PathContext{
			Project:      parts[0],
			Engine:       parts[1],
			ResourceName: parts[2],
		}, nil
	},
}

// DatabaseNestedNativeID defines native ID format for nested resources: "project/engine/clusterId/resourceId"
var DatabaseNestedNativeID = base.NativeIDConfig{
	Format: base.SimpleNameFormat, // We use custom parser
	Parser: func(nativeID string) (base.PathContext, error) {
		// Expect "project/engine/clusterId/resourceId" format
		parts := strings.SplitN(nativeID, "/", 4)
		if len(parts) != 4 {
			return base.PathContext{}, fmt.Errorf("invalid database nested native ID: %s", nativeID)
		}
		return base.PathContext{
			Project:        parts[0],
			Engine:         parts[1],
			ParentResource: parts[2], // clusterId
			ResourceName:   parts[3],
		}, nil
	},
}

// databasePathBuilder builds paths for database resources.
// Service:  /cloud/project/{project}/database/{engine}[/{clusterId}]
// Nested:   /cloud/project/{project}/database/{engine}/{clusterId}/{resourceType}[/{resourceId}]
func databasePathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s/database", ctx.Project)

	// Add engine
	if ctx.Engine != "" {
		path += "/" + ctx.Engine
	}

	// For Service resource (no parent), the clusterId goes directly after engine
	// For nested resources, clusterId is ParentResource
	if ctx.ParentResource != "" {
		// Nested resource: /database/{engine}/{clusterId}/{resourceType}[/{resourceId}]
		path += "/" + ctx.ParentResource + "/" + ctx.ResourceType
	}

	// Add resource ID for single resource operations
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}
