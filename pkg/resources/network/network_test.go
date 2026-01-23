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

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Network Tests
// ============================================================================

func TestNetwork_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-network-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test network for integration tests",
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae"]
	}`, name))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeNetwork,
		Label:        "test-network",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := networkProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = networks.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test network: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify network was actually created in OpenStack
	net, err := networks.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get network from OpenStack")
	assert.Equal(t, name, net.Name)
	assert.True(t, net.AdminStateUp)

	t.Logf("✓ Network created successfully:")
	t.Logf("  ID: %s", net.ID)
	t.Logf("  Name: %s", net.Name)
	t.Logf("  AdminStateUp: %v", net.AdminStateUp)
}

func TestNetwork_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a network using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-network-read-%d", timestamp)
	adminStateUp := true

	createOpts := networks.CreateOpts{
		Name:         name,
		Description:  "Test network for read test",
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test network")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeNetwork,
		NativeID:     net.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := networkProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, net.ID, props["id"])
	assert.Equal(t, name, props["name"])
	assert.Equal(t, true, props["admin_state_up"])

	t.Logf("✓ Network read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
}

func TestNetwork_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a network using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-network-update-initial-%d", timestamp)
	adminStateUp := true

	createOpts := networks.CreateOpts{
		Name:         initialName,
		Description:  "Initial description",
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test network")
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (name and admin_state_up are mutable)
	updatedName := fmt.Sprintf("formae-test-network-update-updated-%d", timestamp)
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"admin_state_up": false,
		"tags": ["env:test", "updated:true"]
	}`, updatedName))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeNetwork,
		NativeID:          net.ID,
		Label:             "test-network",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := networkProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, net.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedNet, err := networks.Get(ctx, networkClient, net.ID).Extract()
	require.NoError(t, err, "Should be able to get updated network")
	assert.Equal(t, updatedName, updatedNet.Name, "Name should be updated")
	assert.False(t, updatedNet.AdminStateUp, "AdminStateUp should be false")

	t.Logf("✓ Network updated successfully:")
	t.Logf("  ID: %s", updatedNet.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedNet.Name)
	t.Logf("  AdminStateUp: %v", updatedNet.AdminStateUp)
}

func TestNetwork_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a network using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-network-delete-%d", timestamp)
	adminStateUp := true

	createOpts := networks.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test network")

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeNetwork,
		NativeID:     net.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := networkProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify network was actually deleted from OpenStack
	_, err = networks.Get(ctx, networkClient, net.ID).Extract()
	assert.Error(t, err, "Network should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted network")

	t.Logf("✓ Network deleted successfully:")
	t.Logf("  ID: %s", net.ID)
	t.Logf("  Name: %s", name)
}

func TestNetwork_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent network ID
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeNetwork,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := networkProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestNetwork_Read_WithTags_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-network-read-tags-%d", timestamp)
	expectedTags := []string{"env:test", "managed-by:formae", "test:read-tags"}

	// Prepare resource properties with tags
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test network for read tags integration test",
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae", "test:read-tags"]
	}`, name))

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: ResourceTypeNetwork,
		Label:        "test-network-tags",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	createResult, err := networkProvisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "ProgressResult should not be nil")
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Create should succeed")

	networkID := createResult.ProgressResult.NativeID

	// Cleanup after test
	defer func() {
		_ = networks.Delete(ctx, networkClient, networkID).ExtractErr()
		t.Logf("Cleaned up test network: %s", networkID)
	}()

	// Now read the network back and verify tags are returned
	readReq := &resource.ReadRequest{
		ResourceType: ResourceTypeNetwork,
		NativeID:     networkID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := networkProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Parse and verify properties including tags
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, networkID, props["id"])
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

	t.Logf("Network read with tags successful:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Tags: %v", actualTags)
}

func TestNetwork_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test network
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-network-list-%d", timestamp)
	adminStateUp := true

	createOpts := networks.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	net, err := networks.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test network")

	// Cleanup after test
	defer func() {
		_ = networks.Delete(ctx, networkClient, net.ID).ExtractErr()
	}()

	// Create Network provisioner
	networkProvisioner := &Network{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeNetwork,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := networkProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test network is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == net.ID {
			found = true
			t.Logf("✓ Found test network in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test network in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total networks found: %d", len(result.NativeIDs))
	t.Logf("  Test network found: %v", found)
}
