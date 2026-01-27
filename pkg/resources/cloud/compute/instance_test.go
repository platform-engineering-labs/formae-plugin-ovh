// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package compute

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

var (
	testOVHClient    *ovhtransport.Client
	testProvisioner  prov.Provisioner
	testTargetConfig json.RawMessage
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

	// Get the provisioner factory from registry
	factory, ok := registry.GetOVHFactory(InstanceResourceType)
	if !ok {
		fmt.Printf("Instance resource type not registered: %s\n", InstanceResourceType)
		os.Exit(1)
	}
	testProvisioner = factory(testOVHClient)

	// Build target config with project ID
	testTargetConfig, _ = json.Marshal(map[string]interface{}{
		"projectId": testutil.OVHCloudProjectID,
		"region":    testutil.Region,
	})

	os.Exit(m.Run())
}

// TestInstance_ListFlavorsAndImages_Integration is a helper test to list available flavors and images.
// Run this first to get valid IDs for your region.
func TestInstance_ListFlavorsAndImages_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// List flavors
	t.Log("Available Flavors:")
	flavorsResp, err := testOVHClient.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   fmt.Sprintf("/cloud/project/%s/flavor", testutil.OVHCloudProjectID),
	})
	if err != nil {
		t.Logf("Failed to list flavors: %v", err)
	} else {
		for _, f := range flavorsResp.BodyArray {
			if flavor, ok := f.(map[string]interface{}); ok {
				t.Logf("  - %s: %s (region: %s)", flavor["id"], flavor["name"], flavor["region"])
			}
		}
	}

	// List images (limit to region)
	t.Log("\nAvailable Images (first 10):")
	imagesResp, err := testOVHClient.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   fmt.Sprintf("/cloud/project/%s/image?region=%s", testutil.OVHCloudProjectID, testutil.Region),
	})
	if err != nil {
		t.Logf("Failed to list images: %v", err)
	} else {
		count := 0
		for _, i := range imagesResp.BodyArray {
			if count >= 10 {
				break
			}
			if image, ok := i.(map[string]interface{}); ok {
				t.Logf("  - %s: %s", image["id"], image["name"])
				count++
			}
		}
	}

	t.Log("\nSet OS_TEST_FLAVOR_ID and OS_TEST_IMAGE_ID in .env with valid IDs from above")
}

func TestInstance_Create_Read_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	instanceName := fmt.Sprintf("formae-test-instance-read-%d", time.Now().Unix())

	// First create an instance
	createProps, _ := json.Marshal(map[string]interface{}{
		"name":     instanceName,
		"flavorId": testutil.TestFlavorID,
		"imageId":  testutil.TestImageID,
		"region":   testutil.Region,
	})

	createResult, err := testProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: InstanceResourceType,
		Label:        instanceName,
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
			ResourceType: InstanceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test instance: %s", nativeID)
	}()

	// Wait for instance to be ACTIVE before reading
	t.Log("Waiting for instance to be ACTIVE...")
	_, err = testutil.WaitForCreate(t, ctx, testProvisioner, createResult, testTargetConfig, InstanceResourceType)
	require.NoError(t, err, "Instance should become ACTIVE")

	// Now test Read
	readReq := &resource.ReadRequest{
		ResourceType: InstanceResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")
	assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

	// Verify the properties contain expected fields
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, instanceName, props["name"], "Instance name should match")
	assert.NotEmpty(t, props["id"], "Instance should have an ID")
	assert.Equal(t, "ACTIVE", props["status"], "Instance status should be ACTIVE")

	t.Logf("✓ Read instance: %s", nativeID)
}

func TestInstance_Update_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	instanceName := fmt.Sprintf("formae-test-instance-update-%d", time.Now().Unix())

	// First create an instance
	createProps, _ := json.Marshal(map[string]interface{}{
		"name":     instanceName,
		"flavorId": testutil.TestFlavorID,
		"imageId":  testutil.TestImageID,
		"region":   testutil.Region,
	})

	createResult, err := testProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: InstanceResourceType,
		Label:        instanceName,
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
			ResourceType: InstanceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test instance: %s", nativeID)
	}()

	// Wait for instance to be ACTIVE before updating
	t.Log("Waiting for instance to be ACTIVE...")
	_, err = testutil.WaitForCreate(t, ctx, testProvisioner, createResult, testTargetConfig, InstanceResourceType)
	require.NoError(t, err, "Instance should become ACTIVE")

	// Now test Update (rename the instance)
	updatedName := instanceName + "-updated"
	updateProps, _ := json.Marshal(map[string]interface{}{
		"instanceName": updatedName,
	})

	updateReq := &resource.UpdateRequest{
		ResourceType:      InstanceResourceType,
		NativeID:          nativeID,
		Label:             updatedName,
		DesiredProperties: updateProps,
		TargetConfig:      testTargetConfig,
	}

	updateResult, err := testProvisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "UpdateResult should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus,
		"Update operation should succeed, got: %s - %s",
		updateResult.ProgressResult.OperationStatus, updateResult.ProgressResult.StatusMessage)

	t.Logf("✓ Updated instance: %s", nativeID)
}

func TestInstance_Delete_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	instanceName := fmt.Sprintf("formae-test-instance-delete-%d", time.Now().Unix())

	// First create an instance
	createProps, _ := json.Marshal(map[string]interface{}{
		"name":     instanceName,
		"flavorId": testutil.TestFlavorID,
		"imageId":  testutil.TestImageID,
		"region":   testutil.Region,
	})

	createResult, err := testProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: InstanceResourceType,
		Label:        instanceName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Wait for instance to be ACTIVE before deleting
	t.Log("Waiting for instance to be ACTIVE...")
	_, err = testutil.WaitForCreate(t, ctx, testProvisioner, createResult, testTargetConfig, InstanceResourceType)
	require.NoError(t, err, "Instance should become ACTIVE")

	// Now test Delete
	deleteReq := &resource.DeleteRequest{
		ResourceType: InstanceResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete operation should succeed")

	// Wait for instance to be fully deleted (deletion is async)
	t.Log("Waiting for instance to be fully deleted...")
	err = testutil.WaitForDeleteComplete(t, ctx, testProvisioner, nativeID, testTargetConfig, InstanceResourceType)
	require.NoError(t, err, "Instance should be fully deleted")

	t.Logf("✓ Deleted instance: %s", nativeID)
}

func TestInstance_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// Use a non-existent instance ID
	nonExistentNativeID := fmt.Sprintf("%s/00000000-0000-0000-0000-000000000000", testutil.OVHCloudProjectID)

	deleteReq := &resource.DeleteRequest{
		ResourceType: InstanceResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	// Delete should be idempotent - 404 is treated as success
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent resource should succeed (idempotent)")

	t.Logf("✓ Delete of non-existent instance returned success (idempotent)")
}

func TestInstance_List_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	instanceName := fmt.Sprintf("formae-test-instance-list-%d", time.Now().Unix())

	// First create an instance_
	createProps, _ := json.Marshal(map[string]interface{}{
		"name":     instanceName,
		"flavorId": testutil.TestFlavorID,
		"imageId":  testutil.TestImageID,
		"region":   testutil.Region,
	})

	createResult, err := testProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: InstanceResourceType,
		Label:        instanceName,
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
			ResourceType: InstanceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testProvisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test instance: %s", nativeID)
	}()

	// Wait for instance to be ACTIVE
	t.Log("Waiting for instance to be ACTIVE...")
	_, err = testutil.WaitForCreate(t, ctx, testProvisioner, createResult, testTargetConfig, InstanceResourceType)
	require.NoError(t, err, "Instance should become ACTIVE")

	// Now test List
	listReq := &resource.ListRequest{
		ResourceType: InstanceResourceType,
		TargetConfig: testTargetConfig,
	}

	listResult, err := testProvisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, listResult, "ListResult should not be nil")

	// The created instance should be in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created instance should be in the list. NativeID: %s, List: %v",
		nativeID, listResult.NativeIDs)

	t.Logf("✓ List returned %d instances, including test instance", len(listResult.NativeIDs))
}
