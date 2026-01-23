// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/attachinterfaces"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/secgroups"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
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

const (
	ResourceTypeInstance = "OVH::Compute::Instance"
)

// Instance schema and descriptor
var (
	InstanceDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeInstance,
		Discoverable: true,
	}

	InstanceSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields: []string{
			"name", "flavor_id", "image_id", "networks",
			"security_groups", "key_name", "user_data",
			"availability_zone", "metadata", "config_drive",
		},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: true,
			},
			"flavor_id": {
				Required:   true,
				CreateOnly: true,
			},
			"image_id": {
				Required:   true,
				CreateOnly: true,
			},
			"networks": {
				Required:   false,
				CreateOnly: true,
			},
			"security_groups": {
				Required: false,
			},
			"key_name": {
				Required:   false,
				CreateOnly: true,
			},
			"user_data": {
				Required:   false,
				CreateOnly: true,
			},
			"availability_zone": {
				Required:   false,
				CreateOnly: true,
			},
			"metadata": {
				Required: false,
			},
			"config_drive": {
				Required:   false,
				CreateOnly: true,
			},
		},
	}
)

// Instance provisioner
type Instance struct {
	Client *client.Client
	Config *config.Config
}

// instanceToProperties converts an OpenStack server to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
// The networkList parameter allows passing pre-fetched network information (from attachinterfaces API).
func instanceToProperties(server *servers.Server, networkList []interface{}) map[string]interface{} {
	props := map[string]interface{}{
		"id":   server.ID,
		"name": server.Name,
	}

	// Add flavor reference
	if len(server.Flavor) > 0 {
		if flavorID, ok := server.Flavor["id"].(string); ok {
			props["flavor_id"] = flavorID
		}
	}

	// Add image reference
	if len(server.Image) > 0 {
		if imageID, ok := server.Image["id"].(string); ok {
			props["image_id"] = imageID
		}
	}

	// Add optional fields if present
	if server.KeyName != "" {
		props["key_name"] = server.KeyName
	}

	if len(server.Metadata) > 0 {
		props["metadata"] = server.Metadata
	}

	if len(server.SecurityGroups) > 0 {
		sgNames := make([]string, 0, len(server.SecurityGroups))
		for _, sg := range server.SecurityGroups {
			if sgName, ok := sg["name"].(string); ok {
				sgNames = append(sgNames, sgName)
			}
		}
		if len(sgNames) > 0 {
			props["security_groups"] = sgNames
		}
	}

	// Add network attachments if provided
	if len(networkList) > 0 {
		props["networks"] = networkList
	}

	// Add availability zone (directly from server struct)
	if server.AvailabilityZone != "" {
		props["availability_zone"] = server.AvailabilityZone
	}

	// Note: config_drive is CreateOnly and not returned from the OpenStack API

	return props
}

// Register the Instance resource type
func init() {
	registry.Register(
		ResourceTypeInstance,
		InstanceDescriptor,
		InstanceSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Instance{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new instance (async operation)
func (i *Instance) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeInstance, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Extract required fields
	name, ok := props["name"].(string)
	if !ok || name == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "name is required",
			},
		}, nil
	}

	flavorID, ok := props["flavor_id"].(string)
	if !ok || flavorID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "flavor_id is required",
			},
		}, nil
	}

	imageID, ok := props["image_id"].(string)
	if !ok || imageID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "image_id is required",
			},
		}, nil
	}

	// Build create options
	createOpts := servers.CreateOpts{
		Name:      name,
		FlavorRef: flavorID,
		ImageRef:  imageID,
	}

	// Add optional networks
	if networksRaw, ok := props["networks"]; ok && networksRaw != nil {
		networks, ok := networksRaw.([]interface{})
		if ok {
			serverNetworks := make([]servers.Network, 0, len(networks))
			for _, netRaw := range networks {
				netMap, ok := netRaw.(map[string]interface{})
				if ok {
					network := servers.Network{}
					if uuid, ok := netMap["uuid"].(string); ok {
						network.UUID = uuid
					}
					if port, ok := netMap["port"].(string); ok {
						network.Port = port
					}
					if fixedIP, ok := netMap["fixed_ip"].(string); ok {
						network.FixedIP = fixedIP
					}
					serverNetworks = append(serverNetworks, network)
				}
			}
			createOpts.Networks = serverNetworks
		}
	}

	// Add optional security groups
	if sgRaw, ok := props["security_groups"]; ok && sgRaw != nil {
		securityGroups, ok := sgRaw.([]interface{})
		if ok {
			sgNames := make([]string, 0, len(securityGroups))
			for _, sg := range securityGroups {
				if sgName, ok := sg.(string); ok {
					sgNames = append(sgNames, sgName)
				}
			}
			createOpts.SecurityGroups = sgNames
		}
	}

	// Add optional user data
	if userData, ok := props["user_data"].(string); ok && userData != "" {
		// User data should be base64 encoded but gophercloud handles this
		createOpts.UserData = []byte(userData)
	}

	// Add optional availability zone
	if az, ok := props["availability_zone"].(string); ok && az != "" {
		createOpts.AvailabilityZone = az
	}

	// Add optional metadata
	if metadataRaw, ok := props["metadata"]; ok && metadataRaw != nil {
		metadata, ok := metadataRaw.(map[string]interface{})
		if ok {
			metadataStr := make(map[string]string, len(metadata))
			for k, v := range metadata {
				if vStr, ok := v.(string); ok {
					metadataStr[k] = vStr
				}
			}
			createOpts.Metadata = metadataStr
		}
	}

	// Add optional config drive
	if configDrive, ok := props["config_drive"].(bool); ok {
		createOpts.ConfigDrive = &configDrive
	}

	// Wrap createOpts with keypairs extension if key_name is provided
	var serverCreateOpts servers.CreateOptsBuilder = createOpts
	if keyName, ok := props["key_name"].(string); ok && keyName != "" {
		serverCreateOpts = keypairs.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			KeyName:           keyName,
		}
	}

	// Create the server via OpenStack
	server, err := servers.Create(ctx, i.Client.ComputeClient, serverCreateOpts, nil).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create instance: %v", err),
			},
		}, nil
	}

	// Return InProgress - instances are created asynchronously
	// The caller will poll Status() to check when the instance is ready
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			RequestID:       server.ID, // Use server ID for polling
			NativeID:        server.ID,
		},
	}, nil
}

// Read retrieves the current state of an instance
func (i *Instance) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the instance ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil // Don't return Go error for expected errors
	}

	// Get the server from OpenStack
	server, err := servers.Get(ctx, i.Client.ComputeClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, nil // Don't return Go error for expected errors like NotFound
	}

	// Fetch network attachments using the attachinterfaces API
	// This is more reliable than server.Addresses which may take time to populate
	var networkList []interface{}
	ifacePages, ifaceErr := attachinterfaces.List(i.Client.ComputeClient, id).AllPages(ctx)
	if ifaceErr == nil {
		ifaces, extractErr := attachinterfaces.ExtractInterfaces(ifacePages)
		if extractErr == nil && len(ifaces) > 0 {
			networkList = make([]interface{}, 0, len(ifaces))
			for _, iface := range ifaces {
				networkList = append(networkList, map[string]interface{}{
					"uuid": iface.NetID,
				})
			}
		}
	}
	// Fallback: if attachinterfaces didn't work, try using server.Addresses
	// (requires looking up network IDs by name since Addresses uses network names as keys)
	if len(networkList) == 0 && len(server.Addresses) > 0 {
		// Get all networks to build name -> ID mapping
		networkPages, netErr := networks.List(i.Client.NetworkClient, networks.ListOpts{}).AllPages(ctx)
		if netErr == nil {
			allNetworks, extractErr := networks.ExtractNetworks(networkPages)
			if extractErr == nil {
				networkNameToID := make(map[string]string)
				for _, n := range allNetworks {
					networkNameToID[n.Name] = n.ID
				}
				networkList = make([]interface{}, 0, len(server.Addresses))
				for networkName := range server.Addresses {
					networkID := networkName
					if mappedID, ok := networkNameToID[networkName]; ok {
						networkID = mappedID
					}
					networkList = append(networkList, map[string]interface{}{
						"uuid": networkID,
					})
				}
			}
		}
	}

	// Convert server to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(instanceToProperties(server, networkList))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, nil // Don't return Go error for expected errors
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing instance (only mutable fields: name, metadata, security_groups)
func (i *Instance) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the instance ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeInstance, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypeInstance, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options - only name is mutable via standard update
	updateOpts := servers.UpdateOpts{}

	if name, ok := props["name"].(string); ok {
		updateOpts.Name = name
	}

	// Update the server via OpenStack
	server, err := servers.Update(ctx, i.Client.ComputeClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update instance: %v", err),
			},
		}, nil
	}

	// Handle metadata update separately if provided
	if metadataRaw, ok := props["metadata"]; ok && metadataRaw != nil {
		metadata, ok := metadataRaw.(map[string]interface{})
		if ok {
			metadataStr := make(map[string]string, len(metadata))
			for k, v := range metadata {
				if vStr, ok := v.(string); ok {
					metadataStr[k] = vStr
				}
			}
			// Reset all metadata (delete all, then set new)
			_, err = servers.ResetMetadata(ctx, i.Client.ComputeClient, id, servers.MetadataOpts(metadataStr)).Extract()
			if err != nil {
				return &resource.UpdateResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationUpdate,
						OperationStatus: resource.OperationStatusFailure,
						ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
						StatusMessage:   fmt.Sprintf("failed to update instance metadata: %v", err),
					},
				}, nil
			}
		}
	}

	// Handle security group updates via diff-based add/remove
	if sgRaw, ok := props["security_groups"].([]interface{}); ok {
		// Build set of desired security group names
		desiredSGs := make(map[string]struct{}, len(sgRaw))
		for _, sg := range sgRaw {
			if sgName, ok := sg.(string); ok {
				desiredSGs[sgName] = struct{}{}
			}
		}

		// Build set of current security group names from the server
		currentSGs := make(map[string]struct{}, len(server.SecurityGroups))
		for _, sg := range server.SecurityGroups {
			if sgName, ok := sg["name"].(string); ok {
				currentSGs[sgName] = struct{}{}
			}
		}

		// Compute groups to remove (current - desired)
		for sgName := range currentSGs {
			if _, inDesired := desiredSGs[sgName]; !inDesired {
				err := secgroups.RemoveServer(ctx, i.Client.ComputeClient, id, sgName).ExtractErr()
				if err != nil {
					return &resource.UpdateResult{
						ProgressResult: &resource.ProgressResult{
							Operation:       resource.OperationUpdate,
							OperationStatus: resource.OperationStatusFailure,
							ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
							StatusMessage:   fmt.Sprintf("failed to remove security group %s: %v", sgName, err),
						},
					}, nil
				}
			}
		}

		// Compute groups to add (desired - current)
		for sgName := range desiredSGs {
			if _, inCurrent := currentSGs[sgName]; !inCurrent {
				err := secgroups.AddServer(ctx, i.Client.ComputeClient, id, sgName).ExtractErr()
				if err != nil {
					return &resource.UpdateResult{
						ProgressResult: &resource.ProgressResult{
							Operation:       resource.OperationUpdate,
							OperationStatus: resource.OperationStatusFailure,
							ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
							StatusMessage:   fmt.Sprintf("failed to add security group %s: %v", sgName, err),
						},
					}, nil
				}
			}
		}

		// Re-fetch server to get updated security groups for the response
		server, err = servers.Get(ctx, i.Client.ComputeClient, id).Extract()
		if err != nil {
			return &resource.UpdateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationUpdate,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
					StatusMessage:   fmt.Sprintf("failed to fetch updated instance: %v", err),
				},
			}, nil
		}
	}

	// Fetch network attachments using the attachinterfaces API
	var networkList []interface{}
	ifacePages, ifaceErr := attachinterfaces.List(i.Client.ComputeClient, server.ID).AllPages(ctx)
	if ifaceErr == nil {
		ifaces, extractErr := attachinterfaces.ExtractInterfaces(ifacePages)
		if extractErr == nil && len(ifaces) > 0 {
			networkList = make([]interface{}, 0, len(ifaces))
			for _, iface := range ifaces {
				networkList = append(networkList, map[string]interface{}{
					"uuid": iface.NetID,
				})
			}
		}
	}

	// Convert server to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(instanceToProperties(server, networkList))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        server.ID,
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
			NativeID:           server.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes an instance (async operation)
func (i *Instance) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the instance ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeInstance, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the server from OpenStack
	err := servers.Delete(ctx, i.Client.ComputeClient, id).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete instance: %v", err),
			},
		}, nil
	}

	// Check the server status to see if deletion is in progress
	server, err := servers.Get(ctx, i.Client.ComputeClient, id).Extract()
	if err != nil {
		// If we get NotFound, deletion completed immediately
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
		// Other errors during status check
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
				StatusMessage:   fmt.Sprintf("failed to check instance status after delete: %v", err),
			},
		}, nil
	}

	// If server is in "deleting" state, return InProgress
	if server.Status == "DELETED" || server.Status == "SOFT_DELETED" {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       id,
				NativeID:        id,
			},
		}, nil
	}

	// If server is in error state
	if server.Status == "ERROR" {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				StatusMessage:   fmt.Sprintf("instance deletion failed with status: %s", server.Status),
			},
		}, nil
	}

	// Otherwise, deletion is in progress
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			RequestID:       id,
			NativeID:        id,
		},
	}, nil
}

// Status checks the status of a long-running operation (Create/Delete)
func (i *Instance) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// Get the server by RequestID (which is the server ID)
	if request.RequestID == "" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
			},
		}, fmt.Errorf("requestID is required")
	}

	server, err := servers.Get(ctx, i.Client.ComputeClient, request.RequestID).Extract()
	if err != nil {
		// NotFound means the server was deleted (or never existed)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			// Server is gone - this is success for a delete operation
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        request.RequestID,
				},
			}, nil
		}

		// Other errors
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errCode,
			},
		}, fmt.Errorf("failed to get instance status: %w", err)
	}

	// Map server status to operation status
	// OpenStack server statuses: BUILD, ACTIVE, REBUILD, DELETED, SOFT_DELETED, ERROR, etc.
	switch server.Status {
	case "ACTIVE":
		// Server is ACTIVE, but we need to wait for network info to be populated
		// This can take a few seconds after the server becomes ACTIVE
		hasNetworkInfo := len(server.Addresses) > 0
		if !hasNetworkInfo {
			// Try attachinterfaces API as fallback
			ifacePages, err := attachinterfaces.List(i.Client.ComputeClient, server.ID).AllPages(ctx)
			if err == nil {
				ifaces, extractErr := attachinterfaces.ExtractInterfaces(ifacePages)
				hasNetworkInfo = (extractErr == nil && len(ifaces) > 0)
			}
		}

		if !hasNetworkInfo {
			// Network info not yet available - keep waiting
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationCreate,
					OperationStatus: resource.OperationStatusInProgress,
					RequestID:       server.ID,
					NativeID:        server.ID,
				},
			}, nil
		}

		// Server is ready with network info - fetch network attachments
		var networkList []interface{}
		ifacePages, ifaceErr := attachinterfaces.List(i.Client.ComputeClient, server.ID).AllPages(ctx)
		if ifaceErr == nil {
			ifaces, extractErr := attachinterfaces.ExtractInterfaces(ifacePages)
			if extractErr == nil && len(ifaces) > 0 {
				networkList = make([]interface{}, 0, len(ifaces))
				for _, iface := range ifaces {
					networkList = append(networkList, map[string]interface{}{
						"uuid": iface.NetID,
					})
				}
			}
		}

		// Convert server to properties and marshal to JSON
		propsJSON, marshalErr := resources.MarshalProperties(instanceToProperties(server, networkList))
		if marshalErr != nil {
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationCreate,
					OperationStatus: resource.OperationStatusFailure,
					NativeID:        server.ID,
					ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
				},
			}, fmt.Errorf("failed to marshal properties: %w", marshalErr)
		}

		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCreate,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           server.ID,
				ResourceProperties: []byte(propsJSON),
			},
		}, nil

	case "BUILD", "REBUILD":
		// Server creation/rebuild is in progress
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       server.ID,
				NativeID:        server.ID,
			},
		}, nil

	case "DELETED", "SOFT_DELETED":
		// Server deletion is in progress or complete
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       server.ID,
				NativeID:        server.ID,
			},
		}, nil

	case "ERROR":
		// Server operation failed - determine operation from context
		// If we can't determine, default to create error
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeGeneralServiceException,
			},
		}, fmt.Errorf("instance in error state: %s", server.Status)

	default:
		// Unknown state - treat as in progress (likely create)
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusInProgress,
				RequestID:       server.ID,
				NativeID:        server.ID,
			},
		}, nil
	}
}

// List discovers instances
func (i *Instance) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all servers using pagination
	allPages, err := servers.List(i.Client.ComputeClient, servers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list instances: %w", err)
	}

	// Extract servers from pages
	serverList, err := servers.ExtractServers(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract instances: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(serverList))
	for _, srv := range serverList {
		nativeIDs = append(nativeIDs, srv.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
