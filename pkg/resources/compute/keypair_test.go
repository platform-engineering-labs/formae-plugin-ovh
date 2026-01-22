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

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Test plugin client (minimal - only compute client needed)
	testClient *client.Client

	// Gophercloud clients for verification and cleanup
	computeClient *gophercloud.ServiceClient
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

	// Create compute client
	computeClient, err = openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: testutil.Region,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create compute client: %v\n", err)
		os.Exit(1)
	}

	// Create network client (needed for Instance tests)
	networkClient, err = openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: testutil.Region,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create network client: %v\n", err)
		os.Exit(1)
	}

	// Create minimal plugin client with compute and network services
	testClient = &client.Client{
		Config:        testutil.Config,
		ComputeClient: computeClient,
		NetworkClient: networkClient,
	}

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// ============================================================================
// Keypair Tests
// ============================================================================

func TestKeypair_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-create-%d", timestamp)

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"publicKey": "%s"
	}`, name, testutil.TestSSHPublicKey))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeKeypair,
		Label:        "test-keypair",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := keypairProvisioner.Create(ctx, req)

	// Cleanup immediately after test (keypair NativeID is the name)
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = keypairs.Delete(ctx, computeClient, result.ProgressResult.NativeID, nil).ExtractErr()
			t.Logf("✓ Cleaned up test keypair: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, name, result.ProgressResult.NativeID, "NativeID should be the keypair name")

	// Verify keypair was actually created in OpenStack
	kp, err := keypairs.Get(ctx, computeClient, name, nil).Extract()
	require.NoError(t, err, "Should be able to get keypair from OpenStack")
	assert.Equal(t, name, kp.Name)
	assert.NotEmpty(t, kp.Fingerprint, "Fingerprint should be set")

	t.Logf("✓ Keypair created successfully:")
	t.Logf("  Name: %s", kp.Name)
	t.Logf("  Fingerprint: %s", kp.Fingerprint)
}

func TestKeypair_Create_WithoutPublicKey_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-gen-%d", timestamp)

	// Prepare resource properties without public key (OpenStack will generate one)
	properties := []byte(fmt.Sprintf(`{
		"name": "%s"
	}`, name))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeKeypair,
		Label:        "test-keypair-gen",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := keypairProvisioner.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = keypairs.Delete(ctx, computeClient, result.ProgressResult.NativeID, nil).ExtractErr()
			t.Logf("✓ Cleaned up test keypair: %s", result.ProgressResult.NativeID)
		}()
	}

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, name, result.ProgressResult.NativeID)

	// Verify keypair was created
	kp, err := keypairs.Get(ctx, computeClient, name, nil).Extract()
	require.NoError(t, err, "Should be able to get keypair from OpenStack")
	assert.Equal(t, name, kp.Name)
	assert.NotEmpty(t, kp.Fingerprint)

	t.Logf("✓ Keypair created (OpenStack generated key):")
	t.Logf("  Name: %s", kp.Name)
	t.Logf("  Fingerprint: %s", kp.Fingerprint)
}

func TestKeypair_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a keypair using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-read-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: testutil.TestSSHPublicKey,
	}
	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")
	defer func() {
		_ = keypairs.Delete(ctx, computeClient, kp.Name, nil).ExtractErr()
	}()

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request (NativeID is the keypair name)
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeKeypair,
		NativeID:     kp.Name,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := keypairProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, name, props["name"])
	assert.NotEmpty(t, props["fingerprint"])
	assert.NotEmpty(t, props["publicKey"])

	t.Logf("✓ Keypair read successfully:")
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Fingerprint: %s", props["fingerprint"])
}

func TestKeypair_Update_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a keypair using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-update-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: testutil.TestSSHPublicKey,
	}
	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")
	defer func() {
		_ = keypairs.Delete(ctx, computeClient, kp.Name, nil).ExtractErr()
	}()

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare update properties
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"publicKey": "ssh-rsa AAAAB3newkey formae-updated-key"
	}`, name))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeKeypair,
		NativeID:          kp.Name,
		Label:             "test-keypair",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := keypairProvisioner.Update(ctx, req)

	// Assert results - Update should FAIL because keypairs are immutable
	require.NoError(t, err, "Update should not return a Go error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus)
	assert.Equal(t, resource.OperationErrorCodeInvalidRequest, result.ProgressResult.ErrorCode)
	assert.Contains(t, result.ProgressResult.StatusMessage, "immutable")

	t.Logf("✓ Update correctly rejected (keypairs are immutable):")
	t.Logf("  ErrorCode: %s", result.ProgressResult.ErrorCode)
	t.Logf("  StatusMessage: %s", result.ProgressResult.StatusMessage)
}

func TestKeypair_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// First, create a keypair using gophercloud directly
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-delete-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: testutil.TestSSHPublicKey,
	}
	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeKeypair,
		NativeID:     kp.Name,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := keypairProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	// Verify deletion was successful
	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify keypair was actually deleted from OpenStack
	_, err = keypairs.Get(ctx, computeClient, kp.Name, nil).Extract()
	assert.Error(t, err, "Keypair should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted keypair")

	t.Logf("✓ Keypair deleted successfully:")
	t.Logf("  Name: %s", kp.Name)
}

func TestKeypair_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent keypair name
	nonExistentName := "formae-test-keypair-nonexistent-00000000"

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeKeypair,
		NativeID:     nonExistentName,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := keypairProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestKeypair_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a test keypair
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("formae-test-keypair-list-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: testutil.TestSSHPublicKey,
	}
	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")

	// Cleanup after test
	defer func() {
		_ = keypairs.Delete(ctx, computeClient, kp.Name, nil).ExtractErr()
	}()

	// Create Keypair provisioner
	keypairProvisioner := &Keypair{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeKeypair,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := keypairProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test keypair is in the list (NativeID is the name)
	found := false
	for _, id := range result.NativeIDs {
		if id == kp.Name {
			found = true
			t.Logf("✓ Found test keypair in list: %s", id)
			break
		}
	}

	assert.True(t, found, "Should find test keypair in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total keypairs found: %d", len(result.NativeIDs))
	t.Logf("  Test keypair found: %v", found)
}
