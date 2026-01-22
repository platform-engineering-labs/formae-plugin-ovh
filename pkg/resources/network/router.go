// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources"
)

const (
	ResourceTypeRouter = "OVH::Network::Router"
)

// Router schema and descriptor
var (
	RouterDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeRouter,
		Discoverable: true,
	}

	RouterSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "admin_state_up", "external_gateway_info", "routes", "tags"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: false,
			},
			"description": {
				Required: false,
			},
			"admin_state_up": {
				Required: false,
			},
			"external_gateway_info": {
				Required: false,
			},
			"routes": {
				Required: false,
			},
			"tags": {
				Required: false,
			},
		},
	}
)

// Router provisioner
type Router struct {
	Client *client.Client
	Config *config.Config
}

// routerToProperties converts an OpenStack router to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func routerToProperties(router *routers.Router) map[string]interface{} {
	props := map[string]interface{}{
		"id":             router.ID,
		"name":           router.Name,
		"description":    router.Description,
		"admin_state_up": router.AdminStateUp,
	}

	// Add external gateway info if present
	// Only return network_id - external_fixed_ips is computed by OpenStack
	// TODO: Investigate enable_snat handling - OVH sets it automatically and policy
	// prevents users from explicitly setting it. For now we omit it from Read output
	// to avoid drift detection issues. May need revisiting for other OpenStack providers.
	if router.GatewayInfo.NetworkID != "" {
		gatewayInfo := map[string]interface{}{
			"network_id": router.GatewayInfo.NetworkID,
		}
		props["external_gateway_info"] = gatewayInfo
	}

	// Add routes if present
	if len(router.Routes) > 0 {
		routes := make([]map[string]interface{}, 0, len(router.Routes))
		for _, route := range router.Routes {
			routes = append(routes, map[string]interface{}{
				"destination": route.DestinationCIDR,
				"nexthop":     route.NextHop,
			})
		}
		props["routes"] = routes
	}

	// Add tags if present
	if len(router.Tags) > 0 {
		props["tags"] = router.Tags
	}

	return props
}

// Register the Router resource type
func init() {
	registry.Register(
		ResourceTypeRouter,
		RouterDescriptor,
		RouterSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Router{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new router
func (r *Router) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeRouter, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Build create options
	createOpts := routers.CreateOpts{}

	// Add optional name
	if name, ok := props["name"].(string); ok && name != "" {
		createOpts.Name = name
	}

	// Add optional description
	if description, ok := props["description"].(string); ok && description != "" {
		createOpts.Description = description
	}

	// Add optional admin_state_up (defaults to true if not specified)
	if adminStateUp, ok := props["admin_state_up"].(bool); ok {
		createOpts.AdminStateUp = &adminStateUp
	}

	// Add optional external_gateway_info
	if gatewayInfo, ok := props["external_gateway_info"].(map[string]interface{}); ok {
		gwi := &routers.GatewayInfo{}

		if networkID, ok := gatewayInfo["network_id"].(string); ok && networkID != "" {
			gwi.NetworkID = networkID
		}

		if enableSNAT, ok := gatewayInfo["enable_snat"].(bool); ok {
			gwi.EnableSNAT = &enableSNAT
		}

		createOpts.GatewayInfo = gwi
	}

	// Create the router via OpenStack
	router, err := routers.Create(ctx, r.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create router: %v", err),
			},
		}, nil
	}

	// Set tags if provided (must be done after creation via attributestags API)
	tags := resources.ParseTags(props["tags"])
	if len(tags) > 0 {
		_, err = attributestags.ReplaceAll(ctx, r.Client.NetworkClient, "routers", router.ID, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - router was created successfully
			fmt.Printf("warning: failed to set tags on router %s: %v\n", router.ID, err)
		} else {
			router.Tags = tags
		}
	}

	// Convert router to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(routerToProperties(router))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        router.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           router.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a router
func (r *Router) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the router ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the router from OpenStack
	router, err := routers.Get(ctx, r.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read router: %w", err)
	}

	// Convert router to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(routerToProperties(router))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing router
func (r *Router) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the router ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeRouter, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeRouter, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := routers.UpdateOpts{}

	// Update mutable fields
	if name, ok := props["name"].(string); ok {
		updateOpts.Name = name
	}

	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	if adminStateUp, ok := props["admin_state_up"].(bool); ok {
		updateOpts.AdminStateUp = &adminStateUp
	}

	// Update external gateway info if present
	if gatewayInfo, ok := props["external_gateway_info"].(map[string]interface{}); ok {
		gwi := &routers.GatewayInfo{}

		if networkID, ok := gatewayInfo["network_id"].(string); ok {
			gwi.NetworkID = networkID
		}

		if enableSNAT, ok := gatewayInfo["enable_snat"].(bool); ok {
			gwi.EnableSNAT = &enableSNAT
		}

		updateOpts.GatewayInfo = gwi
	}

	// Update routes if present
	if routesRaw, ok := props["routes"].([]interface{}); ok {
		routes := make([]routers.Route, 0, len(routesRaw))
		for _, routeRaw := range routesRaw {
			if routeMap, ok := routeRaw.(map[string]interface{}); ok {
				route := routers.Route{}
				if destination, ok := routeMap["destination"].(string); ok {
					route.DestinationCIDR = destination
				}
				if nexthop, ok := routeMap["nexthop"].(string); ok {
					route.NextHop = nexthop
				}
				routes = append(routes, route)
			}
		}
		updateOpts.Routes = &routes
	}

	// Update the router via OpenStack
	router, err := routers.Update(ctx, r.Client.NetworkClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update router: %v", err),
			},
		}, nil
	}

	// Update tags if provided (via attributestags API)
	if _, hasTags := props["tags"]; hasTags {
		tags := resources.ParseTags(props["tags"])
		if tags == nil {
			tags = []string{} // Empty slice to clear all tags
		}
		updatedTags, err := attributestags.ReplaceAll(ctx, r.Client.NetworkClient, "routers", id, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - router was updated successfully
			fmt.Printf("warning: failed to update tags on router %s: %v\n", id, err)
		} else {
			router.Tags = updatedTags
		}
	}

	// Convert router to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(routerToProperties(router))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        router.ID,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("failed to marshal properties: %v", err),
			},
		}, nil
	}

	// Return success with properties
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           router.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a router
func (r *Router) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the router ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeRouter, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the router from OpenStack
	err := routers.Delete(ctx, r.Client.NetworkClient, id).ExtractErr()
	if err != nil {
		// Check if the error is NotFound - if so, consider it a success (idempotent delete)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			// Resource already deleted - this is a success
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        id,
				},
			}, nil
		}

		// Other errors are actual failures
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
				StatusMessage:   fmt.Sprintf("failed to delete router: %v", err),
			},
		}, nil
	}

	// Return success
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        id,
		},
	}, nil
}

// Status checks the status of a long-running operation (routers are synchronous, so not used)
func (r *Router) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers routers
func (r *Router) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all routers using pagination
	allPages, err := routers.List(r.Client.NetworkClient, routers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list routers: %w", err)
	}

	// Extract routers from pages
	routerList, err := routers.ExtractRouters(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract routers: %w", err)
	}

	// Convert each router to a resource
	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(routerList))
	for _, router := range routerList {
		nativeIDs = append(nativeIDs, router.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
