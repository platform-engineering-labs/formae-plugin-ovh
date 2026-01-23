// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testClient *client.Client
)

func TestMain(m *testing.M) {
	if !testutil.IsConfigured() {
		fmt.Println("Skipping storage integration tests: OVH credentials not configured")
		os.Exit(0)
	}

	// Create client using the standard client factory
	var err error
	testClient, err = client.NewClient(testutil.Config)
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Check if Swift is available
	if !testClient.HasSwift() {
		fmt.Println("Skipping storage integration tests: Swift is not available")
		os.Exit(0)
	}

	fmt.Println("Swift storage backend available")

	os.Exit(m.Run())
}

// createTestContainer creates a Swift container directly
func createTestContainer(ctx context.Context, t *testing.T, name string) {
	_, err := containers.Create(ctx, testClient.ObjectStorageClient, name, containers.CreateOpts{}).Extract()
	require.NoError(t, err, "Failed to create test Swift container")
}

// cleanupContainer removes a Swift container
func cleanupContainer(ctx context.Context, name string) {
	_, _ = containers.Delete(ctx, testClient.ObjectStorageClient, name).Extract()
}

// containerExists checks if a Swift container exists
func containerExists(ctx context.Context, name string) bool {
	_, err := containers.Get(ctx, testClient.ObjectStorageClient, name, nil).Extract()
	return err == nil
}

// ============================================================================
// Container Tests
// ============================================================================

func TestContainer_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-create-%d", timestamp)

	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"metadata": {
			"env": "test",
			"managed-by": "formae"
		}
	}`, name))

	req := &resource.CreateRequest{
		ResourceType: ResourceTypeContainer,
		Label:        "test-container",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.Create(ctx, req)

	// Cleanup
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			cleanupContainer(ctx, result.ProgressResult.NativeID)
			t.Logf("Cleaned up test container: %s", result.ProgressResult.NativeID)
		}()
	}

	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, name, result.ProgressResult.NativeID, "NativeID should be the container name")

	t.Logf("Container created successfully:")
	t.Logf("  Name: %s", name)
}

func TestContainer_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-read-%d", timestamp)

	// Create container directly
	createTestContainer(ctx, t, name)
	defer cleanupContainer(ctx, name)

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	req := &resource.ReadRequest{
		ResourceType: ResourceTypeContainer,
		NativeID:     name,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.Read(ctx, req)

	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")
	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")
	assert.Equal(t, name, props["name"])

	t.Logf("Container read successfully:")
	t.Logf("  Name: %s", props["name"])
}

func TestContainer_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-update-%d", timestamp)

	// Create container directly
	createTestContainer(ctx, t, name)
	defer cleanupContainer(ctx, name)

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"metadata": {
			"env": "test",
			"status": "updated",
			"new-key": "new-value"
		}
	}`, name))

	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeContainer,
		NativeID:          name,
		Label:             "test-container",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	result, err := containerProvisioner.Update(ctx, req)

	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, name, result.ProgressResult.NativeID, "NativeID should not change")

	t.Logf("Container updated successfully:")
	t.Logf("  Name: %s", name)
}

func TestContainer_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-delete-%d", timestamp)

	// Create container directly (no defer cleanup - we're testing delete)
	createTestContainer(ctx, t, name)

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeContainer,
		NativeID:     name,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.Delete(ctx, req)

	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify container was actually deleted
	assert.False(t, containerExists(ctx, name), "Container should not exist after deletion")

	t.Logf("Container deleted successfully:")
	t.Logf("  Name: %s", name)
}

func TestContainer_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	nonExistentName := "formae-test-container-nonexistent-00000000"

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeContainer,
		NativeID:     nonExistentName,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.Delete(ctx, req)

	// Deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	t.Logf("Idempotent delete test passed (resource already gone)")
}

func TestContainer_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-list-%d", timestamp)

	// Create container directly
	createTestContainer(ctx, t, name)
	defer cleanupContainer(ctx, name)

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	req := &resource.ListRequest{
		ResourceType: ResourceTypeContainer,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.List(ctx, req)

	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	// Verify our test container is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == name {
			found = true
			t.Logf("Found test container in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test container in the list")

	t.Logf("List operation successful:")
	t.Logf("  Total containers found: %d", len(result.NativeIDs))
	t.Logf("  Test container found: %v", found)
}

func TestContainer_Create_WithMetadata_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	containerProvisioner := &Container{
		Client: testClient,
		Config: testClient.Config,
	}

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-container-metadata-%d", timestamp)

	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"metadata": {
			"env": "test",
			"managed-by": "formae",
			"purpose": "integration-test"
		}
	}`, name))

	req := &resource.CreateRequest{
		ResourceType: ResourceTypeContainer,
		Label:        "test-container-metadata",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	result, err := containerProvisioner.Create(ctx, req)

	// Cleanup
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			cleanupContainer(ctx, result.ProgressResult.NativeID)
			t.Logf("Cleaned up test container: %s", result.ProgressResult.NativeID)
		}()
	}

	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	t.Logf("Container created with metadata successfully:")
	t.Logf("  Name: %s", name)
}
