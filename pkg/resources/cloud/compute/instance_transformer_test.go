// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceRequestTransformer_UpdateSendsOnlyInstanceName(t *testing.T) {
	props := map[string]interface{}{
		"name":     "renamed",
		"flavorId": "flavor-1",
		"imageId":  "image-1",
		"region":   "DE1",
	}
	body, err := instanceRequestTransformer.Transform(props, base.TransformContext{Operation: resource.OperationUpdate})
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"instanceName": "renamed"}, body)
}

func TestInstanceRequestTransformer_CreatePassesThrough(t *testing.T) {
	props := map[string]interface{}{"name": "vm", "flavorId": "flavor-1"}
	body, err := instanceRequestTransformer.Transform(props, base.TransformContext{Operation: resource.OperationCreate})
	require.NoError(t, err)
	assert.Equal(t, props, body)
}

func TestInstanceStatusChecker(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		wantReady bool
		wantErr   bool
	}{
		{name: "building", data: map[string]interface{}{"status": "BUILD"}},
		{name: "no status", data: map[string]interface{}{}},
		{name: "active without IPs yet", data: map[string]interface{}{"status": "ACTIVE", "ipAddresses": []interface{}{}}},
		{name: "active with IPs", data: map[string]interface{}{"status": "ACTIVE", "ipAddresses": []interface{}{map[string]interface{}{"ip": "10.0.1.5"}}}, wantReady: true},
		{name: "error is terminal", data: map[string]interface{}{"status": "ERROR", "name": "vm", "id": "i-1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, err := instanceStatusChecker(tt.data)
			assert.Equal(t, tt.wantReady, ready)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "i-1")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
