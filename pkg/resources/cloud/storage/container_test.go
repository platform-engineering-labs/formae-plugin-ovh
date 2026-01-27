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

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SWIFT storage uses 2-letter region codes (DE, GRA, BHS, etc.)
// This differs from OpenStack compute regions (DE1, GRA9, BHS5, etc.)
const testSwiftRegion = "DE"

var (
	testOVHClient            *ovhtransport.Client
	testContainerProvisioner prov.Provisioner
	testTargetConfig         json.RawMessage
)

func TestMain(m *testing.M) {
	if !testutil.IsOVHConfigured() {
		fmt.Println("Skipping integration tests: OVH credentials not configured")
		fmt.Println("Set OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET, OVH_CONSUMER_KEY, and OVH_CLOUD_PROJECT_ID")
		os.Exit(0)
	}

	var err error
	testOVHClient, err = testutil.NewOVHClient()
	if err != nil {
		fmt.Printf("Failed to create OVH client: %v\n", err)
		os.Exit(1)
	}

	// Get the Container provisioner factory from registry
	containerFactory, ok := registry.GetOVHFactory(ContainerResourceType)
	if !ok {
		fmt.Printf("Container resource type not registered: %s\n", ContainerResourceType)
		os.Exit(1)
	}
	testContainerProvisioner = containerFactory(testOVHClient)

	// Build target config with project ID
	testTargetConfig, _ = json.Marshal(map[string]interface{}{
		"projectId": testutil.OVHCloudProjectID,
		"region":    testSwiftRegion,
	})

	os.Exit(m.Run())
}

func TestContainer_Create_Read_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	containerName := fmt.Sprintf("formae-test-container-read-%d", time.Now().Unix())

	// Create a container
	createProps, _ := json.Marshal(map[string]interface{}{
		"containerName": containerName,
		"region":        testSwiftRegion,
		"archive":       false,
	})

	createResult, err := testContainerProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ContainerResourceType,
		Label:        containerName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Created container with native ID: %s", nativeID)

	// Clean up after test
	defer func() {
		deleteReq := &resource.DeleteRequest{
			ResourceType: ContainerResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testContainerProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test container: %s", nativeID)
	}()

	// Container creation is synchronous, so we can read immediately
	readReq := &resource.ReadRequest{
		ResourceType: ContainerResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testContainerProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")
	assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

	// Verify the properties contain expected fields
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, containerName, props["name"], "Container name should match") // API returns "name" in response

	t.Logf("✓ Read container: %s", nativeID)
}

func TestContainer_Update_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	containerName := fmt.Sprintf("formae-test-container-update-%d", time.Now().Unix())

	// Create a container
	createProps, _ := json.Marshal(map[string]interface{}{
		"containerName": containerName,
		"region":        testSwiftRegion,
		"archive":       false,
	})

	createResult, err := testContainerProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ContainerResourceType,
		Label:        containerName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Clean up after test
	defer func() {
		deleteReq := &resource.DeleteRequest{
			ResourceType: ContainerResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testContainerProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test container: %s", nativeID)
	}()

	// Update containerType (the only updatable field per OVH API)
	// Valid values: "private", "public", "static"
	updateProps, _ := json.Marshal(map[string]interface{}{
		"containerType": "public",
	})

	updateReq := &resource.UpdateRequest{
		ResourceType:      ContainerResourceType,
		NativeID:          nativeID,
		Label:             containerName,
		DesiredProperties: updateProps,
		TargetConfig:      testTargetConfig,
	}

	updateResult, err := testContainerProvisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "UpdateResult should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus,
		"Update operation should succeed, got: %s - %s",
		updateResult.ProgressResult.OperationStatus, updateResult.ProgressResult.StatusMessage)

	t.Logf("✓ Updated container: %s", nativeID)
}

func TestContainer_Delete_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	containerName := fmt.Sprintf("formae-test-container-delete-%d", time.Now().Unix())

	// Create a container
	createProps, _ := json.Marshal(map[string]interface{}{
		"containerName": containerName,
		"region":        testSwiftRegion,
		"archive":       false,
	})

	createResult, err := testContainerProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ContainerResourceType,
		Label:        containerName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Delete the container
	deleteReq := &resource.DeleteRequest{
		ResourceType: ContainerResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testContainerProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete operation should succeed")

	t.Logf("✓ Deleted container: %s", nativeID)
}

func TestContainer_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// Use a non-existent container ID
	nonExistentNativeID := fmt.Sprintf("%s/non-existent-container-id", testutil.OVHCloudProjectID)

	deleteReq := &resource.DeleteRequest{
		ResourceType: ContainerResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testContainerProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	// Delete should be idempotent - 404 is treated as success
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent resource should succeed (idempotent)")

	t.Logf("✓ Delete of non-existent container returned success (idempotent)")
}

func TestContainer_List_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	containerName := fmt.Sprintf("formae-test-container-list-%d", time.Now().Unix())

	// Create a container
	createProps, _ := json.Marshal(map[string]interface{}{
		"containerName": containerName,
		"region":        testSwiftRegion,
		"archive":       false,
	})

	createResult, err := testContainerProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ContainerResourceType,
		Label:        containerName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Clean up after test
	defer func() {
		deleteReq := &resource.DeleteRequest{
			ResourceType: ContainerResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testContainerProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test container: %s", nativeID)
	}()

	// Test List
	listReq := &resource.ListRequest{
		ResourceType: ContainerResourceType,
		TargetConfig: testTargetConfig,
		AdditionalProperties: map[string]string{
			"serviceName": testutil.OVHCloudProjectID,
		},
	}

	listResult, err := testContainerProvisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, listResult, "ListResult should not be nil")

	// The created container should be in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created container should be in the list. NativeID: %s, List: %v",
		nativeID, listResult.NativeIDs)

	t.Logf("✓ List returned %d containers, including test container", len(listResult.NativeIDs))
}
