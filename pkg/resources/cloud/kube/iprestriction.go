// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// IpRestrictionResourceType is the resource type for Kubernetes IP restrictions.
const IpRestrictionResourceType = "OVH::Kube::IpRestriction"

// ipRestrictionProvisioner handles Kubernetes IP restriction operations.
//
// OVH exposes a single collection endpoint:
//
//	GET  /cloud/project/{p}/kube/{k}/ipRestrictions  -> ipBlock[] (array of CIDR strings)
//	POST /cloud/project/{p}/kube/{k}/ipRestrictions  body { ips: ipBlock[] } (append)
//	PUT  /cloud/project/{p}/kube/{k}/ipRestrictions  body { ips: ipBlock[] } (replace)
//
// There is no DELETE endpoint; removing one IP requires PUT-ing the remainder.
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

	log := plugin.LoggerFromContext(ctx)
	log.Debug("kube ipRestriction create request", "project", project, "kubeID", kubeID, "ip", ip)

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/ipRestrictions", project, kubeID)
	body := map[string]interface{}{"ips": []string{ip}}

	if _, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	}); err != nil {
		log.Debug("kube ipRestriction create error", "err", err)
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return createFailure(ovhtransport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
		}
		return createFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	nativeID := fmt.Sprintf("%s/%s/%s", project, kubeID, ip)
	propsJSON, _ := json.Marshal(map[string]interface{}{
		"ip":          ip,
		"serviceName": project,
		"kubeId":      kubeID,
	})

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

	currentIPs, err := p.listIPRestrictions(ctx, project, kubeID)
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}

	for _, existing := range currentIPs {
		if existing == ip {
			// Include URL-only fields (serviceName, kubeId) so discovered
			// resources pass formae's required-field validation.
			propsJSON, _ := json.Marshal(map[string]interface{}{
				"ip":          ip,
				"serviceName": project,
				"kubeId":      kubeID,
			})
			return &resource.ReadResult{Properties: string(propsJSON)}, nil
		}
	}

	return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
}

func (p *ipRestrictionProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, kubeID, ip, err := parseNestedNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	log := plugin.LoggerFromContext(ctx)

	currentIPs, err := p.listIPRestrictions(ctx, project, kubeID)
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

	remaining := make([]string, 0, len(currentIPs))
	for _, existing := range currentIPs {
		if existing != ip {
			remaining = append(remaining, existing)
		}
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/ipRestrictions", project, kubeID)
	body := map[string]interface{}{"ips": remaining}

	log.Debug("kube ipRestriction replace (delete)", "remaining", remaining)
	if _, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "PUT",
		Path:   url,
		Body:   body,
	}); err != nil {
		log.Debug("kube ipRestriction replace error", "err", err)
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

// List enumerates IP restrictions across the project. Discovery calls List
// with empty AdditionalProperties, so when no kubeId is supplied we iterate
// every cluster in the project.
func (p *ipRestrictionProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := extractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	if project == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	kubeIDs := []string{}
	if k := request.AdditionalProperties["kubeId"]; k != "" {
		kubeIDs = append(kubeIDs, k)
	} else {
		all, err := listClusterIDs(ctx, p.client, project)
		if err != nil {
			return nil, fmt.Errorf("failed to list kube clusters: %w", err)
		}
		kubeIDs = all
	}

	var nativeIDs []string
	for _, kubeID := range kubeIDs {
		currentIPs, err := p.listIPRestrictions(ctx, project, kubeID)
		if err != nil {
			// Cluster gone or transient — skip rather than fail the whole listing.
			continue
		}
		for _, ip := range currentIPs {
			nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, kubeID, ip))
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status is unused — IP restrictions are synchronous, so Create/Delete return
// Success directly and the operation list below does not register OperationCheckStatus.
// The method exists only to satisfy the prov.Provisioner interface.
func (p *ipRestrictionProvisioner) Status(_ context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}, nil
}

// Update is unused — IP restrictions have no mutable fields. The method exists
// only to satisfy the prov.Provisioner interface; OperationUpdate is not
// registered below, so this should never be invoked.
func (p *ipRestrictionProvisioner) Update(_ context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
		"IpRestriction has no mutable fields"), nil
}

// listIPRestrictions fetches current IP restrictions for a cluster. The OVH API
// returns a JSON array of CIDR strings (`ipBlock[]`).
func (p *ipRestrictionProvisioner) listIPRestrictions(ctx context.Context, project, kubeID string) ([]string, error) {
	url := fmt.Sprintf("/cloud/project/%s/kube/%s/ipRestrictions", project, kubeID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(response.BodyArray))
	for _, item := range response.BodyArray {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result, nil
}

func init() {
	registry.Register(
		IpRestrictionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &ipRestrictionProvisioner{client: client}
		},
	)
}
