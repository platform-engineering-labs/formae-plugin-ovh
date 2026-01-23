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
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Port Test Helpers
// ============================================================================

// createTestNetworkAndSubnet creates a test network and subnet for port tests
func createTestNetworkAndSubnet(ctx context.Context, t *testing.T, suffix string) (*networks.Network, *subnets.Subnet) {
	timestamp := time.Now().Unix()
	netName := fmt.Sprintf("formae-test-port-net-%s-%d", suffix, timestamp)
	adminStateUp := true

	netCreateOpts := networks.CreateOpts{
		Name:         netName,
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, netCreateOpts).Extract()
	require.NoError(t, err, "Failed to create test network for port")

	subnetName := fmt.Sprintf("formae-test-port-subnet-%s-%d", suffix, timestamp)
	enableDHCP := true
	subnetCreateOpts := subnets.CreateOpts{
		NetworkID:  net.ID,
		Name:       subnetName,
		CIDR:       fmt.Sprintf("10.%d.0.0/24", timestamp%255),
		IPVersion:  gophercloud.IPv4,
		EnableDHCP: &enableDHCP,
	}
	subnet, err := subnets.Create(ctx, networkClient, subnetCreateOpts).Extract()
	require.NoError(t, err, "Failed to create test subnet for port")

	return net, subnet
}

// cleanupTestNetworkAndSubnet cleans up test network and subnet
func cleanupTestNetworkAndSubnet(ctx context.Context, net *networks.Network, subnet *subnets.Subnet) {
	if subnet != nil {
		_ = subnets.Delete(ctx, networkClient, subnet.ID).ExtractErr()
	}
	if net != nil {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}
}

// ============================================================================
// Port Tests
// ============================================================================

func TestPort_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "create")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-port-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test port for integration tests",
		"network_id": "%s",
		"fixed_ips": [{"subnet_id": "%s"}],
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae"]
	}`, name, net.ID, subnet.ID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypePort,
		Label:        "test-port",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := portProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = ports.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test port: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify port was actually created in OpenStack
	port, err := ports.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get port from OpenStack")
	assert.Equal(t, name, port.Name)
	assert.Equal(t, net.ID, port.NetworkID)
	assert.True(t, port.AdminStateUp)
	assert.NotEmpty(t, port.FixedIPs, "FixedIPs should be assigned")

	t.Logf("✓ Port created successfully:")
	t.Logf("  ID: %s", port.ID)
	t.Logf("  Name: %s", port.Name)
	t.Logf("  NetworkID: %s", port.NetworkID)
	t.Logf("  MACAddress: %s", port.MACAddress)
	if len(port.FixedIPs) > 0 {
		t.Logf("  FixedIP: %s", port.FixedIPs[0].IPAddress)
	}
}

func TestPort_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "read")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create a port using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-port-read-%d", timestamp)
	adminStateUp := true

	createOpts := ports.CreateOpts{
		NetworkID:    net.ID,
		Name:         name,
		Description:  "Test port for read test",
		AdminStateUp: &adminStateUp,
		FixedIPs: []ports.IP{
			{SubnetID: subnet.ID},
		},
	}
	port, err := ports.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test port")
	defer func() {
		_ = ports.Delete(ctx, networkClient, port.ID).ExtractErr()
	}()

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypePort,
		NativeID:     port.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := portProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, port.ID, props["id"])
	assert.Equal(t, name, props["name"])
	assert.Equal(t, net.ID, props["network_id"])
	assert.NotEmpty(t, props["mac_address"])

	t.Logf("✓ Port read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  MACAddress: %s", props["mac_address"])
}

func TestPort_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "update")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create a port using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-port-update-initial-%d", timestamp)
	adminStateUp := true

	createOpts := ports.CreateOpts{
		NetworkID:    net.ID,
		Name:         initialName,
		AdminStateUp: &adminStateUp,
		FixedIPs: []ports.IP{
			{SubnetID: subnet.ID},
		},
	}
	port, err := ports.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test port")
	defer func() {
		_ = ports.Delete(ctx, networkClient, port.ID).ExtractErr()
	}()

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (name and admin_state_up are mutable)
	updatedName := fmt.Sprintf("formae-test-port-update-updated-%d", timestamp)
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"network_id": "%s",
		"admin_state_up": false,
		"tags": ["env:test", "updated:true"]
	}`, updatedName, net.ID))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypePort,
		NativeID:          port.ID,
		Label:             "test-port",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := portProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, port.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedPort, err := ports.Get(ctx, networkClient, port.ID).Extract()
	require.NoError(t, err, "Should be able to get updated port")
	assert.Equal(t, updatedName, updatedPort.Name, "Name should be updated")
	assert.False(t, updatedPort.AdminStateUp, "AdminStateUp should be false")

	t.Logf("✓ Port updated successfully:")
	t.Logf("  ID: %s", updatedPort.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedPort.Name)
	t.Logf("  AdminStateUp: %v", updatedPort.AdminStateUp)
}

func TestPort_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "delete")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create a port using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-port-delete-%d", timestamp)
	adminStateUp := true

	createOpts := ports.CreateOpts{
		NetworkID:    net.ID,
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	port, err := ports.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test port")

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypePort,
		NativeID:     port.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := portProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify port was actually deleted from OpenStack
	_, err = ports.Get(ctx, networkClient, port.ID).Extract()
	assert.Error(t, err, "Port should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted port")

	t.Logf("✓ Port deleted successfully:")
	t.Logf("  ID: %s", port.ID)
	t.Logf("  Name: %s", name)
}

func TestPort_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent port ID
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypePort,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := portProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestPort_Read_WithTags_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "read-tags")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-port-read-tags-%d", timestamp)
	expectedTags := []string{"env:test", "managed-by:formae", "test:read-tags"}

	// Prepare resource properties with tags
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test port for read tags integration test",
		"network_id": "%s",
		"fixed_ips": [{"subnet_id": "%s"}],
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae", "test:read-tags"]
	}`, name, net.ID, subnet.ID))

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: ResourceTypePort,
		Label:        "test-port-tags",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	createResult, err := portProvisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "ProgressResult should not be nil")
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Create should succeed")

	portID := createResult.ProgressResult.NativeID

	// Cleanup after test
	defer func() {
		_ = ports.Delete(ctx, networkClient, portID).ExtractErr()
		t.Logf("Cleaned up test port: %s", portID)
	}()

	// Now read the port back and verify tags are returned
	readReq := &resource.ReadRequest{
		ResourceType: ResourceTypePort,
		NativeID:     portID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := portProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Parse and verify properties including tags
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, portID, props["id"])
	assert.Equal(t, name, props["name"])

	// Verify tags are present in the read result
	tagsRaw, ok := props["tags"]
	require.True(t, ok, "Tags should be present in read result")

	tagsSlice, ok := tagsRaw.([]interface{})
	require.True(t, ok, "Tags should be a slice")
	require.Len(t, tagsSlice, len(expectedTags), "Should have correct number of tags")

	// Convert to string slice for comparison
	actualTags := make([]string, len(tagsSlice))
	for i, tag := range tagsSlice {
		actualTags[i] = tag.(string)
	}

	// Verify all expected tags are present (order may vary)
	for _, expectedTag := range expectedTags {
		assert.Contains(t, actualTags, expectedTag, "Should contain tag: %s", expectedTag)
	}

	t.Logf("Port read with tags successful:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Tags: %v", actualTags)
}

func TestPort_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network and subnet first
	net, subnet := createTestNetworkAndSubnet(ctx, t, "list")
	defer cleanupTestNetworkAndSubnet(ctx, net, subnet)

	// Create a test port
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-port-list-%d", timestamp)
	adminStateUp := true

	createOpts := ports.CreateOpts{
		NetworkID:    net.ID,
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	port, err := ports.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test port")

	// Cleanup after test
	defer func() {
		_ = ports.Delete(ctx, networkClient, port.ID).ExtractErr()
	}()

	// Create Port provisioner
	portProvisioner := &Port{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypePort,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := portProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	// Note: List filters out ports attached to devices, so our standalone port should be included
	// Verify our test port is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == port.ID {
			found = true
			t.Logf("✓ Found test port in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test port in the list (standalone ports only)")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total standalone ports found: %d", len(result.NativeIDs))
	t.Logf("  Test port found: %v", found)
}
