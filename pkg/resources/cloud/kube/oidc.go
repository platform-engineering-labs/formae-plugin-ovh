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

// OidcResourceType is the resource type for Kubernetes OIDC configuration.
const OidcResourceType = "OVH::Kube::Oidc"

// oidcProvisioner handles Kubernetes OIDC configuration operations.
//
// OIDC is a singleton per cluster. OVH exposes:
//
//	GET    /cloud/project/{p}/kube/{k}/openIdConnect      -> cloud.kube.OpenIdConnect
//	POST   /cloud/project/{p}/kube/{k}/openIdConnect      body cloud.ProjectKubeOpenIdConnectCreation (create)
//	PUT    /cloud/project/{p}/kube/{k}/openIdConnect      body cloud.ProjectKubeOpenIdConnectUpdate   (edit)
//	DELETE /cloud/project/{p}/kube/{k}/openIdConnect
//
// Configuring OIDC reconfigures the cluster API server, so the cluster enters
// REDEPLOYING state for ~30s after each Create/Update/Delete. Subsequent
// operations against the same cluster (e.g. an Update right after Create) get
// rejected while it is REDEPLOYING. We therefore return InProgress and use
// Status to poll the parent cluster until status=READY.
type oidcProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &oidcProvisioner{}

func (p *oidcProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := extractProject(request.TargetConfig, props)
	kubeID := resolveString(props["kubeId"])

	if project == "" || kubeID == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName and kubeId are required"), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID)
	body := filterProps(props, "serviceName", "kubeId")

	log := plugin.LoggerFromContext(ctx)
	log.Debug("kube oidc create request", "url", url, "body", body)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		log.Debug("kube oidc create error", "err", err)
		return handleTransportError(err), nil
	}
	log.Debug("kube oidc create response", "body", response.Body)

	nativeID := fmt.Sprintf("%s/%s", project, kubeID)
	propsJSON, _ := json.Marshal(response.Body)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusInProgress,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *oidcProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, kubeID, err := parseClusterNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return &resource.ReadResult{
				ErrorCode: ovhtransport.ToResourceErrorCode(transportErr.Code),
			}, nil
		}
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}

	// OVH's GET returns 200 with empty fields when OIDC is not configured (post-delete)
	// rather than 404. An empty clientId means no integration exists, so report NotFound
	// - without this check, formae's sync never detects out-of-band deletions.
	if clientID, _ := response.Body["clientId"].(string); clientID == "" {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	// serviceName and kubeId are URL parameters, not in the OVH API body. Inject
	// them so discovery sync passes formae's required-field validation.
	if response.Body != nil {
		response.Body["serviceName"] = project
		response.Body["kubeId"] = kubeID
	}

	propsJSON, _ := json.Marshal(response.Body)
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

func (p *oidcProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, kubeID, err := parseClusterNativeID(request.NativeID)
	if err != nil {
		return updateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID)
	body := filterProps(props, "serviceName", "kubeId")

	log := plugin.LoggerFromContext(ctx)
	log.Debug("kube oidc update request", "url", url, "body", body)

	_, err = p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "PUT",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		log.Debug("kube oidc update error", "err", err)
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return updateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return updateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	// PUT returns void - re-read the current state to populate properties.
	read, err := p.client.Do(ctx, ovhtransport.RequestOptions{Method: "GET", Path: url})
	var propsJSON []byte
	if err == nil {
		propsJSON, _ = json.Marshal(read.Body)
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusInProgress,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *oidcProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, kubeID, err := parseClusterNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID)

	log := plugin.LoggerFromContext(ctx)
	log.Debug("kube oidc delete request", "url", url)

	_, err = p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "DELETE",
		Path:   url,
	})
	if err != nil {
		log.Debug("kube oidc delete error", "err", err)
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

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
		},
	}, nil
}

// List enumerates OIDC integrations across the project. Discovery calls List
// with empty AdditionalProperties, so when no kubeId is supplied we iterate
// every cluster in the project. OIDC is a singleton per cluster - at most one
// native ID per cluster.
func (p *oidcProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
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
		response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
			Method: "GET",
			Path:   fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID),
		})
		if err != nil {
			// No OIDC configured (or 404) - skip this cluster.
			continue
		}
		// OVH returns 200 with an empty body when OIDC is not configured; only
		// report a native ID when an integration is actually present.
		if clientID, _ := response.Body["clientId"].(string); clientID == "" {
			continue
		}
		nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s", project, kubeID))
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status polls the parent cluster's status. OIDC has no state of its own; the
// API-server reconfiguration shows up as the cluster transitioning through
// REDEPLOYING and back to READY. Once the cluster is READY again, the OIDC
// change has applied.
func (p *oidcProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	project, kubeID, err := parseClusterNativeID(request.NativeID)
	if err != nil {
		return statusFailure(request, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	clusterURL := fmt.Sprintf("/cloud/project/%s/kube/%s", project, kubeID)
	log := plugin.LoggerFromContext(ctx)
	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   clusterURL,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			// Cluster (and therefore OIDC) is gone - treat as terminal Success
			// so a Delete poll completes cleanly.
			if transportErr.Code == ovhtransport.ErrorCodeResourceNotFound {
				return &resource.StatusResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationCheckStatus,
						OperationStatus: resource.OperationStatusSuccess,
						RequestID:       request.RequestID,
						NativeID:        request.NativeID,
					},
				}, nil
			}
		}
		// Any other transport error (5xx, network blip, OVH rate-limit) while
		// the cluster is mid-REDEPLOYING is transient - keep polling instead
		// of failing the whole Update/Create command.
		log.Debug("kube oidc status: cluster GET transient error, retrying", "err", err)
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   fmt.Sprintf("transient cluster GET error: %v", err),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	clusterStatus, _ := response.Body["status"].(string)
	if clusterStatus != "READY" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   fmt.Sprintf("Parent cluster status: %s", clusterStatus),
				RequestID:       request.RequestID,
				NativeID:        request.NativeID,
			},
		}, nil
	}

	// Cluster READY - fetch the current OIDC config to populate properties.
	oidcURL := fmt.Sprintf("/cloud/project/%s/kube/%s/openIdConnect", project, kubeID)
	oidc, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   oidcURL,
	})
	var propsJSON []byte
	if err == nil {
		// Inject URL-only fields so ResourceProperties satisfies required-field validation.
		if oidc.Body != nil {
			oidc.Body["serviceName"] = project
			oidc.Body["kubeId"] = kubeID
		}
		propsJSON, _ = json.Marshal(oidc.Body)
	}

	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCheckStatus,
			OperationStatus:    resource.OperationStatusSuccess,
			RequestID:          request.RequestID,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func init() {
	registry.Register(
		OidcResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &oidcProvisioner{client: client}
		},
	)
}
