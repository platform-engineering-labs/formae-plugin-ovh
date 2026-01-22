// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Test plugin client (minimal - only network client needed for FloatingIP tests)
	testClient *client.Client

	// Gophercloud client for verification and cleanup
	networkClient *gophercloud.ServiceClient
)

func TestMain(m *testing.M) {
	// Check if credentials are configured
	if !testutil.IsConfigured() {
		fmt.Println("ERROR: Missing required environment variables:")
		fmt.Println("  OS_USERNAME, OS_PASSWORD, OS_PROJECT_ID")
		fmt.Println("\nPlease source your OpenStack credentials file:")
		fmt.Println("  source ~/.ovh-openstack-credentials")
		os.Exit(1)
	}

	ctx := context.Background()

	// Create provider client for authentication
	provider, err := openstack.NewClient(testutil.AuthURL)
	if err != nil {
		fmt.Printf("ERROR: Failed to create OpenStack client: %v\n", err)
		os.Exit(1)
	}

	err = openstack.Authenticate(ctx, provider, gophercloud.AuthOptions{
		IdentityEndpoint: testutil.AuthURL,
		Username:         testutil.Username,
		Password:         testutil.Password,
		TenantID:         testutil.ProjectID,
		DomainName:       testutil.DomainName,
		AllowReauth:      true,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to authenticate with OpenStack: %v\n", err)
		os.Exit(1)
	}

	// Create network client only (FloatingIP tests don't need compute or volume)
	networkClient, err = openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: testutil.Region,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create network client: %v\n", err)
		os.Exit(1)
	}

	// Create minimal plugin client with only network service
	testClient = &client.Client{
		Config:        testutil.Config,
		NetworkClient: networkClient,
	}

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// ============================================================================
// FloatingIP Tests
// ============================================================================

func TestFloatingIP_Create_Integration(t *testing.T) {
	ctx := context.Background()

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique description for this test
	timestamp := time.Now().Unix()
	description := fmt.Sprintf("formae-test-fip-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"floating_network_id": "%s",
		"description": "%s"
	}`, testutil.ExternalNetworkID, description))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeFloatingIP,
		Label:        "test-floatingip",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := fipProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = floatingips.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
			t.Logf("✓ Cleaned up test floating IP: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify floating IP was actually created in OpenStack
	fip, err := floatingips.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get floating IP from OpenStack")
	assert.Equal(t, testutil.ExternalNetworkID, fip.FloatingNetworkID)
	assert.Equal(t, description, fip.Description)
	assert.NotEmpty(t, fip.FloatingIP, "Floating IP address should be allocated")

	t.Logf("✓ Floating IP created successfully:")
	t.Logf("  ID: %s", fip.ID)
	t.Logf("  IP Address: %s", fip.FloatingIP)
	t.Logf("  Description: %s", fip.Description)
	t.Logf("  Status: %s", fip.Status)
}

func TestFloatingIP_Read_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a floating IP using gophercloud directly
	timestamp := time.Now().Unix()
	description := fmt.Sprintf("formae-test-fip-read-%d", timestamp)

	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: testutil.ExternalNetworkID,
		Description:       description,
	}
	fip, err := floatingips.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test floating IP")
	defer func() {
		_ = floatingips.Delete(ctx, networkClient, fip.ID).ExtractErr()
	}()

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeFloatingIP,
		NativeID:     fip.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := fipProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, fip.ID, props["id"])
	assert.Equal(t, testutil.ExternalNetworkID, props["floating_network_id"])
	assert.Equal(t, fip.FloatingIP, props["floating_ip_address"])
	assert.Equal(t, description, props["description"])

	t.Logf("✓ Floating IP read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  IP Address: %s", props["floating_ip_address"])
	t.Logf("  Description: %s", props["description"])
}

func TestFloatingIP_Update_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a floating IP using gophercloud directly
	timestamp := time.Now().Unix()
	initialDescription := fmt.Sprintf("formae-test-fip-update-initial-%d", timestamp)

	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: testutil.ExternalNetworkID,
		Description:       initialDescription,
	}
	fip, err := floatingips.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test floating IP")
	defer func() {
		_ = floatingips.Delete(ctx, networkClient, fip.ID).ExtractErr()
	}()

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare updated properties (mutable field: description)
	updatedDescription := fmt.Sprintf("formae-test-fip-update-updated-%d", timestamp)
	updatedProperties := []byte(fmt.Sprintf(`{
		"floating_network_id": "%s",
		"description": "%s"
	}`, testutil.ExternalNetworkID, updatedDescription))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeFloatingIP,
		NativeID:          fip.ID,
		Label:             "test-floatingip",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := fipProvisioner.Update(ctx, req)

	// Assert results
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, fip.ID, result.ProgressResult.NativeID, "NativeID should not change")

	// Verify the update was applied in OpenStack
	updatedFip, err := floatingips.Get(ctx, networkClient, fip.ID).Extract()
	require.NoError(t, err, "Should be able to get updated floating IP")
	assert.Equal(t, updatedDescription, updatedFip.Description, "Description should be updated")
	assert.Equal(t, fip.FloatingIP, updatedFip.FloatingIP, "IP address should remain unchanged")

	t.Logf("✓ Floating IP updated successfully:")
	t.Logf("  ID: %s", updatedFip.ID)
	t.Logf("  IP Address: %s", updatedFip.FloatingIP)
	t.Logf("  Old Description: %s", initialDescription)
	t.Logf("  New Description: %s", updatedFip.Description)
}

func TestFloatingIP_Delete_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a floating IP using gophercloud directly
	timestamp := time.Now().Unix()
	description := fmt.Sprintf("formae-test-fip-delete-%d", timestamp)

	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: testutil.ExternalNetworkID,
		Description:       description,
	}
	fip, err := floatingips.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test floating IP")

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeFloatingIP,
		NativeID:     fip.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := fipProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	// Verify deletion was successful
	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify floating IP was actually deleted from OpenStack
	_, err = floatingips.Get(ctx, networkClient, fip.ID).Extract()
	assert.Error(t, err, "Floating IP should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted floating IP")

	t.Logf("✓ Floating IP deleted successfully:")
	t.Logf("  ID: %s", fip.ID)
	t.Logf("  IP Address: %s", fip.FloatingIP)
}

func TestFloatingIP_Delete_NotFound_Integration(t *testing.T) {
	ctx := context.Background()

	// Use a non-existent floating IP ID (valid UUID format)
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeFloatingIP,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := fipProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestFloatingIP_List_Integration(t *testing.T) {
	ctx := context.Background()

	// Create ONE test floating IP (due to quota constraints)
	timestamp := time.Now().Unix()
	description := fmt.Sprintf("formae-test-fip-list-%d", timestamp)

	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: testutil.ExternalNetworkID,
		Description:       description,
	}
	fip, err := floatingips.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test floating IP")

	// Cleanup after test
	defer func() {
		_ = floatingips.Delete(ctx, networkClient, fip.ID).ExtractErr()
	}()

	// Create FloatingIP provisioner
	fipProvisioner := &FloatingIP{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeFloatingIP,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := fipProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test floating IP is in the list
	found := false
	for _, id := range result.NativeIDs {
		if id == fip.ID {
			found = true
			t.Logf("✓ Found test floating IP in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test floating IP in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total floating IPs found: %d", len(result.NativeIDs))
	t.Logf("  Test floating IP found: %v", found)
}
