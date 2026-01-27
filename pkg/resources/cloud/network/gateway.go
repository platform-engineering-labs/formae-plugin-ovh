// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Gateway has a special path structure:
// - Create: POST /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet/{subnetId}/gateway
// - Read:   GET /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
// - Update: PUT /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
// - Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
// - List:   GET /cloud/project/{serviceName}/region/{regionName}/gateway

// gatewayPathBuilder handles the special case where Create is nested under network/subnet.
func gatewayPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s", ctx.Project)

	// Add region (required for all gateway operations)
	if ctx.Region != "" {
		path += fmt.Sprintf("/region/%s", ctx.Region)
	}

	// For Create (collection URL with CustomSegments set): use network/subnet path
	// POST /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet/{subnetId}/gateway
	if ctx.ResourceName == "" && len(ctx.CustomSegments) >= 2 {
		networkID := ctx.CustomSegments[0]
		subnetID := ctx.CustomSegments[1]
		return path + fmt.Sprintf("/network/%s/subnet/%s/gateway", networkID, subnetID)
	}

	// For List (collection URL without CustomSegments): use gateway path
	// GET /cloud/project/{serviceName}/region/{regionName}/gateway
	if ctx.ResourceName == "" {
		return path + "/gateway"
	}

	// For Read/Update/Delete (resource URL): use gateway/{id} path
	// GET/PUT/DELETE /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
	return path + fmt.Sprintf("/gateway/%s", ctx.ResourceName)
}

// GatewayAPI defines API config for gateways with custom path builder.
var GatewayAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: gatewayPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// GatewayOperations defines operation behavior for gateways.
// Native ID format: project/region/gatewayId (regional resource).
var GatewayOperations = base.OperationConfig{
	Synchronous: false, // Gateway creation is async
	// OperationIDExtractor extracts the operation ID from an async response
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
			return false, nil
		}
	},
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		// Extract the gateway ID from response
		if id, ok := response["id"].(string); ok {
			if ctx.Project != "" && ctx.Region != "" {
				return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.Region, id)
			}
			if ctx.Project != "" {
				return fmt.Sprintf("%s/%s", ctx.Project, id)
			}
			return id
		}
		return ""
	},
}

// GatewayNativeID defines native ID format for gateways: "project/region/resourceId"
var GatewayNativeID = base.NativeIDConfig{
	Format: base.ProjectRegionalFormat,
}

// gatewayRequestTransformer strips network_id and subnet_id from the request body.
// These are used in the URL path for Create operations.
type gatewayRequestTransformer struct{}

func (t *gatewayRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range props {
		// Strip network_id and subnet_id - they're used in the URL path, not the body
		if k == "network_id" || k == "subnet_id" {
			continue
		}
		result[k] = v
	}
	return result, nil
}

var gatewayTransformer = &gatewayRequestTransformer{}

// gatewayRegistry is separate from cloudNetworkRegistry to use custom API config.
var gatewayRegistry *base.ResourceRegistry

func init() {
	// Create a separate registry for Gateway with custom path builder
	gatewayRegistry = base.NewResourceRegistry(
		GatewayAPI,
		GatewayOperations,
		GatewayNativeID,
	)

	// Gateway (OVH Cloud Gateway for private networks)
	// Create: POST /cloud/project/{serviceName}/region/{regionName}/network/{networkId}/subnet/{subnetId}/gateway
	// Read:   GET /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
	// Update: PUT /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
	// Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/gateway/{gatewayId}
	// List:   GET /cloud/project/{serviceName}/region/{regionName}/gateway
	err := gatewayRegistry.Register(base.ResourceDefinition{
		ResourceType: GatewayResourceType,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "gateway",
			Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
			// CustomSegments will be populated with [networkId, subnetId] for Create
			CustomSegmentsConfig: &base.CustomSegmentsConfig{
				PropertyNames: []string{"network_id", "subnet_id"},
			},
			SupportsUpdate: true, // Name and model can be updated
			UpdateMethod:   base.UpdateMethodPut,
		},
		// Strip network_id and subnet_id from request body (used in URL path)
		RequestTransformer: gatewayTransformer,
		// Gateway creation is async - need to poll for status
		StatusChecker: gatewayStatusChecker,
		Operations: []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
	})

	if err != nil {
		panic(err)
	}
}
