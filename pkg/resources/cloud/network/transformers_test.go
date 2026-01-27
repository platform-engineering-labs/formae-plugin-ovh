// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateDefaultAllocationRange(t *testing.T) {
	tests := []struct {
		name      string
		cidr      string
		wantStart string
		wantEnd   string
		wantErr   bool
	}{
		{
			name:      "/24 network",
			cidr:      "10.0.3.0/24",
			wantStart: "10.0.3.2",
			wantEnd:   "10.0.3.254",
		},
		{
			name:      "/16 network",
			cidr:      "192.168.0.0/16",
			wantStart: "192.168.0.2",
			wantEnd:   "192.168.255.254",
		},
		{
			name:      "/28 small network",
			cidr:      "10.1.2.0/28",
			wantStart: "10.1.2.2",
			wantEnd:   "10.1.2.14",
		},
		{
			name:    "invalid CIDR",
			cidr:    "invalid",
			wantErr: true,
		},
		{
			name:    "/31 too small",
			cidr:    "10.0.0.0/31",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := calculateDefaultAllocationRange(tt.cidr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestSubnetRequestTransformer(t *testing.T) {
	transformer := &subnetRequestTransformer{}

	input := map[string]interface{}{
		"region":          "DE1",
		"network_id":      "some-network-id", // Should be ignored (not in result)
		"cidr":            "10.0.3.0/24",
		"ipVersion":       4, // Should be ignored (not mapped)
		"enableDhcp":      true,
		"enableGatewayIp": true,
		"name":            "test-subnet", // Should be ignored (not mapped)
	}

	result, err := transformer.Transform(input, base.TransformContext{})
	require.NoError(t, err)

	// Check mapped fields
	assert.Equal(t, "DE1", result["region"])
	assert.Equal(t, "10.0.3.0/24", result["network"])
	assert.Equal(t, "10.0.3.2", result["start"])
	assert.Equal(t, "10.0.3.254", result["end"])
	assert.Equal(t, true, result["dhcp"])
	assert.Equal(t, false, result["noGateway"]) // Inverted from enableGatewayIp=true

	// Check that unmapped fields are NOT included
	assert.NotContains(t, result, "network_id")
	assert.NotContains(t, result, "cidr")
	assert.NotContains(t, result, "enableDhcp")
	assert.NotContains(t, result, "enableGatewayIp")
	assert.NotContains(t, result, "ipVersion")
	assert.NotContains(t, result, "name")
}

func TestSubnetRequestTransformer_NoGateway(t *testing.T) {
	transformer := &subnetRequestTransformer{}

	input := map[string]interface{}{
		"region":          "DE1",
		"cidr":            "10.0.3.0/24",
		"enableDhcp":      false,
		"enableGatewayIp": false, // Should become noGateway=true
	}

	result, err := transformer.Transform(input, base.TransformContext{})
	require.NoError(t, err)

	assert.Equal(t, false, result["dhcp"])
	assert.Equal(t, true, result["noGateway"]) // Inverted from enableGatewayIp=false
}

func TestSubnetPrivateRequestTransformer(t *testing.T) {
	transformer := &subnetPrivateRequestTransformer{}

	input := map[string]interface{}{
		"region":     "DE1",
		"network_id": "some-network-id", // Should be stripped (used in URL path)
		"network":    "10.0.3.0/24",
		"dhcp":       true,
		"noGateway":  false,
		"start":      "10.0.3.2",
		"end":        "10.0.3.254",
	}

	result, err := transformer.Transform(input, base.TransformContext{})
	require.NoError(t, err)

	// Check that API fields are passed through correctly
	assert.Equal(t, "DE1", result["region"])
	assert.Equal(t, "10.0.3.0/24", result["network"])
	assert.Equal(t, true, result["dhcp"])
	assert.Equal(t, false, result["noGateway"])
	assert.Equal(t, "10.0.3.2", result["start"])
	assert.Equal(t, "10.0.3.254", result["end"])

	// Check that network_id is NOT included (used in URL path)
	assert.NotContains(t, result, "network_id")
}

func TestFloatingIPPathBuilder(t *testing.T) {
	tests := []struct {
		name     string
		ctx      base.PathContext
		wantPath string
	}{
		{
			name: "Create - POST with instance_id (ParentResource)",
			ctx: base.PathContext{
				Project:        "my-project",
				Region:         "GRA7",
				ParentResource: "instance-123",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/instance/instance-123/floatingIp",
		},
		{
			name: "List - GET without instance_id",
			ctx: base.PathContext{
				Project: "my-project",
				Region:  "GRA7",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/floatingip",
		},
		{
			name: "Read - GET with floatingip ID",
			ctx: base.PathContext{
				Project:      "my-project",
				Region:       "GRA7",
				ResourceName: "floatingip-456",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/floatingip/floatingip-456",
		},
		{
			name: "Delete - DELETE with floatingip ID",
			ctx: base.PathContext{
				Project:      "my-project",
				Region:       "GRA7",
				ResourceName: "floatingip-789",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/floatingip/floatingip-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath := floatingIPPathBuilder(tt.ctx)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestFloatingIPRequestTransformer(t *testing.T) {
	transformer := &floatingIPRequestTransformer{}

	input := map[string]interface{}{
		"instance_id": "instance-123", // Should be stripped (used in URL path)
		"ip":          "192.168.1.100",
	}

	result, err := transformer.Transform(input, base.TransformContext{})
	require.NoError(t, err)

	// Check that instance_id is NOT included
	assert.NotContains(t, result, "instance_id")

	// Check that other fields are preserved
	assert.Equal(t, "192.168.1.100", result["ip"])
}
