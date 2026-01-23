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

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Router Tests
// ============================================================================

func TestRouter_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-create-%d", timestamp)
	description := "Test router created by integration tests"

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "%s",
		"admin_state_up": true
	}`, name, description))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeRouter,
		Label:        "test-router",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := routerProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = routers.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test router: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify router was actually created in OpenStack
	router, err := routers.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get router from OpenStack")
	assert.Equal(t, name, router.Name)
	assert.Equal(t, description, router.Description)
	assert.True(t, router.AdminStateUp)

	t.Logf("✓ Router created successfully:")
	t.Logf("  ID: %s", router.ID)
	t.Logf("  Name: %s", router.Name)
	t.Logf("  Description: %s", router.Description)
	t.Logf("  AdminStateUp: %v", router.AdminStateUp)
}

func TestRouter_Create_WithExternalGateway_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-gateway-%d", timestamp)

	// Prepare resource properties with external gateway
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Router with external gateway",
		"admin_state_up": true,
		"external_gateway_info": {
			"network_id": "%s"
		}
	}`, name, testutil.ExternalNetworkID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeRouter,
		Label:        "test-router-gateway",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := routerProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = routers.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test router: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify router was created with external gateway
	router, err := routers.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get router from OpenStack")
	assert.Equal(t, name, router.Name)
	assert.Equal(t, testutil.ExternalNetworkID, router.GatewayInfo.NetworkID)

	t.Logf("✓ Router with external gateway created successfully:")
	t.Logf("  ID: %s", router.ID)
	t.Logf("  Name: %s", router.Name)
	t.Logf("  External Gateway Network: %s", router.GatewayInfo.NetworkID)
}

func TestRouter_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a router using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-read-%d", timestamp)
	description := "Test router for read integration test"
	adminStateUp := true

	createOpts := routers.CreateOpts{
		Name:         name,
		Description:  description,
		AdminStateUp: &adminStateUp,
	}
	router, err := routers.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test router")
	defer func() {
		_ = routers.Delete(ctx, networkClient, router.ID).ExtractErr()
	}()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeRouter,
		NativeID:     router.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := routerProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, router.ID, props["id"])
	assert.Equal(t, name, props["name"])
	assert.Equal(t, description, props["description"])
	assert.Equal(t, true, props["admin_state_up"])

	t.Logf("✓ Router read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Description: %s", props["description"])
}

func TestRouter_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a router using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-router-update-initial-%d", timestamp)
	initialDescription := "Initial description"
	adminStateUp := true

	createOpts := routers.CreateOpts{
		Name:         initialName,
		Description:  initialDescription,
		AdminStateUp: &adminStateUp,
	}
	router, err := routers.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test router")
	defer func() {
		_ = routers.Delete(ctx, networkClient, router.ID).ExtractErr()
	}()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties
	updatedName := fmt.Sprintf("formae-test-router-update-updated-%d", timestamp)
	updatedDescription := "Updated description"
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "%s",
		"admin_state_up": true
	}`, updatedName, updatedDescription))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeRouter,
		NativeID:          router.ID,
		Label:             "test-router",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := routerProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, router.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedRouter, err := routers.Get(ctx, networkClient, router.ID).Extract()
	require.NoError(t, err, "Should be able to get updated router")
	assert.Equal(t, updatedName, updatedRouter.Name, "Name should be updated")
	assert.Equal(t, updatedDescription, updatedRouter.Description, "Description should be updated")

	t.Logf("✓ Router updated successfully:")
	t.Logf("  ID: %s", updatedRouter.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedRouter.Name)
	t.Logf("  Old Description: %s", initialDescription)
	t.Logf("  New Description: %s", updatedRouter.Description)
}

func TestRouter_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a router using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-delete-%d", timestamp)
	adminStateUp := true

	createOpts := routers.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	router, err := routers.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test router")

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeRouter,
		NativeID:     router.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := routerProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	// Verify deletion was successful
	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify router was actually deleted from OpenStack
	_, err = routers.Get(ctx, networkClient, router.ID).Extract()
	assert.Error(t, err, "Router should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted router")

	t.Logf("✓ Router deleted successfully:")
	t.Logf("  ID: %s", router.ID)
	t.Logf("  Name: %s", name)
}

func TestRouter_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent router ID (valid UUID format)
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeRouter,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := routerProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

// TestRouter_CreateAndUpdate_WithTags_Integration tests the full lifecycle
// matching the testdata/router.pkl and testdata/router-update.pkl scenarios
func TestRouter_CreateAndUpdate_WithTags_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique test run ID like the pkl files do
	timestamp := time.Now().Unix()
	testRunID := fmt.Sprintf("%d", timestamp)

	// === PHASE 1: Create router (matching testdata/router.pkl) ===
	initialName := fmt.Sprintf("formae-plugin-sdk-test-router-%s", testRunID)
	initialDescription := "Test router for plugin SDK tests"

	createProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "%s",
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae"]
	}`, initialName, initialDescription))

	createReq := &resource.CreateRequest{
		ResourceType: ResourceTypeRouter,
		Label:        "plugin-sdk-test-router",
		Properties:   createProperties,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routerProvisioner.Create(ctx, createReq)

	// Cleanup after test
	if createResult != nil && createResult.ProgressResult != nil && createResult.ProgressResult.NativeID != "" {
		defer func() {
			_ = routers.Delete(ctx, networkClient, createResult.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test router: %s", createResult.ProgressResult.NativeID)
		}()
	}

	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "ProgressResult should not be nil")
	assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	routerID := createResult.ProgressResult.NativeID
	require.NotEmpty(t, routerID, "Router ID should be set")

	// Verify initial creation
	router, err := routers.Get(ctx, networkClient, routerID).Extract()
	require.NoError(t, err, "Should be able to get router from OpenStack")
	assert.Equal(t, initialName, router.Name)
	assert.Equal(t, initialDescription, router.Description)
	assert.True(t, router.AdminStateUp)
	assert.Contains(t, router.Tags, "env:test")
	assert.Contains(t, router.Tags, "managed-by:formae")

	t.Logf("✓ Phase 1 - Router created (matching router.pkl):")
	t.Logf("  ID: %s", router.ID)
	t.Logf("  Name: %s", router.Name)
	t.Logf("  Description: %s", router.Description)
	t.Logf("  AdminStateUp: %v", router.AdminStateUp)
	t.Logf("  Tags: %v", router.Tags)

	// === PHASE 2: Update router (matching testdata/router-update.pkl) ===
	updatedName := fmt.Sprintf("formae-plugin-sdk-test-router-%s-updated", testRunID)
	updatedDescription := "Updated description for router"

	updateProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "%s",
		"admin_state_up": false,
		"tags": ["env:test", "managed-by:formae", "updated:true"]
	}`, updatedName, updatedDescription))

	updateReq := &resource.UpdateRequest{
		ResourceType:      ResourceTypeRouter,
		NativeID:          routerID,
		Label:             "plugin-sdk-test-router",
		DesiredProperties: updateProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := routerProvisioner.Update(ctx, updateReq)

	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "Update result should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")
	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus)
	assert.Equal(t, routerID, updateResult.ProgressResult.NativeID, "Router ID should not change")

	// Verify update was applied
	updatedRouter, err := routers.Get(ctx, networkClient, routerID).Extract()
	require.NoError(t, err, "Should be able to get updated router from OpenStack")
	assert.Equal(t, updatedName, updatedRouter.Name, "Name should be updated")
	assert.Equal(t, updatedDescription, updatedRouter.Description, "Description should be updated")
	assert.False(t, updatedRouter.AdminStateUp, "AdminStateUp should be false")
	assert.Contains(t, updatedRouter.Tags, "env:test")
	assert.Contains(t, updatedRouter.Tags, "managed-by:formae")
	assert.Contains(t, updatedRouter.Tags, "updated:true", "New tag should be added")

	t.Logf("✓ Phase 2 - Router updated (matching router-update.pkl):")
	t.Logf("  ID: %s", updatedRouter.ID)
	t.Logf("  Name: %s (was: %s)", updatedRouter.Name, initialName)
	t.Logf("  Description: %s (was: %s)", updatedRouter.Description, initialDescription)
	t.Logf("  AdminStateUp: %v (was: true)", updatedRouter.AdminStateUp)
	t.Logf("  Tags: %v", updatedRouter.Tags)
}

func TestRouter_Read_WithTags_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-read-tags-%d", timestamp)
	expectedTags := []string{"env:test", "managed-by:formae", "test:read-tags"}

	// Prepare resource properties with tags
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test router for read tags integration test",
		"admin_state_up": true,
		"tags": ["env:test", "managed-by:formae", "test:read-tags"]
	}`, name))

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: ResourceTypeRouter,
		Label:        "test-router-tags",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	createResult, err := routerProvisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "ProgressResult should not be nil")
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Create should succeed")

	routerID := createResult.ProgressResult.NativeID

	// Cleanup after test
	defer func() {
		_ = routers.Delete(ctx, networkClient, routerID).ExtractErr()
		t.Logf("Cleaned up test router: %s", routerID)
	}()

	// Now read the router back and verify tags are returned
	readReq := &resource.ReadRequest{
		ResourceType: ResourceTypeRouter,
		NativeID:     routerID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routerProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Parse and verify properties including tags
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, routerID, props["id"])
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

	t.Logf("Router read with tags successful:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Tags: %v", actualTags)
}

func TestRouter_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test router
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-router-list-%d", timestamp)
	adminStateUp := true

	createOpts := routers.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	router, err := routers.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test router")

	// Cleanup after test
	defer func() {
		_ = routers.Delete(ctx, networkClient, router.ID).ExtractErr()
	}()

	// Create Router provisioner
	routerProvisioner := &Router{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeRouter,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := routerProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test router is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == router.ID {
			found = true
			t.Logf("✓ Found test router in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test router in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total routers found: %d", len(result.NativeIDs))
	t.Logf("  Test router found: %v", found)
}
