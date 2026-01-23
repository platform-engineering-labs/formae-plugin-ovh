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

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SecurityGroup Tests
// ============================================================================

func TestSecurityGroup_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-sg-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test security group for integration tests",
		"tags": ["env:test", "managed-by:formae"]
	}`, name))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeSecurityGroup,
		Label:        "test-security-group",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := sgProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = groups.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test security group: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify security group was actually created in OpenStack
	sg, err := groups.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get security group from OpenStack")
	assert.Equal(t, name, sg.Name)
	assert.Equal(t, "Test security group for integration tests", sg.Description)

	t.Logf("✓ SecurityGroup created successfully:")
	t.Logf("  ID: %s", sg.ID)
	t.Logf("  Name: %s", sg.Name)
	t.Logf("  Description: %s", sg.Description)
}

func TestSecurityGroup_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-sg-read-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        name,
		Description: "Test security group for read test",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeSecurityGroup,
		NativeID:     sg.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := sgProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, sg.ID, props["id"])
	assert.Equal(t, name, props["name"])
	assert.Equal(t, "Test security group for read test", props["description"])

	t.Logf("✓ SecurityGroup read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Description: %s", props["description"])
}

func TestSecurityGroup_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	initialName := fmt.Sprintf("formae-test-sg-update-initial-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        initialName,
		Description: "Initial description",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (name and description are mutable)
	updatedName := fmt.Sprintf("formae-test-sg-update-updated-%d", timestamp)
	updatedDescription := "Updated description"
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "%s",
		"tags": ["env:test", "updated:true"]
	}`, updatedName, updatedDescription))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeSecurityGroup,
		NativeID:          sg.ID,
		Label:             "test-security-group",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := sgProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, sg.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedSG, err := groups.Get(ctx, networkClient, sg.ID).Extract()
	require.NoError(t, err, "Should be able to get updated security group")
	assert.Equal(t, updatedName, updatedSG.Name, "Name should be updated")
	assert.Equal(t, updatedDescription, updatedSG.Description, "Description should be updated")

	t.Logf("✓ SecurityGroup updated successfully:")
	t.Logf("  ID: %s", updatedSG.ID)
	t.Logf("  Old Name: %s", initialName)
	t.Logf("  New Name: %s", updatedSG.Name)
	t.Logf("  Description: %s", updatedSG.Description)
}

func TestSecurityGroup_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-sg-delete-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        name,
		Description: "Test security group for delete test",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSecurityGroup,
		NativeID:     sg.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := sgProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify security group was actually deleted from OpenStack
	_, err = groups.Get(ctx, networkClient, sg.ID).Extract()
	assert.Error(t, err, "SecurityGroup should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted security group")

	t.Logf("✓ SecurityGroup deleted successfully:")
	t.Logf("  ID: %s", sg.ID)
	t.Logf("  Name: %s", name)
}

func TestSecurityGroup_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent security group ID
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSecurityGroup,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := sgProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestSecurityGroup_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test security group
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-sg-list-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        name,
		Description: "Test security group for list test",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")

	// Cleanup after test
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Create SecurityGroup provisioner
	sgProvisioner := &SecurityGroup{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeSecurityGroup,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := sgProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test security group is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == sg.ID {
			found = true
			t.Logf("✓ Found test security group in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test security group in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total security groups found: %d", len(result.NativeIDs))
	t.Logf("  Test security group found: %v", found)
}
