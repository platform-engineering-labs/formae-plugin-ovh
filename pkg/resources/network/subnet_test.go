// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Subnet Test Helpers
// ============================================================================

// createTestNetwork creates a test network for subnet tests
func createTestNetwork(ctx context.Context, t *testing.T, suffix string) *networks.Network {
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-subnet-net-%s-%d", suffix, timestamp)
	adminStateUp := true

	createOpts := networks.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test network for subnet")
	return net
}

// ============================================================================
// Subnet Tests
// ============================================================================

func TestSubnet_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network first
	net := createTestNetwork(ctx, t, "create")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-subnet-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"network_id": "%s",
		"cidr": "10.100.0.0/24",
		"ip_version": 4,
		"enable_dhcp": true,
		"dns_nameservers": ["8.8.8.8", "8.8.4.4"],
		"tags": ["env:test", "managed-by:formae"]
	}`, name, net.ID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeSubnet,
		Label:        "test-subnet",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := subnetProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = subnets.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test subnet: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify subnet was actually created in OpenStack
	subnet, err := subnets.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get subnet from OpenStack")
	assert.Equal(t, name, subnet.Name)
	assert.Equal(t, net.ID, subnet.NetworkID)
	assert.Equal(t, "10.100.0.0/24", subnet.CIDR)
	assert.EqualValues(t, gophercloud.IPv4, subnet.IPVersion)
	assert.True(t, subnet.EnableDHCP)

	t.Logf("✓ Subnet created successfully:")
	t.Logf("  ID: %s", subnet.ID)
	t.Logf("  Name: %s", subnet.Name)
	t.Logf("  CIDR: %s", subnet.CIDR)
	t.Logf("  NetworkID: %s", subnet.NetworkID)
}

func TestSubnet_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network first
	net := createTestNetwork(ctx, t, "read")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create a subnet using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-subnet-read-%d", timestamp)
	enableDHCP := true

	createOpts := subnets.CreateOpts{
		NetworkID:  net.ID,
		Name:       name,
		CIDR:       "10.101.0.0/24",
		IPVersion:  gophercloud.IPv4,
		EnableDHCP: &enableDHCP,
	}
	subnet, err := subnets.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test subnet")
	defer func() {
		_ = subnets.Delete(ctx, networkClient, subnet.ID).ExtractErr()
	}()

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeSubnet,
		NativeID:     subnet.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := subnetProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, subnet.ID, props["id"])
	assert.Equal(t, name, props["name"])
	assert.Equal(t, net.ID, props["network_id"])
	assert.Equal(t, "10.101.0.0/24", props["cidr"])

	t.Logf("✓ Subnet read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  CIDR: %s", props["cidr"])
}

func TestSubnet_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network first
	net := createTestNetwork(ctx, t, "update")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create a subnet using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-subnet-update-initial-%d", timestamp)
	enableDHCP := true

	createOpts := subnets.CreateOpts{
		NetworkID:      net.ID,
		Name:           initialName,
		CIDR:           "10.102.0.0/24",
		IPVersion:      gophercloud.IPv4,
		EnableDHCP:     &enableDHCP,
		DNSNameservers: []string{"8.8.8.8"},
	}
	subnet, err := subnets.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test subnet")
	defer func() {
		_ = subnets.Delete(ctx, networkClient, subnet.ID).ExtractErr()
	}()

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (name, enable_dhcp, dns_nameservers are mutable)
	updatedName := fmt.Sprintf("formae-test-subnet-update-updated-%d", timestamp)
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"network_id": "%s",
		"cidr": "10.102.0.0/24",
		"enable_dhcp": false,
		"dns_nameservers": ["1.1.1.1", "1.0.0.1"],
		"tags": ["env:test", "updated:true"]
	}`, updatedName, net.ID))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeSubnet,
		NativeID:          subnet.ID,
		Label:             "test-subnet",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := subnetProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, subnet.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedSubnet, err := subnets.Get(ctx, networkClient, subnet.ID).Extract()
	require.NoError(t, err, "Should be able to get updated subnet")
	assert.Equal(t, updatedName, updatedSubnet.Name, "Name should be updated")
	assert.False(t, updatedSubnet.EnableDHCP, "EnableDHCP should be false")
	assert.Contains(t, updatedSubnet.DNSNameservers, "1.1.1.1", "DNS should be updated")

	t.Logf("✓ Subnet updated successfully:")
	t.Logf("  ID: %s", updatedSubnet.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedSubnet.Name)
	t.Logf("  EnableDHCP: %v", updatedSubnet.EnableDHCP)
	t.Logf("  DNS: %v", updatedSubnet.DNSNameservers)
}

func TestSubnet_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network first
	net := createTestNetwork(ctx, t, "delete")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create a subnet using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-subnet-delete-%d", timestamp)

	createOpts := subnets.CreateOpts{
		NetworkID: net.ID,
		Name:      name,
		CIDR:      "10.103.0.0/24",
		IPVersion: gophercloud.IPv4,
	}
	subnet, err := subnets.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test subnet")

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSubnet,
		NativeID:     subnet.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := subnetProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify subnet was actually deleted from OpenStack
	_, err = subnets.Get(ctx, networkClient, subnet.ID).Extract()
	assert.Error(t, err, "Subnet should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted subnet")

	t.Logf("✓ Subnet deleted successfully:")
	t.Logf("  ID: %s", subnet.ID)
	t.Logf("  Name: %s", name)
}

func TestSubnet_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent subnet ID
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSubnet,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := subnetProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestSubnet_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network first
	net := createTestNetwork(ctx, t, "list")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create a test subnet
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-subnet-list-%d", timestamp)

	createOpts := subnets.CreateOpts{
		NetworkID: net.ID,
		Name:      name,
		CIDR:      "10.104.0.0/24",
		IPVersion: gophercloud.IPv4,
	}
	subnet, err := subnets.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test subnet")

	// Cleanup after test
	defer func() {
		_ = subnets.Delete(ctx, networkClient, subnet.ID).ExtractErr()
	}()

	// Create Subnet provisioner
	subnetProvisioner := &Subnet{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeSubnet,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := subnetProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test subnet is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == subnet.ID {
			found = true
			t.Logf("✓ Found test subnet in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test subnet in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total subnets found: %d", len(result.NativeIDs))
	t.Logf("  Test subnet found: %v", found)
}
