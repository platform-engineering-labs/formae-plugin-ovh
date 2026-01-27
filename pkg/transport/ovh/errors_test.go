// pkg/transport/ovh/errors_test.go
package ovh

import (
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestClassifyHTTPStatus(t *testing.T) {
	tests := []struct {
		statusCode int
		want       ErrorCode
	}{
		{400, ErrorCodeInvalidInput},
		{401, ErrorCodeUnauthorized},
		{403, ErrorCodeUnauthorized},
		{404, ErrorCodeResourceNotFound},
		{409, ErrorCodeAlreadyExists},
		{429, ErrorCodeThrottling},
		{500, ErrorCodeInternalError},
		{200, ErrorCodeNone},
	}

	for _, tt := range tests {
		got := ClassifyHTTPStatus(tt.statusCode)
		if got != tt.want {
			t.Errorf("ClassifyHTTPStatus(%d) = %v, want %v", tt.statusCode, got, tt.want)
		}
	}
}

func TestToResourceErrorCode(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want resource.OperationErrorCode
	}{
		{ErrorCodeInvalidInput, resource.OperationErrorCodeInvalidRequest},
		{ErrorCodeUnauthorized, resource.OperationErrorCodeAccessDenied},
		{ErrorCodeResourceNotFound, resource.OperationErrorCodeNotFound},
		{ErrorCodeAlreadyExists, resource.OperationErrorCodeAlreadyExists},
	}

	for _, tt := range tests {
		got := ToResourceErrorCode(tt.code)
		if got != tt.want {
			t.Errorf("ToResourceErrorCode(%v) = %v, want %v", tt.code, got, tt.want)
		}
	}
}
