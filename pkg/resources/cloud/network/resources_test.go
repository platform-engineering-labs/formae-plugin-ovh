// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"context"
	"errors"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateNetworkStatusChecker(t *testing.T) {
	activeRegion := map[string]interface{}{"region": "DE1", "status": "ACTIVE", "openstackId": "os-1"}
	tests := []struct {
		name      string
		data      map[string]interface{}
		wantReady bool
		wantErr   bool
	}{
		{name: "top-level building", data: map[string]interface{}{"status": "BUILDING", "regions": []interface{}{activeRegion}}},
		{name: "top-level error is terminal", data: map[string]interface{}{"id": "pn-1", "status": "ERROR"}, wantErr: true},
		{name: "region not active", data: map[string]interface{}{"status": "ACTIVE", "regions": []interface{}{
			map[string]interface{}{"region": "DE1", "status": "BUILDING", "openstackId": "os-1"},
		}}},
		{name: "region missing openstackId", data: map[string]interface{}{"status": "ACTIVE", "regions": []interface{}{
			map[string]interface{}{"region": "DE1", "status": "ACTIVE"},
		}}},
		{name: "fully provisioned", data: map[string]interface{}{"status": "ACTIVE", "regions": []interface{}{activeRegion}}, wantReady: true},
		{name: "no regions field", data: map[string]interface{}{"status": "ACTIVE"}, wantReady: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, err := privateNetworkStatusChecker(tt.data)
			assert.Equal(t, tt.wantReady, ready)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type probeClient struct {
	network map[string]interface{}
	failing map[string]bool
	calls   []string
}

func (c *probeClient) Do(_ context.Context, opts ovhtransport.RequestOptions) (*ovhtransport.Response, error) {
	c.calls = append(c.calls, opts.Path)
	if c.failing[opts.Path] {
		return nil, errors.New("not found")
	}
	if opts.Path == "/cloud/project/proj/network/private/pn-1" {
		return &ovhtransport.Response{StatusCode: 200, Body: c.network}, nil
	}
	return &ovhtransport.Response{StatusCode: 200}, nil
}

func TestPrivateNetworkReadinessProbe(t *testing.T) {
	const (
		subnetURL   = "/cloud/project/proj/network/private/pn-1/subnet"
		regionalURL = "/cloud/project/proj/region/DE1/network/os-1"
	)
	pathCtx := base.PathContext{Project: "proj", ResourceName: "pn-1"}
	network := map[string]interface{}{"regions": []interface{}{
		map[string]interface{}{"region": "DE1", "status": "ACTIVE", "openstackId": "os-1"},
	}}

	t.Run("ready when subnet and regional endpoints answer", func(t *testing.T) {
		client := &probeClient{network: network}
		ready, err := privateNetworkReadinessProbe(context.Background(), client, pathCtx)
		require.NoError(t, err)
		assert.True(t, ready)
		assert.Contains(t, client.calls, regionalURL)
	})

	t.Run("not ready when subnet endpoint fails", func(t *testing.T) {
		client := &probeClient{network: network, failing: map[string]bool{subnetURL: true}}
		ready, err := privateNetworkReadinessProbe(context.Background(), client, pathCtx)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("not ready when regional endpoint fails", func(t *testing.T) {
		client := &probeClient{network: network, failing: map[string]bool{regionalURL: true}}
		ready, err := privateNetworkReadinessProbe(context.Background(), client, pathCtx)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("not ready before openstackId is assigned", func(t *testing.T) {
		client := &probeClient{network: map[string]interface{}{"regions": []interface{}{
			map[string]interface{}{"region": "DE1", "status": "ACTIVE"},
		}}}
		ready, err := privateNetworkReadinessProbe(context.Background(), client, pathCtx)
		require.NoError(t, err)
		assert.False(t, ready)
		assert.NotContains(t, client.calls, regionalURL)
	})
}
