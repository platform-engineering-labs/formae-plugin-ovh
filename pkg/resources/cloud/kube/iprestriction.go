// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// IpRestrictionResourceType is the resource type for Kubernetes IP restrictions.
const IpRestrictionResourceType = "OVH::Kube::IpRestriction"

// ipRestrictionProvisioner handles Kubernetes IP restriction operations.
// The OVH API uses a bulk PUT endpoint, so we read-modify-write.
// Path: PUT /cloud/project/{project}/kube/{kubeId}/ipRestrictions
type ipRestrictionProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &ipRestrictionProvisioner{}

func (p *ipRestrictionProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	kubeID := resolveString(props["kubeId"])
	ip, _ := props["ip"].(string)

	if project == "" || kubeID == "" || ip == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName, kubeId, and ip are required"), nil
	}

	// Get current IP restrictions
	currentIPs, err := p.getIPRestrictions(ctx, project, kubeID)
	if err != nil {
		return handleTransportError(err), nil
	}

	// Check if IP already exists
	for _, existing := range currentIPs {
		if existingIP, _ := existing["ip"].(string); existingIP == ip {
			// Already exists, return success with existing data
			nativeID := fmt.Sprintf("%s/%s/%s", project, kubeID, ip)
			propsJSON, _ := json.Marshal(existing)
			return &resource.CreateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:          resource.OperationCreate,
					OperationStatus:    resource.OperationStatusSuccess,
					NativeID:           nativeID,
					ResourceProperties: propsJSON,
				},
			}, nil
		}
	}

	// Add new IP restriction
	newRestriction := map[string]interface{}{
		"ip": ip,
	}
	if desc, ok := props["description"].(string); ok && desc != "" {
		newRestriction["description"] = desc
	}

	currentIPs = append(currentIPs, newRestriction)

	// Put the updated list
	if err := p.putIPRestrictions(ctx, project, kubeID, currentIPs); err != nil {
		return handleTransportError(err), nil
	}

	// Native ID: project/kubeId/ip
	nativeID := fmt.Sprintf("%s/%s/%s", project, kubeID, ip)

	propsJSON, _ := json.Marshal(newRestriction)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *ipRestrictionProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, kubeID, ip, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, kubeID)
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}

	// Find the specific IP
	for _, existing := range currentIPs {
		if existingIP, _ := existing["ip"].(string); existingIP == ip {
			propsJSON, _ := json.Marshal(existing)
			return &resource.ReadResult{Properties: string(propsJSON)}, nil
		}
	}

	return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
}

func (p *ipRestrictionProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, kubeID, ip, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, kubeID)
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Find and update the specific IP
	found := false
	for i, existing := range currentIPs {
		if existingIP, _ := existing["ip"].(string); existingIP == ip {
			// Update description if provided
			if desc, ok := props["description"].(string); ok {
				currentIPs[i]["description"] = desc
			}
			found = true
			break
		}
	}

	if !found {
		return updateFailure(request.NativeID, resource.OperationErrorCodeNotFound, "IP restriction not found"), nil
	}

	// Put the updated list
	if err := p.putIPRestrictions(ctx, project, kubeID, currentIPs); err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Return updated properties
	for _, existing := range currentIPs {
		if existingIP, _ := existing["ip"].(string); existingIP == ip {
			propsJSON, _ := json.Marshal(existing)
			return &resource.UpdateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:          resource.OperationUpdate,
					OperationStatus:    resource.OperationStatusSuccess,
					NativeID:           request.NativeID,
					ResourceProperties: propsJSON,
				},
			}, nil
		}
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *ipRestrictionProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, kubeID, ip, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, kubeID)
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			if transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.DeleteResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationDelete,
						OperationStatus: resource.OperationStatusSuccess,
						NativeID:        request.NativeID,
					},
				}, nil
			}
			return deleteFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return deleteFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Remove the specific IP
	var newIPs []map[string]interface{}
	for _, existing := range currentIPs {
		if existingIP, _ := existing["ip"].(string); existingIP != ip {
			newIPs = append(newIPs, existing)
		}
	}

	// Put the updated list
	if err := p.putIPRestrictions(ctx, project, kubeID, newIPs); err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return deleteFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return deleteFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *ipRestrictionProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	kubeID := request.AdditionalProperties["kubeId"]

	if project == "" || kubeID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, kubeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list IP restrictions: %w", err)
	}

	var nativeIDs []string
	for _, item := range currentIPs {
		if ip, ok := item["ip"].(string); ok {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, kubeID, ip))
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *ipRestrictionProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

// getIPRestrictions fetches current IP restrictions for a cluster
func (p *ipRestrictionProvisioner) getIPRestrictions(ctx context.Context, project, kubeID string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("/cloud/project/%s/kube/%s/ipRestrictions", project, kubeID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for _, item := range response.BodyArray {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}

	return result, nil
}

// putIPRestrictions updates IP restrictions for a cluster
func (p *ipRestrictionProvisioner) putIPRestrictions(ctx context.Context, project, kubeID string, ips []map[string]interface{}) error {
	url := fmt.Sprintf("/cloud/project/%s/kube/%s/ipRestrictions", project, kubeID)

	// Convert to interface{} slice for the API (transport Body accepts interface{})
	body := make([]interface{}, len(ips))
	for i, ip := range ips {
		body[i] = ip
	}

	_, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "PUT",
		Path:   url,
		Body:   body,
	})

	return err
}

func init() {
	registry.Register(
		IpRestrictionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &ipRestrictionProvisioner{client: client}
		},
	)
}
