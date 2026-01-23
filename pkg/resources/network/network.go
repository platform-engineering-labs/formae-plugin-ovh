// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/mtu"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources"
)

// networkWithMTU embeds networks.Network and mtu.NetworkMTUExt to properly
// extract the MTU field from OpenStack API responses.
type networkWithMTU struct {
	networks.Network
	mtu.NetworkMTUExt
}

const (
	ResourceTypeNetwork = "OVH::Network::Network"
)

// Network schema and descriptor
var (
	NetworkDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeNetwork,
		Discoverable: true,
	}

	NetworkSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "admin_state_up", "shared", "mtu", "tags"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: false,
			},
			"description": {
				Required:   false,
				CreateOnly: true, // OVH doesn't support updating network descriptions
			},
			"admin_state_up": {
				Required: false,
			},
			"shared": {
				Required:   false,
				CreateOnly: true,
			},
			"mtu": {
				Required:   false,
				CreateOnly: true,
			},
			"tags": {
				Required: false,
			},
		},
	}
)

// Network provisioner
type Network struct {
	Client *client.Client
	Config *config.Config
}

// networkToProperties converts an OpenStack network to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func networkToProperties(net *networkWithMTU) map[string]interface{} {
	props := map[string]interface{}{
		"id":             net.ID,
		"name":           net.Name,
		"description":    net.Description,
		"admin_state_up": net.AdminStateUp,
		"shared":         net.Shared,
	}

	// Add MTU if non-zero
	if net.MTU > 0 {
		props["mtu"] = net.MTU
	}

	// Add tags if present
	if len(net.Tags) > 0 {
		props["tags"] = net.Tags
	}

	return props
}

// Register the Network resource type
func init() {
	registry.Register(
		ResourceTypeNetwork,
		NetworkDescriptor,
		NetworkSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Network{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new network
func (n *Network) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeNetwork, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Build create options
	createOpts := networks.CreateOpts{}

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

	// Add optional shared
	if shared, ok := props["shared"].(bool); ok {
		createOpts.Shared = &shared
	}

	// Wrap with MTU extension if MTU is specified
	var finalCreateOpts networks.CreateOptsBuilder = createOpts
	if mtuVal, ok := props["mtu"].(float64); ok && mtuVal > 0 {
		finalCreateOpts = mtu.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			MTU:               int(mtuVal),
		}
	}

	// Create the network via OpenStack
	net, err := networks.Create(ctx, n.Client.NetworkClient, finalCreateOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create network: %v", err),
			},
		}, nil
	}

	// Set tags if provided (must be done after creation via attributestags API)
	tags := resources.ParseTags(props["tags"])
	if len(tags) > 0 {
		_, err = attributestags.ReplaceAll(ctx, n.Client.NetworkClient, "networks", net.ID, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - network was created successfully
			// Tags can be set on subsequent update
			fmt.Printf("warning: failed to set tags on network %s: %v\n", net.ID, err)
		} else {
			net.Tags = tags
		}
	}

	// Build networkWithMTU from result, including requested MTU value
	netWithMTU := &networkWithMTU{Network: *net}
	if mtuVal, ok := props["mtu"].(float64); ok && mtuVal > 0 {
		netWithMTU.MTU = int(mtuVal)
	}

	// Convert network to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(networkToProperties(netWithMTU))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        net.ID,
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
			NativeID:           net.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a network
func (n *Network) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the network ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the network from OpenStack using ExtractInto to get MTU extension field
	var net networkWithMTU
	err := networks.Get(ctx, n.Client.NetworkClient, id).ExtractInto(&net)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read network: %w", err)
	}

	// Explicitly fetch tags - OpenStack often doesn't include them in the standard GET response
	tags, err := attributestags.List(ctx, n.Client.NetworkClient, "networks", id).Extract()
	if err != nil {
		// Log warning but continue - tags are optional
		fmt.Printf("warning: failed to fetch tags for network %s: %v\n", id, err)
	} else {
		net.Tags = tags
	}

	// Convert network to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(networkToProperties(&net))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing network
func (n *Network) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the network ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeNetwork, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeNetwork, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := networks.UpdateOpts{}

	// Update mutable fields
	if name, ok := props["name"].(string); ok {
		updateOpts.Name = &name
	}

	if description, ok := props["description"].(string); ok {
		updateOpts.Description = &description
	}

	if adminStateUp, ok := props["admin_state_up"].(bool); ok {
		updateOpts.AdminStateUp = &adminStateUp
	}

	// Update the network via OpenStack using ExtractInto to get MTU extension field
	var net networkWithMTU
	err = networks.Update(ctx, n.Client.NetworkClient, id, updateOpts).ExtractInto(&net)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update network: %v", err),
			},
		}, nil
	}

	// Update tags if provided (via attributestags API)
	if _, hasTags := props["tags"]; hasTags {
		tags := resources.ParseTags(props["tags"])
		if tags == nil {
			tags = []string{} // Empty slice to clear all tags
		}
		updatedTags, err := attributestags.ReplaceAll(ctx, n.Client.NetworkClient, "networks", id, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - network was updated successfully
			fmt.Printf("warning: failed to update tags on network %s: %v\n", id, err)
		} else {
			net.Tags = updatedTags
		}
	}

	// Convert network to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(networkToProperties(&net))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        net.ID,
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
			NativeID:           net.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a network
func (n *Network) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the network ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeNetwork, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the network from OpenStack
	err := networks.Delete(ctx, n.Client.NetworkClient, id).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete network: %v", err),
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

// Status checks the status of a long-running operation (networks are synchronous, so not used)
func (n *Network) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers networks
func (n *Network) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all networks using pagination
	allPages, err := networks.List(n.Client.NetworkClient, networks.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list networks: %w", err)
	}

	// Extract networks from pages using ExtractNetworksInto to get MTU extension field
	var nets []networkWithMTU
	err = networks.ExtractNetworksInto(allPages, &nets)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract networks: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(nets))
	for _, net := range nets {
		nativeIDs = append(nativeIDs, net.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
