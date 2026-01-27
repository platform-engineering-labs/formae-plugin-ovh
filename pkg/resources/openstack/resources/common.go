// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ParseProperties unmarshals JSON properties from a request into a map.
// Returns an error if the properties cannot be parsed.
func ParseProperties(data []byte) (map[string]interface{}, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, fmt.Errorf("failed to parse resource properties: %w", err)
	}
	return props, nil
}

// ValidateNativeID checks that the NativeID is present and not empty.
// Returns an error if validation fails.
func ValidateNativeID(nativeID string) error {
	if nativeID == "" {
		return fmt.Errorf("nativeID is required")
	}
	return nil
}

// MarshalProperties marshals a properties map to a JSON string.
// Returns an error if marshaling fails.
func MarshalProperties(props map[string]interface{}) (string, error) {
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("failed to marshal properties: %w", err)
	}
	return string(propsJSON), nil
}

// NewFailureResult creates a standardized failure ProgressResult.
// This helps reduce boilerplate when creating error responses.
// Note: resourceType parameter kept for backward compatibility but is no longer used in ProgressResult
func NewFailureResult(op resource.Operation, resourceType string, errCode resource.OperationErrorCode, nativeID string) *resource.ProgressResult {
	result := &resource.ProgressResult{
		Operation:       op,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       errCode,
	}
	if nativeID != "" {
		result.NativeID = nativeID
	}
	return result
}

// NewFailureResultWithMessage creates a standardized failure ProgressResult with a status message.
// Use this when you have an error message to include in the result.
func NewFailureResultWithMessage(op resource.Operation, resourceType string, errCode resource.OperationErrorCode, nativeID string, message string) *resource.ProgressResult {
	result := NewFailureResult(op, resourceType, errCode, nativeID)
	result.StatusMessage = message
	return result
}

// ParseTags extracts a string slice from a tags property value.
// Tags come from JSON as []interface{}, so this handles the conversion.
// Returns nil if the input is nil or not a valid tags format.
func ParseTags(v interface{}) []string {
	if v == nil {
		return nil
	}

	// Handle []interface{} (common from JSON unmarshal)
	if arr, ok := v.([]interface{}); ok {
		tags := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				tags = append(tags, s)
			}
		}
		if len(tags) > 0 {
			return tags
		}
		return nil
	}

	// Handle []string directly
	if tags, ok := v.([]string); ok && len(tags) > 0 {
		return tags
	}

	return nil
}

// MapOpenStackErrorToOperationErrorCode maps OpenStack/gophercloud errors to standard operation error codes
func MapOpenStackErrorToOperationErrorCode(err error) resource.OperationErrorCode {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "404"), strings.Contains(errStr, "not found"), strings.Contains(errStr, "NotFound"):
		return resource.OperationErrorCodeNotFound

	case strings.Contains(errStr, "409"), strings.Contains(errStr, "conflict"), strings.Contains(errStr, "already exists"):
		return resource.OperationErrorCodeAlreadyExists

	case strings.Contains(errStr, "401"), strings.Contains(errStr, "unauthorized"), strings.Contains(errStr, "Unauthorized"):
		return resource.OperationErrorCodeAccessDenied

	case strings.Contains(errStr, "403"), strings.Contains(errStr, "forbidden"), strings.Contains(errStr, "Forbidden"):
		return resource.OperationErrorCodeAccessDenied

	case strings.Contains(errStr, "400"), strings.Contains(errStr, "bad request"), strings.Contains(errStr, "BadRequest"):
		return resource.OperationErrorCodeInvalidRequest

	case strings.Contains(errStr, "429"), strings.Contains(errStr, "too many requests"), strings.Contains(errStr, "rate limit"):
		return resource.OperationErrorCodeThrottling

	case strings.Contains(errStr, "500"), strings.Contains(errStr, "internal server error"):
		return resource.OperationErrorCodeGeneralServiceException

	case strings.Contains(errStr, "503"), strings.Contains(errStr, "service unavailable"):
		return resource.OperationErrorCodeGeneralServiceException

	case strings.Contains(errStr, "quota"), strings.Contains(errStr, "Quota"):
		return resource.OperationErrorCodeServiceLimitExceeded

	default:
		return resource.OperationErrorCodeGeneralServiceException
	}
}
