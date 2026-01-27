// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// FloatingIP has a special path structure:
// - Create: POST /cloud/project/{serviceName}/region/{regionName}/instance/{instanceId}/floatingIp
// - Read:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
// - Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
// - List:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip

// floatingIPPathBuilder handles the special case where Create is nested under instance.
func floatingIPPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s", ctx.Project)

	// Add region (required for all floating IP operations)
	if ctx.Region != "" {
		path += fmt.Sprintf("/region/%s", ctx.Region)
	}

	// For Create (collection URL with ParentResource set): use instance path
	// POST /cloud/project/{serviceName}/region/{regionName}/instance/{instanceId}/floatingIp
	if ctx.ResourceName == "" && ctx.ParentResource != "" {
		return path + fmt.Sprintf("/instance/%s/floatingIp", ctx.ParentResource)
	}

	// For List (collection URL without ParentResource): use floatingip path
	// GET /cloud/project/{serviceName}/region/{regionName}/floatingip
	if ctx.ResourceName == "" {
		return path + "/floatingip"
	}

	// For Read/Update/Delete (resource URL): use floatingip/{id} path
	// GET/DELETE /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
	return path + fmt.Sprintf("/floatingip/%s", ctx.ResourceName)
}

// FloatingIPAPI defines API config for floating IPs with custom path builder.
var FloatingIPAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: floatingIPPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// FloatingIPOperations defines operation behavior for floating IPs.
// Native ID format: project/region/floatingIpId (regional resource).
var FloatingIPOperations = base.OperationConfig{
	Synchronous: true, // Floating IP operations are synchronous
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		// Extract the floating IP ID from response
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

// FloatingIPNativeID defines native ID format for floating IPs: "project/region/resourceId"
var FloatingIPNativeID = base.NativeIDConfig{
	Format: base.ProjectRegionalFormat,
}

// floatingIPRequestTransformer strips instance_id from the request body.
// instance_id is used in the URL path for Create operations.
type floatingIPRequestTransformer struct{}

func (t *floatingIPRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range props {
		// Strip instance_id - it's used in the URL path, not the body
		if k == "instance_id" {
			continue
		}
		result[k] = v
	}
	return result, nil
}

var floatingIPTransformer = &floatingIPRequestTransformer{}

// floatingIPRegistry is separate from cloudNetworkRegistry to use custom API config.
var floatingIPRegistry *base.ResourceRegistry

func init() {
	// Create a separate registry for FloatingIP with custom path builder
	floatingIPRegistry = base.NewResourceRegistry(
		FloatingIPAPI,
		FloatingIPOperations,
		FloatingIPNativeID, // Uses project/region/resourceId format
	)

	// FloatingIP (OVH Cloud Floating IP)
	// Create: POST /cloud/project/{serviceName}/region/{regionName}/instance/{instanceId}/floatingIp
	// Read:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
	// Delete: DELETE /cloud/project/{serviceName}/region/{regionName}/floatingip/{floatingIpId}
	// List:   GET /cloud/project/{serviceName}/region/{regionName}/floatingip
	err := floatingIPRegistry.Register(base.ResourceDefinition{
		ResourceType: FloatingIPResourceType,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "floatingip", // Base type for path construction
			Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
			// ParentResource is used ONLY for Create path (instance/{instanceId})
			// It's NOT included in native ID since Read/Delete don't need it
			ParentResource: &base.ParentResourceConfig{
				RequiresParent: true,
				ParentType:     "instance",
				PropertyName:   "instance_id",
			},
			SupportsUpdate: false, // OVH floating IPs are not updatable via this API
		},
		// Strip instance_id from request body (used in URL path)
		RequestTransformer: floatingIPTransformer,
		Operations: []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
	})

	if err != nil {
		panic(err)
	}
}
