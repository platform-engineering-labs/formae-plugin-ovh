// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient replays canned responses keyed by "METHOD path" and records calls.
type fakeClient struct {
	responses map[string]*ovhtransport.Response
	errors    map[string]error
	calls     []string
}

func (f *fakeClient) Do(_ context.Context, opts ovhtransport.RequestOptions) (*ovhtransport.Response, error) {
	key := opts.Method + " " + opts.Path
	f.calls = append(f.calls, key)
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	if resp, ok := f.responses[key]; ok {
		return resp, nil
	}
	return &ovhtransport.Response{StatusCode: 200}, nil
}

func newTestResource(client TransportClient) *BaseResource {
	return &BaseResource{
		APIConfig: APIConfig{
			PathBuilder: func(ctx PathContext) string {
				path := fmt.Sprintf("/cloud/project/%s/%s", ctx.Project, ctx.ResourceType)
				if ctx.ResourceName != "" {
					path += "/" + ctx.ResourceName
				}
				return path
			},
		},
		ResourceConfig: ResourceConfig{
			ResourceType:   "instance",
			SupportsUpdate: true,
			UpdateMethod:   UpdateMethodPut,
		},
		NativeIDConfig: NativeIDConfig{Format: ProjectHierarchicalFormat},
		Client:         client,
	}
}

func TestUpdate_EmptyPutResponseReReadsResource(t *testing.T) {
	const url = "/cloud/project/proj/instance/i-1"
	client := &fakeClient{responses: map[string]*ovhtransport.Response{
		"GET " + url: {StatusCode: 200, Body: map[string]interface{}{"id": "i-1", "name": "renamed"}},
	}}

	result, err := newTestResource(client).Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "proj/i-1",
		DesiredProperties: json.RawMessage(`{"name":"renamed"}`),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, []string{"PUT " + url, "GET " + url}, client.calls)

	var props map[string]interface{}
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &props))
	assert.Equal(t, "renamed", props["name"])
	assert.Equal(t, "i-1", props["id"])
}

func TestUpdate_NonEmptyPutResponseSkipsReRead(t *testing.T) {
	const url = "/cloud/project/proj/instance/i-1"
	client := &fakeClient{responses: map[string]*ovhtransport.Response{
		"PUT " + url: {StatusCode: 200, Body: map[string]interface{}{"id": "i-1", "name": "renamed"}},
	}}

	result, err := newTestResource(client).Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "proj/i-1",
		DesiredProperties: json.RawMessage(`{"name":"renamed"}`),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, []string{"PUT " + url}, client.calls)
}

func TestUpdate_TransportErrorMessageIncludesContext(t *testing.T) {
	const url = "/cloud/project/proj/instance/i-1"
	client := &fakeClient{errors: map[string]error{
		"PUT " + url: &ovhtransport.Error{Code: ovhtransport.ErrorCodeInvalidInput, Message: "bad body", HTTPCode: 400},
	}}

	result, err := newTestResource(client).Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "proj/i-1",
		DesiredProperties: json.RawMessage(`{"name":"renamed"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus)
	assert.Equal(t, "OVH instance: HTTP 400 INVALID_INPUT: bad body", result.ProgressResult.StatusMessage)
}

func TestFormatTransportError(t *testing.T) {
	assert.Equal(t, "OVH instance: HTTP 404 RESOURCE_NOT_FOUND: gone",
		formatTransportError("instance", &ovhtransport.Error{Code: ovhtransport.ErrorCodeResourceNotFound, Message: "gone", HTTPCode: 404}))
	assert.Equal(t, "OVH volume: UNKNOWN: timeout",
		formatTransportError("volume", &ovhtransport.Error{Code: ovhtransport.ErrorCodeUnknown, Message: "timeout"}))
}
