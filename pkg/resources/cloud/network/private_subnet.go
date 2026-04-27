// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// Create posts the subnet, then lists subnets to recover the API-generated ID.
func (p *PrivateSubnetProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return privateSubnetCreateFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig)
	if project == "" {
		return privateSubnetCreateFailure(resource.OperationErrorCodeInvalidRequest, "project/serviceName is required but not found in target config"), nil
	}

	networkID, ok := props["network_id"].(string)
	if !ok || networkID == "" {
		return privateSubnetCreateFailure(resource.OperationErrorCodeInvalidRequest, "network_id is required"), nil
	}

	body, err := subnetPrivateTransformer.Transform(props, base.TransformContext{})
	if err != nil {
		return privateSubnetCreateFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to transform request: %v", err)), nil
	}

	createURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", project, networkID)
	if _, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{Method: "POST", Path: createURL, Body: body}); err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return privateSubnetCreateFailure(ovhtransport.ToResourceErrorCode(transportErr.Code), err.Error()), nil
		}
		return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	listURL := fmt.Sprintf("/cloud/project/%s/network/private/%s/subnet", project, networkID)
	deadline := time.Now().Add(5 * time.Minute)
	for {
		subnet, err := p.findCreatedSubnet(ctx, listURL, props)
		if err != nil {
			return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
		}
		if subnet != nil {
			id, _ := subnet["id"].(string)
			if id == "" {
				return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, "created subnet response did not include id"), nil
			}

			result := transformSubnetResponse(subnet, networkID)
			propsJSON, err := json.Marshal(result)
			if err != nil {
				return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, fmt.Sprintf("failed to marshal properties: %v", err)), nil
			}

			return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCreate,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           fmt.Sprintf("%s/%s/%s", project, networkID, id),
				ResourceProperties: propsJSON,
			}}, nil
		}

		if time.Now().After(deadline) {
			return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, "created subnet was not visible before timeout"), nil
		}

		select {
		case <-ctx.Done():
			return privateSubnetCreateFailure(resource.OperationErrorCodeServiceInternalError, ctx.Err().Error()), nil
		case <-time.After(5 * time.Second):
		}
	}
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

func (p *PrivateSubnetProvisioner) findCreatedSubnet(ctx context.Context, listURL string, expected map[string]interface{}) (map[string]interface{}, error) {
	response, err := p.base.Client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: listURL})
	if err != nil {
		return nil, err
	}

	for _, s := range response.BodyArray {
		subnet, ok := s.(map[string]interface{})
		if !ok || !subnetMatchesRequest(subnet, expected) {
			continue
		}
		return subnet, nil
	}

	return nil, nil
}

func subnetMatchesRequest(subnet map[string]interface{}, expected map[string]interface{}) bool {
	ipPools, ok := subnet["ipPools"].([]interface{})
	if ok {
		for _, item := range ipPools {
			pool, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if poolMatchesRequest(pool, expected) {
				return true
			}
		}
	}

	if expectedNetwork, _ := expected["network"].(string); expectedNetwork != "" {
		if cidr, _ := subnet["cidr"].(string); cidr == expectedNetwork {
			return true
		}
	}

	return false
}

func poolMatchesRequest(pool map[string]interface{}, expected map[string]interface{}) bool {
	for _, field := range []string{"network", "region", "start", "end"} {
		if expectedValue, _ := expected[field].(string); expectedValue != "" {
			if actualValue, _ := pool[field].(string); actualValue != expectedValue {
				return false
			}
		}
	}

	if expectedDHCP, ok := expected["dhcp"].(bool); ok {
		if actualDHCP, ok := pool["dhcp"].(bool); !ok || actualDHCP != expectedDHCP {
			return false
		}
	}

	return true
}

func privateSubnetCreateFailure(code resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationCreate,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       code,
		StatusMessage:   message,
	}}
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
