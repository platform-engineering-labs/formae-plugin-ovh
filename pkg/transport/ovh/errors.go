// pkg/transport/ovh/errors.go
package ovh

import (
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ErrorCode represents transport-level error classifications
type ErrorCode string

const (
	ErrorCodeNone              ErrorCode = "NONE"
	ErrorCodeInvalidInput      ErrorCode = "INVALID_INPUT"
	ErrorCodeUnauthorized      ErrorCode = "UNAUTHORIZED"
	ErrorCodeResourceNotFound  ErrorCode = "RESOURCE_NOT_FOUND"
	ErrorCodeAlreadyExists     ErrorCode = "ALREADY_EXISTS"
	ErrorCodeThrottling        ErrorCode = "THROTTLING"
	ErrorCodeInternalError     ErrorCode = "INTERNAL_ERROR"
	ErrorCodeUnknown           ErrorCode = "UNKNOWN"
)

// Error represents a transport layer error with classification
type Error struct {
	Code       ErrorCode
	Message    string
	HTTPCode   int
	Underlying error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Underlying
}

// ClassifyHTTPStatus maps HTTP status codes to error codes
func ClassifyHTTPStatus(statusCode int) ErrorCode {
	switch statusCode {
	case 200, 201, 204:
		return ErrorCodeNone
	case 400:
		return ErrorCodeInvalidInput
	case 401, 403:
		return ErrorCodeUnauthorized
	case 404:
		return ErrorCodeResourceNotFound
	case 409:
		return ErrorCodeAlreadyExists
	case 429:
		return ErrorCodeThrottling
	case 500, 502, 503:
		return ErrorCodeInternalError
	default:
		if statusCode >= 200 && statusCode < 300 {
			return ErrorCodeNone
		}
		return ErrorCodeUnknown
	}
}

// ToResourceErrorCode converts transport error code to formae resource error code
func ToResourceErrorCode(code ErrorCode) resource.OperationErrorCode {
	switch code {
	case ErrorCodeInvalidInput:
		return resource.OperationErrorCodeInvalidRequest
	case ErrorCodeUnauthorized:
		return resource.OperationErrorCodeAccessDenied
	case ErrorCodeResourceNotFound:
		return resource.OperationErrorCodeNotFound
	case ErrorCodeAlreadyExists:
		return resource.OperationErrorCodeAlreadyExists
	case ErrorCodeThrottling:
		return resource.OperationErrorCodeThrottling
	case ErrorCodeInternalError:
		return resource.OperationErrorCodeServiceInternalError
	default:
		return resource.OperationErrorCodeServiceInternalError
	}
}

// NewError creates a new transport error
func NewError(code ErrorCode, message string, underlying error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		Underlying: underlying,
	}
}
