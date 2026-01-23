// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
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
	ResourceTypePort = "OVH::Network::Port"
)

// Port schema and descriptor
var (
	PortDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypePort,
		Discoverable: true,
	}

	PortSchema = model.Schema{
		Identifier:   "id",
		Discoverable: true,
		Fields:       []string{"name", "description", "network_id", "fixed_ips", "security_groups", "admin_state_up", "mac_address", "allowed_address_pairs", "tags"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required: false,
			},
			"description": {
				Required:   false,
				CreateOnly: true, // OVH does not support updating description on ports
			},
			"network_id": {
				Required:   true,
				CreateOnly: true,
			},
			"fixed_ips": {
				Required:   false,
				CreateOnly: true,
			},
			"security_groups": {
				Required: false,
			},
			"admin_state_up": {
				Required: false,
			},
			"mac_address": {
				Required:   false,
				CreateOnly: true,
			},
			"allowed_address_pairs": {
				Required: false,
			},
			"tags": {
				Required: false,
			},
		},
	}
)

// Port provisioner
type Port struct {
	Client *client.Client
	Config *config.Config
}

// portToProperties converts an OpenStack port to a properties map.
// This is used by Create, Read, Update, and List to ensure consistent property marshaling.
func portToProperties(port *ports.Port) map[string]interface{} {
	props := map[string]interface{}{
		"id":             port.ID,
		"network_id":     port.NetworkID,
		"name":           port.Name,
		"description":    port.Description,
		"admin_state_up": port.AdminStateUp,
		"mac_address":    port.MACAddress,
	}

	// Add fixed_ips if present
	if len(port.FixedIPs) > 0 {
		fixedIPs := make([]map[string]interface{}, 0, len(port.FixedIPs))
		for _, fip := range port.FixedIPs {
			fixedIPs = append(fixedIPs, map[string]interface{}{
				"subnet_id":  fip.SubnetID,
				"ip_address": fip.IPAddress,
			})
		}
		props["fixed_ips"] = fixedIPs
	}

	// Add security_groups if present
	if len(port.SecurityGroups) > 0 {
		props["security_groups"] = port.SecurityGroups
	}

	// Add allowed_address_pairs if present
	if len(port.AllowedAddressPairs) > 0 {
		pairs := make([]map[string]interface{}, 0, len(port.AllowedAddressPairs))
		for _, pair := range port.AllowedAddressPairs {
			p := map[string]interface{}{
				"ip_address": pair.IPAddress,
			}
			// Only include mac_address if it differs from the port's own MAC address
			// OpenStack defaults to the port's MAC when not specified, so we omit it
			// to avoid drift detection issues
			if pair.MACAddress != "" && pair.MACAddress != port.MACAddress {
				p["mac_address"] = pair.MACAddress
			}
			pairs = append(pairs, p)
		}
		props["allowed_address_pairs"] = pairs
	}

	// Add tags if present
	if len(port.Tags) > 0 {
		props["tags"] = port.Tags
	}

	return props
}

// Register the Port resource type
func init() {
	registry.Register(
		ResourceTypePort,
		PortDescriptor,
		PortSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Port{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new port
func (p *Port) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypePort, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Build create options - NetworkID is required
	networkID, ok := props["network_id"].(string)
	if !ok || networkID == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypePort, resource.OperationErrorCodeInvalidRequest, "", "network_id is required"),
		}, nil
	}

	createOpts := ports.CreateOpts{
		NetworkID: networkID,
	}

	// Add optional name
	if name, ok := props["name"].(string); ok && name != "" {
		createOpts.Name = name
	}

	// Add optional description
	if description, ok := props["description"].(string); ok {
		createOpts.Description = description
	}

	// Add optional fixed_ips (for subnet association)
	if fixedIPsRaw, ok := props["fixed_ips"].([]interface{}); ok && len(fixedIPsRaw) > 0 {
		fixedIPs := make([]ports.IP, 0, len(fixedIPsRaw))
		for _, fipRaw := range fixedIPsRaw {
			if fipMap, ok := fipRaw.(map[string]interface{}); ok {
				ip := ports.IP{}
				if subnetID, ok := fipMap["subnet_id"].(string); ok {
					ip.SubnetID = subnetID
				}
				if ipAddr, ok := fipMap["ip_address"].(string); ok {
					ip.IPAddress = ipAddr
				}
				fixedIPs = append(fixedIPs, ip)
			}
		}
		createOpts.FixedIPs = fixedIPs
	}

	// Add optional security groups
	if sgRaw, ok := props["security_groups"].([]interface{}); ok && len(sgRaw) > 0 {
		securityGroups := make([]string, 0, len(sgRaw))
		for _, sg := range sgRaw {
			if sgID, ok := sg.(string); ok {
				securityGroups = append(securityGroups, sgID)
			}
		}
		createOpts.SecurityGroups = &securityGroups
	}

	// Add optional admin_state_up
	if adminStateUp, ok := props["admin_state_up"].(bool); ok {
		createOpts.AdminStateUp = &adminStateUp
	}

	// Add optional mac_address
	if macAddress, ok := props["mac_address"].(string); ok && macAddress != "" {
		createOpts.MACAddress = macAddress
	}

	// Add optional allowed_address_pairs (for HA configurations)
	if pairsRaw, ok := props["allowed_address_pairs"].([]interface{}); ok && len(pairsRaw) > 0 {
		pairs := make([]ports.AddressPair, 0, len(pairsRaw))
		for _, pairRaw := range pairsRaw {
			if pairMap, ok := pairRaw.(map[string]interface{}); ok {
				pair := ports.AddressPair{}
				if ipAddr, ok := pairMap["ip_address"].(string); ok {
					pair.IPAddress = ipAddr
				}
				if macAddr, ok := pairMap["mac_address"].(string); ok {
					pair.MACAddress = macAddr
				}
				pairs = append(pairs, pair)
			}
		}
		createOpts.AllowedAddressPairs = pairs
	}

	// Create the port via OpenStack
	port, err := ports.Create(ctx, p.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create port: %v", err),
			},
		}, nil
	}

	// Set tags if provided (must be done after creation via attributestags API)
	tags := resources.ParseTags(props["tags"])
	if len(tags) > 0 {
		_, err = attributestags.ReplaceAll(ctx, p.Client.NetworkClient, "ports", port.ID, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - port was created successfully
			fmt.Printf("warning: failed to set tags on port %s: %v\n", port.ID, err)
		} else {
			port.Tags = tags
		}
	}

	// Convert port to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(portToProperties(port))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        port.ID,
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
			NativeID:           port.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a port
func (p *Port) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the port ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the port from OpenStack
	port, err := ports.Get(ctx, p.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read port: %w", err)
	}

	// Explicitly fetch tags - OpenStack often doesn't include them in the standard GET response
	tags, err := attributestags.List(ctx, p.Client.NetworkClient, "ports", id).Extract()
	if err != nil {
		// Log warning but continue - tags are optional
		fmt.Printf("warning: failed to fetch tags for port %s: %v\n", id, err)
	} else {
		port.Tags = tags
	}

	// Convert port to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(portToProperties(port))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update updates an existing port
func (p *Port) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get the port ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypePort, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Parse request properties
	props, err := resources.ParseProperties(request.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationUpdate, ResourceTypePort, resource.OperationErrorCodeInvalidRequest, id, err.Error()),
		}, nil
	}

	// Build update options
	updateOpts := ports.UpdateOpts{}

	// Update mutable fields
	if name, ok := props["name"].(string); ok {
		updateOpts.Name = &name
	}

	// Note: Description is CreateOnly on OVH - cannot be updated after creation

	if adminStateUp, ok := props["admin_state_up"].(bool); ok {
		updateOpts.AdminStateUp = &adminStateUp
	}

	// Update security groups if provided
	if sgRaw, ok := props["security_groups"].([]interface{}); ok {
		securityGroups := make([]string, 0, len(sgRaw))
		for _, sg := range sgRaw {
			if sgID, ok := sg.(string); ok {
				securityGroups = append(securityGroups, sgID)
			}
		}
		updateOpts.SecurityGroups = &securityGroups
	}

	// Update allowed_address_pairs if provided
	if pairsRaw, ok := props["allowed_address_pairs"].([]interface{}); ok {
		pairs := make([]ports.AddressPair, 0, len(pairsRaw))
		for _, pairRaw := range pairsRaw {
			if pairMap, ok := pairRaw.(map[string]interface{}); ok {
				pair := ports.AddressPair{}
				if ipAddr, ok := pairMap["ip_address"].(string); ok {
					pair.IPAddress = ipAddr
				}
				if macAddr, ok := pairMap["mac_address"].(string); ok {
					pair.MACAddress = macAddr
				}
				pairs = append(pairs, pair)
			}
		}
		updateOpts.AllowedAddressPairs = &pairs
	}

	// Update the port via OpenStack
	port, err := ports.Update(ctx, p.Client.NetworkClient, id, updateOpts).Extract()
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to update port: %v", err),
			},
		}, nil
	}

	// Update tags if provided (via attributestags API)
	if _, hasTags := props["tags"]; hasTags {
		tags := resources.ParseTags(props["tags"])
		if tags == nil {
			tags = []string{} // Empty slice to clear all tags
		}
		updatedTags, err := attributestags.ReplaceAll(ctx, p.Client.NetworkClient, "ports", id, attributestags.ReplaceAllOpts{
			Tags: tags,
		}).Extract()
		if err != nil {
			// Log warning but don't fail - port was updated successfully
			fmt.Printf("warning: failed to update tags on port %s: %v\n", id, err)
		} else {
			port.Tags = updatedTags
		}
	}

	// Convert port to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(portToProperties(port))
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        port.ID,
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
			NativeID:           port.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Delete removes a port
func (p *Port) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the port ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypePort, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the port from OpenStack
	err := ports.Delete(ctx, p.Client.NetworkClient, id).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete port: %v", err),
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

// Status checks the status of a long-running operation (ports are synchronous, so not used)
func (p *Port) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers ports
func (p *Port) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all ports using pagination
	allPages, err := ports.List(p.Client.NetworkClient, ports.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list ports: %w", err)
	}

	// Extract ports from pages
	portList, err := ports.ExtractPorts(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract ports: %w", err)
	}

	// Collect NativeIDs for discovery (skip ports attached to devices)
	nativeIDs := make([]string, 0, len(portList))
	for _, port := range portList {
		// Skip ports that are attached to devices (like instances or routers)
		// These are managed by their parent resources
		if port.DeviceID != "" {
			continue
		}
		nativeIDs = append(nativeIDs, port.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
