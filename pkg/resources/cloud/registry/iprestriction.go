// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package registry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// IpRestrictionResourceType is the resource type for registry IP restrictions.
const IpRestrictionResourceType = "OVH::Registry::IpRestriction"

// ipRestrictionProvisioner handles registry IP restriction operations.
// The OVH API uses bulk PUT endpoints:
// PUT /cloud/project/{project}/containerRegistry/{registryId}/ipRestrictions/management
// PUT /cloud/project/{project}/containerRegistry/{registryId}/ipRestrictions/registry
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
	registryID := resolveString(props["registryId"])
	ipBlock, _ := props["ipBlock"].(string)
	restrictionType, _ := props["type"].(string)

	if project == "" || registryID == "" || ipBlock == "" || restrictionType == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName, registryId, ipBlock, and type are required"), nil
	}

	if restrictionType != "management" && restrictionType != "registry" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"type must be 'management' or 'registry'"), nil
	}

	// Get current IP restrictions for this type
	currentIPs, err := p.getIPRestrictions(ctx, project, registryID, restrictionType)
	if err != nil {
		return handleTransportError(err), nil
	}

	// Check if IP already exists
	for _, existing := range currentIPs {
		if existingIP, _ := existing["ipBlock"].(string); existingIP == ipBlock {
			// Already exists
			nativeID := fmt.Sprintf("%s/%s/%s/%s", project, registryID, restrictionType, ipBlock)
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
		"ipBlock": ipBlock,
	}
	if desc, ok := props["description"].(string); ok && desc != "" {
		newRestriction["description"] = desc
	}

	currentIPs = append(currentIPs, newRestriction)

	// Put the updated list
	if err := p.putIPRestrictions(ctx, project, registryID, restrictionType, currentIPs); err != nil {
		return handleTransportError(err), nil
	}

	// Native ID: project/registryId/type/ipBlock
	nativeID := fmt.Sprintf("%s/%s/%s/%s", project, registryID, restrictionType, ipBlock)

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
	project, registryID, restrictionType, ipBlock, err := parseIPRestrictionNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, registryID, restrictionType)
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
		if existingIP, _ := existing["ipBlock"].(string); existingIP == ipBlock {
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

	project, registryID, restrictionType, ipBlock, err := parseIPRestrictionNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, registryID, restrictionType)
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// Find and update
	found := false
	for i, existing := range currentIPs {
		if existingIP, _ := existing["ipBlock"].(string); existingIP == ipBlock {
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

	if err := p.putIPRestrictions(ctx, project, registryID, restrictionType, currentIPs); err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	for _, existing := range currentIPs {
		if existingIP, _ := existing["ipBlock"].(string); existingIP == ipBlock {
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
	project, registryID, restrictionType, ipBlock, err := parseIPRestrictionNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	currentIPs, err := p.getIPRestrictions(ctx, project, registryID, restrictionType)
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
		if existingIP, _ := existing["ipBlock"].(string); existingIP != ipBlock {
			newIPs = append(newIPs, existing)
		}
	}

	if err := p.putIPRestrictions(ctx, project, registryID, restrictionType, newIPs); err != nil {
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
	registryID := request.AdditionalProperties["registryId"]
	restrictionType := request.AdditionalProperties["type"]

	if project == "" || registryID == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	var nativeIDs []string

	// If type is specified, list only that type
	if restrictionType != "" {
		currentIPs, err := p.getIPRestrictions(ctx, project, registryID, restrictionType)
		if err != nil {
			return nil, fmt.Errorf("failed to list IP restrictions: %w", err)
		}

		for _, item := range currentIPs {
			if ip, ok := item["ipBlock"].(string); ok {
				nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s/%s", project, registryID, restrictionType, ip))
			}
		}
	} else {
		// List both types
		for _, t := range []string{"management", "registry"} {
			currentIPs, err := p.getIPRestrictions(ctx, project, registryID, t)
			if err != nil {
				continue // Skip if error
			}

			for _, item := range currentIPs {
				if ip, ok := item["ipBlock"].(string); ok {
					nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s/%s", project, registryID, t, ip))
				}
			}
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

// getIPRestrictions fetches current IP restrictions for a registry
func (p *ipRestrictionProvisioner) getIPRestrictions(ctx context.Context, project, registryID, restrictionType string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/ipRestrictions/%s",
		project, registryID, restrictionType)

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

// putIPRestrictions updates IP restrictions for a registry
func (p *ipRestrictionProvisioner) putIPRestrictions(ctx context.Context, project, registryID, restrictionType string, ips []map[string]interface{}) error {
	url := fmt.Sprintf("/cloud/project/%s/containerRegistry/%s/ipRestrictions/%s",
		project, registryID, restrictionType)

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
