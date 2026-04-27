// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// SubnetPrivate uses the private network API:
// - Create: POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
// - Read: GET /cloud/project/{serviceName}/network/private/{networkId}/subnet (list and filter)
// - Delete: DELETE /cloud/project/{serviceName}/network/private/{networkId}/subnet/{subnetId}

// subnetPathBuilder builds paths for the private network subnet API.
func subnetPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/cloud/project/%s/network/private", ctx.Project)

	// Add network ID
	if ctx.ParentResource != "" {
		path += "/" + ctx.ParentResource + "/subnet"
	}

	// Add subnet ID for Delete
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

// SubnetAPI defines API config for subnets with custom path builder.
var SubnetAPI = base.APIConfig{
	BaseURL:     "",
	APIVersion:  "1.0",
	PathBuilder: subnetPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// SubnetOperations defines operation behavior for subnets.
// Native ID format: project/networkId/subnetId (nested resource).
var SubnetOperations = base.OperationConfig{
	Synchronous: true, // Subnet creation is synchronous
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		// Extract the subnet ID from response
		if id, ok := response["id"].(string); ok {
			if ctx.Project != "" && ctx.ParentResource != "" {
				return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.ParentResource, id)
			}
			if ctx.Project != "" {
				return fmt.Sprintf("%s/%s", ctx.Project, id)
			}
			return id
		}
		return ""
	},
}

// SubnetNativeID defines native ID format for subnets: "project/networkId/subnetId"
var SubnetNativeID = base.NativeIDConfig{
	Format: base.ProjectNestedFormat,
}

// PrivateSubnetProvisioner wraps BaseResource and adds custom Read via List+filter.
type PrivateSubnetProvisioner struct {
	base *base.BaseResource
}

// Create delegates to BaseResource and re-injects network_id into the
// returned ResourceProperties. The OVH subnet POST response only echoes
// {id, cidr, gatewayIp, ipPools[]} — it omits the parent networkId which
// lives in the URL path. Dependents (e.g. instance.networks[].networkId)
// resolve `subnet.res.network_id` from these properties; without this
// re-injection, the resolvable returns nothing and the dependent send
// fails with HTTP 400 InvalidInput.
func (p *PrivateSubnetProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	result, err := p.base.Create(ctx, request)
	if err != nil || result == nil || result.ProgressResult == nil {
		return result, err
	}
	if len(result.ProgressResult.ResourceProperties) == 0 {
		return result, nil
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return result, nil
	}
	networkID, _ := props["network_id"].(string)
	if networkID == "" {
		return result, nil
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(result.ProgressResult.ResourceProperties, &resp); err != nil {
		return result, nil
	}
	if _, ok := resp["network_id"]; ok {
		return result, nil
	}
	resp["network_id"] = networkID
	merged, err := json.Marshal(resp)
	if err != nil {
		return result, nil
	}
	result.ProgressResult.ResourceProperties = merged
	return result, nil
}

// Read implements Read by listing subnets and filtering by ID.
// Uses GET /cloud/project/{serviceName}/network/private/{networkId}/subnet
func (p *PrivateSubnetProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Parse native ID: project/networkId/subnetId
	parts := strings.Split(request.NativeID, "/")
	if len(parts) != 3 {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	project := parts[0]
	networkID := parts[1]
	subnetID := parts[2]

	// Build list URL: /cloud/project/{serviceName}/network/private/{networkId}/subnet
	listURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", project, networkID)

	// Call the OVH API to list subnets
	response, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   listURL,
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

	// Response is an array of subnets (stored in BodyArray)
	subnets := response.BodyArray
	if subnets == nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeServiceInternalError,
		}, nil
	}

	// Find the subnet with matching ID
	for _, s := range subnets {
		subnet, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := subnet["id"].(string); ok && id == subnetID {
			// Transform API response to match PKL schema.
			// API returns: {id, cidr, gatewayIp, ipPools: [{dhcp, end, network, region, start}]}
			// Schema expects: {id, network_id, region, network, dhcp, noGateway, start, end}
			result := transformSubnetResponse(subnet, networkID)

			propsJSON, err := json.Marshal(result)
			if err != nil {
				return &resource.ReadResult{
					ErrorCode: resource.OperationErrorCodeServiceInternalError,
				}, nil
			}
			return &resource.ReadResult{
				Properties: string(propsJSON),
			}, nil
		}
	}

	// Subnet not found
	return &resource.ReadResult{
		ErrorCode: resource.OperationErrorCodeNotFound,
	}, nil
}

// Update is not supported for PrivateSubnet
func (p *PrivateSubnetProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return p.base.Update(ctx, request)
}

// Delete delegates to BaseResource
func (p *PrivateSubnetProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.base.Delete(ctx, request)
}

// Status delegates to BaseResource
func (p *PrivateSubnetProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return p.base.Status(ctx, request)
}

// List enumerates all private subnets across all private networks.
// Discovery calls List with empty AdditionalProperties, so we must iterate all networks.
func (p *PrivateSubnetProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProject(request.TargetConfig)
	if project == "" {
		return nil, fmt.Errorf("project/serviceName is required but not found in target config")
	}

	// Step 1: List all private networks
	networksURL := fmt.Sprintf("/cloud/project/%s/network/private", project)
	networksResp, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   networksURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list private networks: %w", err)
	}

	// Step 2: For each network, list its subnets
	var nativeIDs []string
	for _, item := range networksResp.BodyArray {
		network, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		networkID, ok := network["id"].(string)
		if !ok {
			continue
		}

		subnetsURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", project, networkID)
		subnetsResp, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{
			Method: "GET",
			Path:   subnetsURL,
		})
		if err != nil {
			continue // Skip networks we can't read subnets from
		}

		for _, subItem := range subnetsResp.BodyArray {
			subnet, ok := subItem.(map[string]interface{})
			if !ok {
				continue
			}
			subnetID, ok := subnet["id"].(string)
			if !ok {
				continue
			}
			// Native ID format: project/networkId/subnetId
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, networkID, subnetID))
		}
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}

// privateSubnetDefinition holds the resource definition for creating BaseResource instances
var privateSubnetDefinition *base.ResourceDefinition

func init() {
	// Define the resource configuration
	privateSubnetDefinition = &base.ResourceDefinition{
		ResourceType: PrivateSubnetResourceType,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "subnet", // Base type for path construction
			Scope:        &base.ScopeConfig{Type: base.ScopeProject},
			ParentResource: &base.ParentResourceConfig{
				RequiresParent: true,
				ParentType:     "network/private", // Used in URL path
				PropertyName:   "network_id",
			},
			SupportsUpdate: false, // OVH subnets are not updatable
		},
		// Strip network_id from request body (used in URL path)
		RequestTransformer: subnetPrivateTransformer,
		Operations: []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
	}

	// Register with global registry using custom factory (includes OperationList for discovery)
	registry.Register(
		PrivateSubnetResourceType,
		privateSubnetDefinition.Operations,
		func(client *ovhtransport.Client) prov.Provisioner {
			// Create BaseResource directly for Create/Delete operations
			baseResource := &base.BaseResource{
				APIConfig:          SubnetAPI,
				OperationConfig:    SubnetOperations,
				ResourceConfig:     privateSubnetDefinition.ResourceConfig,
				NativeIDConfig:     SubnetNativeID,
				RequestTransformer: subnetPrivateTransformer,
				Client:             client,
			}
			return &PrivateSubnetProvisioner{
				base: baseResource,
			}
		},
	)
}

// transformSubnetResponse maps the OVH API GET response to PKL schema field names.
// API returns: {id, cidr, gatewayIp, ipPools: [{dhcp, end, network, region, start}]}
// Schema expects: {id, network_id, region, network, dhcp, noGateway, start, end}
func transformSubnetResponse(subnet map[string]interface{}, networkID string) map[string]interface{} {
	result := map[string]interface{}{
		"id":         subnet["id"],
		"network_id": networkID,
	}

	// Derive noGateway from gatewayIp (empty means no gateway)
	gatewayIP, _ := subnet["gatewayIp"].(string)
	result["noGateway"] = gatewayIP == ""

	// Flatten ipPools[0] → top-level fields (dhcp, end, network, region, start)
	if ipPools, ok := subnet["ipPools"].([]interface{}); ok && len(ipPools) > 0 {
		if pool, ok := ipPools[0].(map[string]interface{}); ok {
			for _, field := range []string{"dhcp", "end", "network", "region", "start"} {
				if val, ok := pool[field]; ok {
					result[field] = val
				}
			}
		}
	}

	return result
}

// extractProject extracts the project/serviceName from target config JSON.
func extractProject(targetConfig json.RawMessage) string {
	return extractTargetField(targetConfig, []string{"ProjectId", "projectId", "ServiceName", "serviceName"})
}

func extractTargetField(targetConfig json.RawMessage, fields []string) string {
	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}
	for _, field := range fields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}
