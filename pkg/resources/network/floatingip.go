// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
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
	ResourceTypeFloatingIP = "OVH::Network::FloatingIP"
)

// FloatingIP schema and descriptor
var (
	FloatingIPDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeFloatingIP,
		Discoverable: true,
	}

	FloatingIPSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"floating_network_id", "floating_ip_address", "port_id", "fixed_ip_address", "description"},
		Hints: map[string]model.FieldHint{
			"floating_network_id": {
				Required:   true,
				CreateOnly: true,
			},
			"floating_ip_address": {
				Required:   false,
				CreateOnly: true,
			},
			"port_id": {
				Required: false,
			},
			"fixed_ip_address": {
				Required: false,
			},
			"description": {
				Required: false,
			},
		},
	}
)

// FloatingIP provisioner
type FloatingIP struct {
	Client *client.Client
	Config *config.Config
}

// floatingIPToProperties converts an OpenStack floating IP to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func floatingIPToProperties(fip *floatingips.FloatingIP) map[string]interface{} {
	props := map[string]interface{}{
		"id":                  fip.ID,
		"floating_network_id": fip.FloatingNetworkID,
		"floating_ip_address": fip.FloatingIP,
		"status":              fip.Status,
	}

	// Add optional fields if present
	if fip.Description != "" {
		props["description"] = fip.Description
	}

	if fip.PortID != "" {
		props["port_id"] = fip.PortID
	}

	if fip.FixedIP != "" {
		props["fixed_ip_address"] = fip.FixedIP
	}

	if fip.RouterID != "" {
		props["router_id"] = fip.RouterID
	}

	return props
}

// Register the FloatingIP resource type
func init() {
	registry.Register(
		ResourceTypeFloatingIP,
		FloatingIPDescriptor,
		FloatingIPSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &FloatingIP{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new floating IP
func (f *FloatingIP) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeFloatingIP, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Build create options - FloatingNetworkID is required
	floatingNetworkID, ok := props["floating_network_id"].(string)
	if !ok || floatingNetworkID == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeFloatingIP, resource.OperationErrorCodeInvalidRequest, "", "floating_network_id is required"),
		}, nil
	}

	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: floatingNetworkID,
	}

	// Add optional floating_ip_address (specific IP to allocate)
	if floatingIP, ok := props["floating_ip_address"].(string); ok && floatingIP != "" {
		createOpts.FloatingIP = floatingIP
	}

	// Add optional port_id (port to associate with)
	if portID, ok := props["port_id"].(string); ok && portID != "" {
		createOpts.PortID = portID
	}

	// Add optional fixed_ip_address
	if fixedIP, ok := props["fixed_ip_address"].(string); ok && fixedIP != "" {
		createOpts.FixedIP = fixedIP
	}

	// Add optional description
	if description, ok := props["description"].(string); ok {
		createOpts.Description = description
	}

	// Create the floating IP via OpenStack
	fip, err := floatingips.Create(ctx, f.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create floating IP: %v", err),
			},
		}, nil
	}

	// Convert floating IP to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(floatingIPToProperties(fip))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        fip.ID,
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
			NativeID:           fip.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a floating IP
func (f *FloatingIP) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the floating IP ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the floating IP from OpenStack
	fip, err := floatingips.Get(ctx, f.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read floating IP: %w", err)
	}

	// Convert floating IP to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(floatingIPToProperties(fip))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing floating IP
func (f *FloatingIP) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the floating IP ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeFloatingIP, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeFloatingIP, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := floatingips.UpdateOpts{}

	// Update mutable fields
	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	// Handle port_id - can be set to empty string to disassociate
	if portID, ok := props["port_id"].(string); ok {
		updateOpts.PortID = &portID
	}

	// Handle fixed_ip_address
	if fixedIP, ok := props["fixed_ip_address"].(string); ok {
		updateOpts.FixedIP = fixedIP
	}

	// Update the floating IP via OpenStack
	fip, err := floatingips.Update(ctx, f.Client.NetworkClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update floating IP: %v", err),
			},
		}, nil
	}

	// Convert floating IP to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(floatingIPToProperties(fip))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        fip.ID,
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
			NativeID:           fip.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a floating IP
func (f *FloatingIP) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the floating IP ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeFloatingIP, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the floating IP from OpenStack
	err := floatingips.Delete(ctx, f.Client.NetworkClient, id).ExtractErr()
	if err != nil {
		// Check if the error is NotFound - if so, consider it a success (idempotent delete)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
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
				StatusMessage:   fmt.Sprintf("failed to delete floating IP: %v", err),
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

// Status checks the status of a long-running operation (floating IPs are synchronous, so not used)
func (f *FloatingIP) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers floating IPs
func (f *FloatingIP) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all floating IPs using pagination
	allPages, err := floatingips.List(f.Client.NetworkClient, floatingips.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list floating IPs: %w", err)
	}

	// Extract floating IPs from pages
	fipList, err := floatingips.ExtractFloatingIPs(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract floating IPs: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(fipList))
	for _, fip := range fipList {
		nativeIDs = append(nativeIDs, fip.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
