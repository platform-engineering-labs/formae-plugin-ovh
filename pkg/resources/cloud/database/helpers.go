// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// extractProject extracts project from target config or props
func extractProject(targetConfig json.RawMessage, props map[string]interface{}) string {
	// Try props first
	if serviceName, ok := props["serviceName"].(string); ok && serviceName != "" {
		return serviceName
	}

	// Try target config
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

// extractProjectFromAdditional extracts project from target config or additional props
func extractProjectFromAdditional(targetConfig json.RawMessage, additionalProps map[string]string) string {
	// Try additional props first
	if serviceName, ok := additionalProps["serviceName"]; ok && serviceName != "" {
		return serviceName
	}

	// Try target config
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

// filterProps returns a copy of props without the specified keys
func filterProps(props map[string]interface{}, keys ...string) map[string]interface{} {
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
			continue // OVH API rejects null values
		}
		result[k] = v
	}
	return result
}

// parseNestedNativeID parses "project/engine/clusterId/resourceId" format
func parseNestedNativeID(nativeID string) (project, engine, clusterID, resourceID string, err error) {
	parts := strings.SplitN(nativeID, "/", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("invalid nested native ID: %s", nativeID)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// createFailure creates a failure result for Create operations
func createFailure(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

// updateFailure creates a failure result for Update operations
func updateFailure(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.UpdateResult {
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

// deleteFailure creates a failure result for Delete operations
func deleteFailure(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.DeleteResult {
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

// statusFailure creates a failure result for Status operations
func statusFailure(request *resource.StatusRequest, errorCode resource.OperationErrorCode, message string) *resource.StatusResult {
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

// handleTransportError converts transport errors to CreateResult
func handleTransportError(err error) *resource.CreateResult {
	if transportErr, ok := err.(*ovhtransport.Error); ok {
		return createFailure(ovhtransport.ToResourceErrorCode(transportErr.Code), transportErr.Message)
	}
	return createFailure(resource.OperationErrorCodeServiceInternalError, err.Error())
}

// transformNodesPatternRegion transforms region in nodesPattern to short format.
// OVH database API expects short region codes (DE, GRA) not OpenStack codes (DE1, GRA7).
// This modifies the body map in place.
func transformNodesPatternRegion(body map[string]interface{}) {
	nodesPattern, ok := body["nodesPattern"].(map[string]interface{})
	if !ok {
		return
	}

	if region, ok := nodesPattern["region"].(string); ok && region != "" {
		nodesPattern["region"] = base.DeriveShortRegion(region)
	}
}
