// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/openstack/resources"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	openstack "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/openstack"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	ResourceTypeSecurityGroupRule = "OVH::Network::SecurityGroupRule"
)

// SecurityGroupRule provisioner
type SecurityGroupRule struct {
	Client *openstack.Client
	Config *openstack.Config
}

// securityGroupRuleToProperties converts an OpenStack security group rule to a properties map.
// This is used by Create, Read, and List to ensure consistent property marshaling.
func securityGroupRuleToProperties(rule *rules.SecGroupRule) map[string]any {
	props := map[string]any{
		"id":                rule.ID,
		"security_group_id": rule.SecGroupID,
		"direction":         rule.Direction,
		"ethertype":         rule.EtherType,
	}

	// Add optional fields only if set
	if rule.Protocol != "" {
		props["protocol"] = rule.Protocol
	}
	if rule.PortRangeMin != 0 {
		props["port_range_min"] = rule.PortRangeMin
	}
	if rule.PortRangeMax != 0 {
		props["port_range_max"] = rule.PortRangeMax
	}
	if rule.RemoteIPPrefix != "" {
		props["remote_ip_prefix"] = rule.RemoteIPPrefix
	}
	if rule.RemoteGroupID != "" {
		props["remote_group_id"] = rule.RemoteGroupID
	}
	if rule.Description != "" {
		props["description"] = rule.Description
	}

	return props
}

// Register the SecurityGroupRule resource type
func init() {
	registry.RegisterOpenStack(
		ResourceTypeSecurityGroupRule,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *openstack.Client, cfg *openstack.Config) prov.Provisioner {
			return &SecurityGroupRule{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new security group rule
func (s *SecurityGroupRule) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSecurityGroupRule, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Extract required fields
	secGroupID, ok := props["security_group_id"].(string)
	if !ok || secGroupID == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSecurityGroupRule, resource.OperationErrorCodeInvalidRequest, "", "security_group_id is required"),
		}, nil
	}

	direction, ok := props["direction"].(string)
	if !ok || direction == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSecurityGroupRule, resource.OperationErrorCodeInvalidRequest, "", "direction is required"),
		}, nil
	}

	ethertype, ok := props["ethertype"].(string)
	if !ok || ethertype == "" {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeSecurityGroupRule, resource.OperationErrorCodeInvalidRequest, "", "ethertype is required"),
		}, nil
	}

	// Build create options
	createOpts := rules.CreateOpts{
		SecGroupID: secGroupID,
		Direction:  rules.RuleDirection(direction),
		EtherType:  rules.RuleEtherType(ethertype),
	}

	// Add optional fields
	if protocol, ok := props["protocol"].(string); ok && protocol != "" {
		createOpts.Protocol = rules.RuleProtocol(protocol)
	}

	if portMin, ok := props["port_range_min"].(float64); ok {
		portMinInt := int(portMin)
		createOpts.PortRangeMin = portMinInt
	}

	if portMax, ok := props["port_range_max"].(float64); ok {
		portMaxInt := int(portMax)
		createOpts.PortRangeMax = portMaxInt
	}

	if remoteIPPrefix, ok := props["remote_ip_prefix"].(string); ok && remoteIPPrefix != "" {
		createOpts.RemoteIPPrefix = remoteIPPrefix
	}

	if remoteGroupID, ok := props["remote_group_id"].(string); ok && remoteGroupID != "" {
		createOpts.RemoteGroupID = remoteGroupID
	}

	if description, ok := props["description"].(string); ok {
		createOpts.Description = description
	}

	// Create the security group rule via OpenStack
	rule, err := rules.Create(ctx, s.Client.NetworkClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resources.MapOpenStackErrorToOperationErrorCode(err),
				StatusMessage:   fmt.Sprintf("failed to create security group rule: %v", err),
			},
		}, nil
	}

	// Convert rule to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(securityGroupRuleToProperties(rule))
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				NativeID:        rule.ID,
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
			NativeID:           rule.ID,
			ResourceProperties: []byte(propsJSON),
		},
	}, nil
}

// Read retrieves the current state of a security group rule
func (s *SecurityGroupRule) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the security group rule ID from NativeID
	id := request.NativeID
	if id == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil // Don't return Go error for expected errors
	}

	// Get the security group rule from OpenStack
	rule, err := rules.Get(ctx, s.Client.NetworkClient, id).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, nil // Don't return Go error for expected errors like NotFound
	}

	// Convert rule to properties and marshal to JSON
	propsJSON, err := resources.MarshalProperties(securityGroupRuleToProperties(rule))
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, nil // Don't return Go error for expected errors
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update returns an error because security group rules are immutable in OpenStack.
// To change a rule, users must delete and recreate it.
func (s *SecurityGroupRule) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Security group rules are immutable in OpenStack - cannot be updated
	return &resource.UpdateResult{
		ProgressResult: resources.NewFailureResultWithMessage(
			resource.OperationUpdate,
			ResourceTypeSecurityGroupRule,
			resource.OperationErrorCodeInvalidRequest,
			request.NativeID,
			"security group rules are immutable and cannot be updated; delete and recreate instead",
		),
	}, nil
}

// Delete removes a security group rule
func (s *SecurityGroupRule) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get the security group rule ID from NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeSecurityGroupRule, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	id := request.NativeID

	// Delete the security group rule from OpenStack
	err := rules.Delete(ctx, s.Client.NetworkClient, id).ExtractErr()
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
				StatusMessage:   fmt.Sprintf("failed to delete security group rule: %v", err),
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

// Status checks the status of a long-running operation (security group rules are synchronous, so not used)
func (s *SecurityGroupRule) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers security group rules
func (s *SecurityGroupRule) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all security group rules using pagination
	allPages, err := rules.List(s.Client.NetworkClient, rules.ListOpts{}).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list security group rules: %w", err)
	}

	// Extract rules from pages
	ruleList, err := rules.ExtractRules(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract security group rules: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(ruleList))
	for _, rule := range ruleList {
		nativeIDs = append(nativeIDs, rule.ID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
