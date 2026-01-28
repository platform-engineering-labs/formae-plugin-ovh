// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// S3BucketResourceType is the resource type for S3-compatible storage buckets.
const S3BucketResourceType = "OVH::Storage::S3Bucket"

// s3BucketProvisioner handles S3 bucket operations.
type s3BucketProvisioner struct {
	client *ovhtransport.Client
}

var _ prov.Provisioner = &s3BucketProvisioner{}

func (p *s3BucketProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return s3CreateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project := s3ExtractProject(request.TargetConfig, props)
	if project == "" {
		return s3CreateFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceName is required"), nil
	}

	region, _ := props["region"].(string)
	if region == "" {
		return s3CreateFailure(resource.OperationErrorCodeInvalidRequest,
			"region is required"), nil
	}

	// Derive short region for OVH Cloud storage API (DE1 → DE, GRA7 → GRA)
	shortRegion := base.DeriveShortRegion(region)

	name, _ := props["name"].(string)
	if name == "" {
		return s3CreateFailure(resource.OperationErrorCodeInvalidRequest,
			"name is required"), nil
	}

	// Build URL: POST /cloud/project/{serviceName}/region/{regionName}/storage
	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage", project, shortRegion)

	// Strip serviceName and region from body (they're in the URL)
	body := s3FilterProps(props, "serviceName", "region")

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		return s3HandleTransportError(err), nil
	}

	// Native ID: project/region/name (uses short region for consistency)
	nativeID := fmt.Sprintf("%s/%s/%s", project, shortRegion, name)

	propsJSON, _ := json.Marshal(response.Body)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *s3BucketProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	project, region, name, err := parseS3NativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage/%s", project, region, name)

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

	propsJSON, _ := json.Marshal(response.Body)
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

func (p *s3BucketProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return s3UpdateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	project, region, name, err := parseS3NativeID(request.NativeID)
	if err != nil {
		return s3UpdateFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage/%s", project, region, name)

	// Strip immutable fields
	body := s3FilterProps(props, "serviceName", "region", "name", "ownerId", "objectLock")

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "PUT",
		Path:   url,
		Body:   body,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return s3UpdateFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return s3UpdateFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	propsJSON, _ := json.Marshal(response.Body)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           request.NativeID,
			ResourceProperties: propsJSON,
		},
	}, nil
}

func (p *s3BucketProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, region, name, err := parseS3NativeID(request.NativeID)
	if err != nil {
		return s3DeleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage/%s", project, region, name)

	_, err = p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "DELETE",
		Path:   url,
	})
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
			return s3DeleteFailure(request.NativeID, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return s3DeleteFailure(request.NativeID, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
		},
	}, nil
}

func (p *s3BucketProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	project := s3ExtractProjectFromAdditional(request.TargetConfig, request.AdditionalProperties)
	if project == "" {
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	region := request.AdditionalProperties["region"]
	if region == "" {
		// Region is required for listing S3 buckets
		return &resource.ListResult{NativeIDs: nil}, nil
	}

	// Derive short region for OVH Cloud storage API
	shortRegion := base.DeriveShortRegion(region)

	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage", project, shortRegion)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	var nativeIDs []string
	for _, item := range response.BodyArray {
		if bucket, ok := item.(map[string]interface{}); ok {
			if name, ok := bucket["name"].(string); ok {
				nativeIDs = append(nativeIDs, fmt.Sprintf("%s/%s/%s", project, shortRegion, name))
			}
		}
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (p *s3BucketProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	// S3 bucket creation is synchronous, just read and return success
	project, region, name, err := parseS3NativeID(request.NativeID)
	if err != nil {
		return s3StatusFailure(request, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	url := fmt.Sprintf("/cloud/project/%s/region/%s/storage/%s", project, region, name)

	response, err := p.client.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   url,
	})
	if err != nil {
		if transportErr, ok := err.(*ovhtransport.Error); ok {
			return s3StatusFailure(request, ovhtransport.ToResourceErrorCode(transportErr.Code),
				transportErr.Message), nil
		}
		return s3StatusFailure(request, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	propsJSON, _ := json.Marshal(response.Body)

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

// parseS3NativeID parses "project/region/name" format
func parseS3NativeID(nativeID string) (project, region, name string, err error) {
	parts := strings.SplitN(nativeID, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid S3 bucket native ID: %s", nativeID)
	}
	return parts[0], parts[1], parts[2], nil
}

// Helper functions with s3 prefix to avoid conflicts with resources.go

func s3ExtractProject(targetConfig json.RawMessage, props map[string]interface{}) string {
	if serviceName, ok := props["serviceName"].(string); ok && serviceName != "" {
		return serviceName
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}

	projectFields := []string{"ProjectId", "projectId", "ServiceName", "serviceName"}
	for _, field := range projectFields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func s3ExtractProjectFromAdditional(targetConfig json.RawMessage, additionalProps map[string]string) string {
	if serviceName, ok := additionalProps["serviceName"]; ok && serviceName != "" {
		return serviceName
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(targetConfig, &cfg); err != nil {
		return ""
	}

	projectFields := []string{"ProjectId", "projectId", "ServiceName", "serviceName"}
	for _, field := range projectFields {
		if val, ok := cfg[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func s3FilterProps(props map[string]interface{}, keys ...string) map[string]interface{} {
	result := make(map[string]interface{})
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	for k, v := range props {
		if keySet[k] {
			continue
		}
		if v == nil {
			continue
		}
		result[k] = v
	}
	return result
}

func s3CreateFailure(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

func s3UpdateFailure(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

func s3DeleteFailure(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

func s3StatusFailure(request *resource.StatusRequest, errorCode resource.OperationErrorCode, message string) *resource.StatusResult {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			RequestID:       request.RequestID,
			NativeID:        request.NativeID,
		},
	}
}

func s3HandleTransportError(err error) *resource.CreateResult {
	if transportErr, ok := err.(*ovhtransport.Error); ok {
		return s3CreateFailure(ovhtransport.ToResourceErrorCode(transportErr.Code), transportErr.Message)
	}
	return s3CreateFailure(resource.OperationErrorCodeServiceInternalError, err.Error())
}

func init() {
	registry.Register(
		S3BucketResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return &s3BucketProvisioner{client: client}
		},
	)
}
