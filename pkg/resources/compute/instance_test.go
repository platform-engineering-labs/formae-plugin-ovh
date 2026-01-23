// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Instance Test Helpers
// ============================================================================

// skipIfInstanceTestNotConfigured skips the test if instance test requirements aren't met
func skipIfInstanceTestNotConfigured(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	if testutil.TestImageID == "" {
		t.Skip("Skipping instance test: OS_TEST_IMAGE_ID not set")
	}
	if testutil.TestNetworkID == "" {
		t.Skip("Skipping instance test: OS_TEST_NETWORK_ID not set")
	}
}

// waitForInstanceStatus polls the Status endpoint until the operation completes or times out
func waitForInstanceStatus(ctx context.Context, provisioner *Instance, requestID string, timeout time.Duration) (*resource.StatusResult, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := provisioner.Status(ctx, &resource.StatusRequest{
			RequestID: requestID,
		})
		if err != nil {
			return result, err
		}

		// Check if operation completed
		if result.ProgressResult.OperationStatus == resource.OperationStatusSuccess ||
			result.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			return result, nil
		}

		// Still in progress, wait and retry
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for instance operation to complete")
}

// ============================================================================
// Instance Tests
// ============================================================================

func TestInstance_Create_Integration(t *testing.T) {
	skipIfInstanceTestNotConfigured(t)
	ctx := context.Background()

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-instance-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"flavor_id": "%s",
		"image_id": "%s",
		"networks": [{"uuid": "%s"}],
		"metadata": {
			"env": "test",
			"managed-by": "formae"
		}
	}`, name, testutil.TestFlavorID, testutil.TestImageID, testutil.TestNetworkID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeInstance,
		Label:        "test-instance",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := instanceProvisioner.Create(ctx, req)

	// Assert initial response (should be InProgress)
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")
	assert.NotEmpty(t, result.ProgressResult.RequestID, "RequestID should be set for async operation")

	instanceID := result.ProgressResult.NativeID
	t.Logf("Instance creation started: %s", instanceID)

	// Cleanup after test
	defer func() {
		_ = servers.Delete(ctx, computeClient, instanceID).ExtractErr()
		// Wait for deletion to complete
		for i := 0; i < 30; i++ {
			_, err := servers.Get(ctx, computeClient, instanceID).Extract()
			if err != nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
		t.Logf("✓ Cleaned up test instance: %s", instanceID)
	}()

	// Poll for completion
	statusResult, err := waitForInstanceStatus(ctx, instanceProvisioner, instanceID, 5*time.Minute)
	require.NoError(t, err, "Should complete without timeout")
	require.NotNil(t, statusResult, "Status result should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
	assert.Equal(t, instanceID, statusResult.ProgressResult.NativeID)

	// Verify instance was actually created in OpenStack
	server, err := servers.Get(ctx, computeClient, instanceID).Extract()
	require.NoError(t, err, "Should be able to get instance from OpenStack")
	assert.Equal(t, name, server.Name)
	assert.Equal(t, "ACTIVE", server.Status)

	t.Logf("✓ Instance created successfully:")
	t.Logf("  ID: %s", server.ID)
	t.Logf("  Name: %s", server.Name)
	t.Logf("  Status: %s", server.Status)
}

func TestInstance_Read_Integration(t *testing.T) {
	skipIfInstanceTestNotConfigured(t)
	ctx := context.Background()

	// First, create an instance using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-instance-read-%d", timestamp)

	createOpts := servers.CreateOpts{
		Name:      name,
		FlavorRef: testutil.TestFlavorID,
		ImageRef:  testutil.TestImageID,
		Networks: []servers.Network{
			{UUID: testutil.TestNetworkID},
		},
		Metadata: map[string]string{
			"env": "test",
		},
	}
	server, err := servers.Create(ctx, computeClient, createOpts, nil).Extract()
	require.NoError(t, err, "Failed to create test instance")

	// Cleanup after test
	defer func() {
		_ = servers.Delete(ctx, computeClient, server.ID).ExtractErr()
		for i := 0; i < 30; i++ {
			_, err := servers.Get(ctx, computeClient, server.ID).Extract()
			if err != nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Wait for instance to become ACTIVE
	for i := 0; i < 60; i++ {
		server, err = servers.Get(ctx, computeClient, server.ID).Extract()
		require.NoError(t, err)
		if server.Status == "ACTIVE" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.Equal(t, "ACTIVE", server.Status, "Instance should be ACTIVE")

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeInstance,
		NativeID:     server.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := instanceProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, server.ID, props["id"])
	assert.Equal(t, name, props["name"])

	t.Logf("✓ Instance read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
}

func TestInstance_Update_Integration(t *testing.T) {
	skipIfInstanceTestNotConfigured(t)
	ctx := context.Background()

	// First, create an instance using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-instance-update-initial-%d", timestamp)

	createOpts := servers.CreateOpts{
		Name:      initialName,
		FlavorRef: testutil.TestFlavorID,
		ImageRef:  testutil.TestImageID,
		Networks: []servers.Network{
			{UUID: testutil.TestNetworkID},
		},
		Metadata: map[string]string{
			"env":    "test",
			"status": "initial",
		},
	}
	server, err := servers.Create(ctx, computeClient, createOpts, nil).Extract()
	require.NoError(t, err, "Failed to create test instance")

	// Cleanup after test
	defer func() {
		_ = servers.Delete(ctx, computeClient, server.ID).ExtractErr()
		for i := 0; i < 30; i++ {
			_, err := servers.Get(ctx, computeClient, server.ID).Extract()
			if err != nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Wait for instance to become ACTIVE
	for i := 0; i < 60; i++ {
		server, err = servers.Get(ctx, computeClient, server.ID).Extract()
		require.NoError(t, err)
		if server.Status == "ACTIVE" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.Equal(t, "ACTIVE", server.Status, "Instance should be ACTIVE")

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (name and metadata are mutable)
	updatedName := fmt.Sprintf("formae-test-instance-update-updated-%d", timestamp)
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"flavor_id": "%s",
		"image_id": "%s",
		"metadata": {
			"env": "test",
			"status": "updated"
		}
	}`, updatedName, testutil.TestFlavorID, testutil.TestImageID))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeInstance,
		NativeID:          server.ID,
		Label:             "test-instance",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := instanceProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, server.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedServer, err := servers.Get(ctx, computeClient, server.ID).Extract()
	require.NoError(t, err, "Should be able to get updated instance")
	assert.Equal(t, updatedName, updatedServer.Name, "Name should be updated")
	assert.Equal(t, "updated", updatedServer.Metadata["status"], "Metadata should be updated")

	t.Logf("✓ Instance updated successfully:")
	t.Logf("  ID: %s", updatedServer.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedServer.Name)
	t.Logf("  Metadata status: %s", updatedServer.Metadata["status"])
}

func TestInstance_Delete_Integration(t *testing.T) {
	skipIfInstanceTestNotConfigured(t)
	ctx := context.Background()

	// First, create an instance using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-instance-delete-%d", timestamp)

	createOpts := servers.CreateOpts{
		Name:      name,
		FlavorRef: testutil.TestFlavorID,
		ImageRef:  testutil.TestImageID,
		Networks: []servers.Network{
			{UUID: testutil.TestNetworkID},
		},
	}
	server, err := servers.Create(ctx, computeClient, createOpts, nil).Extract()
	require.NoError(t, err, "Failed to create test instance")

	// Wait for instance to become ACTIVE
	for i := 0; i < 60; i++ {
		server, err = servers.Get(ctx, computeClient, server.ID).Extract()
		require.NoError(t, err)
		if server.Status == "ACTIVE" {
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeInstance,
		NativeID:     server.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := instanceProvisioner.Delete(ctx, req)

	// Assert initial response
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)

	// Delete may be InProgress or Success depending on timing
	if result.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		// Poll for completion
		statusResult, err := waitForInstanceStatus(ctx, instanceProvisioner, server.ID, 2*time.Minute)
		require.NoError(t, err, "Should complete without timeout")
		assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
	} else {
		assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	}

	// Verify instance was actually deleted from OpenStack
	_, err = servers.Get(ctx, computeClient, server.ID).Extract()
	assert.Error(t, err, "Instance should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted instance")

	t.Logf("✓ Instance deleted successfully:")
	t.Logf("  ID: %s", server.ID)
	t.Logf("  Name: %s", name)
}

func TestInstance_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent instance ID (valid UUID format)
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeInstance,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := instanceProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestInstance_List_Integration(t *testing.T) {
	skipIfInstanceTestNotConfigured(t)
	ctx := context.Background()

	// Create a test instance
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-instance-list-%d", timestamp)

	createOpts := servers.CreateOpts{
		Name:      name,
		FlavorRef: testutil.TestFlavorID,
		ImageRef:  testutil.TestImageID,
		Networks: []servers.Network{
			{UUID: testutil.TestNetworkID},
		},
	}
	server, err := servers.Create(ctx, computeClient, createOpts, nil).Extract()
	require.NoError(t, err, "Failed to create test instance")

	// Cleanup after test
	defer func() {
		_ = servers.Delete(ctx, computeClient, server.ID).ExtractErr()
		for i := 0; i < 30; i++ {
			_, err := servers.Get(ctx, computeClient, server.ID).Extract()
			if err != nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Wait for instance to be visible (may take a moment)
	time.Sleep(2 * time.Second)

	// Create Instance provisioner
	instanceProvisioner := &Instance{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeInstance,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := instanceProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test instance is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == server.ID {
			found = true
			t.Logf("✓ Found test instance in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test instance in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total instances found: %d", len(result.NativeIDs))
	t.Logf("  Test instance found: %v", found)
}
