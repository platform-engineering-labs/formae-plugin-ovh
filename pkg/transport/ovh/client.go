// pkg/transport/ovh/client.go
package ovh

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ovh/go-ovh/ovh"
)

// Client wraps go-ovh for the REST architecture
type Client struct {
	ovh *ovh.Client
}

// RequestOptions defines options for an API request
type RequestOptions struct {
	Method string
	Path   string
	Body   interface{} // Can be map[string]interface{} or []interface{} for array bodies
}

// Response represents an API response
type Response struct {
	StatusCode int
	Body       map[string]interface{}
	BodyArray  []interface{}
}

// OVHConfig holds OVH REST API credentials
type OVHConfig struct {
	Endpoint          string
	ApplicationKey    string
	ApplicationSecret string
	ConsumerKey       string
}

// NewClient creates a new OVH API client from config
func NewClient(cfg *OVHConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "ovh-eu" // default
	}

	ovhClient, err := ovh.NewClient(endpoint, cfg.ApplicationKey, cfg.ApplicationSecret, cfg.ConsumerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH client: %w", err)
	}
	return &Client{ovh: ovhClient}, nil
}

// Do executes an API request
func (c *Client) Do(ctx context.Context, opts RequestOptions) (*Response, error) {
	var result json.RawMessage
	var err error

	switch opts.Method {
	case "GET":
		err = c.ovh.GetWithContext(ctx, opts.Path, &result)
	case "POST":
		err = c.ovh.PostWithContext(ctx, opts.Path, opts.Body, &result)
	case "PUT":
		err = c.ovh.PutWithContext(ctx, opts.Path, opts.Body, &result)
	case "DELETE":
		err = c.ovh.DeleteWithContext(ctx, opts.Path, &result)
	default:
		return nil, fmt.Errorf("unsupported method: %s", opts.Method)
	}

	if err != nil {
		return nil, c.classifyError(err)
	}

	return c.parseResponse(result)
}

// parseResponse converts raw JSON to Response
func (c *Client) parseResponse(raw json.RawMessage) (*Response, error) {
	if len(raw) == 0 {
		return &Response{StatusCode: 200}, nil
	}

	resp := &Response{StatusCode: 200}

	// Try to parse as object first
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		resp.Body = obj
		return resp, nil
	}

	// Try to parse as array
	var arr []interface{}
	if err := json.Unmarshal(raw, &arr); err == nil {
		resp.BodyArray = arr
		return resp, nil
	}

	return nil, fmt.Errorf("failed to parse response: %s", string(raw))
}

// classifyError converts OVH errors to transport errors
func (c *Client) classifyError(err error) error {
	if err == nil {
		return nil
	}

	// go-ovh returns APIError for HTTP errors
	if apiErr, ok := err.(*ovh.APIError); ok {
		code := ClassifyHTTPStatus(apiErr.Code)
		return &Error{
			Code:       code,
			Message:    apiErr.Message,
			HTTPCode:   apiErr.Code,
			Underlying: err,
		}
	}

	return &Error{
		Code:       ErrorCodeUnknown,
		Message:    err.Error(),
		Underlying: err,
	}
}
