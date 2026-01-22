// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package main

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
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Test configuration from environment
	testAuthURL    string
	testUsername   string
	testPassword   string
	testProjectID  string
	testRegion     string
	testDomainName string

	// Test resources
	testKeypairName       string
	testSecurityGroupName string
	ovhPlugin             *OVH

	// Gophercloud client for verification and cleanup
	computeClient *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
)

func TestMain(m *testing.M) {
	// Load test configuration from environment
	testAuthURL = os.Getenv("OS_AUTH_URL")
	testUsername = os.Getenv("OS_USERNAME")
	testPassword = os.Getenv("OS_PASSWORD")
	testProjectID = os.Getenv("OS_PROJECT_ID")
	testRegion = os.Getenv("OS_REGION_NAME")
	testDomainName = os.Getenv("OS_USER_DOMAIN_NAME")
	if testDomainName == "" {
		testDomainName = "Default"
	}

	// Validate required environment variables
	if testAuthURL == "" || testUsername == "" || testPassword == "" || testProjectID == "" || testRegion == "" {
		fmt.Println("ERROR: Missing required environment variables:")
		fmt.Println("  OS_AUTH_URL:", testAuthURL)
		fmt.Println("  OS_USERNAME:", testUsername)
		fmt.Println("  OS_PASSWORD:", "[set]")
		fmt.Println("  OS_PROJECT_ID:", testProjectID)
		fmt.Println("  OS_REGION_NAME:", testRegion)
		fmt.Println("\nPlease source your OpenStack credentials file:")
		fmt.Println("  source ~/.ovh-openstack-credentials")
		os.Exit(1)
	}

	// Create unique test resource names
	timestamp := time.Now().Unix()
	testKeypairName = fmt.Sprintf("formae-test-keypair-%d", timestamp)
	testSecurityGroupName = fmt.Sprintf("formae-test-secgroup-%d", timestamp)

	// Initialize plugin
	ovhPlugin = &Plugin{}

	// Create gophercloud client for verification and cleanup
	provider, err := openstack.NewClient(testAuthURL)
	if err != nil {
		fmt.Printf("ERROR: Failed to create OpenStack client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	err = openstack.Authenticate(ctx, provider, gophercloud.AuthOptions{
		IdentityEndpoint: testAuthURL,
		Username:         testUsername,
		Password:         testPassword,
		TenantID:         testProjectID,
		DomainName:       testDomainName,
		AllowReauth:      true,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to authenticate with OpenStack: %v\n", err)
		os.Exit(1)
	}

	computeClient, err = openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: testRegion,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create compute client: %v\n", err)
		os.Exit(1)
	}

	networkClient, err = openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: testRegion,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to create network client: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup: Delete test keypair if it exists
	if testKeypairName != "" {
		err = keypairs.Delete(ctx, computeClient, testKeypairName, nil).ExtractErr()
		if err != nil {
			fmt.Printf("WARNING: Failed to cleanup test keypair %s: %v\n", testKeypairName, err)
		} else {
			fmt.Printf("✓ Cleaned up test keypair: %s\n", testKeypairName)
		}
	}

	// Cleanup: Delete test security group if it exists
	if testSecurityGroupName != "" {
		// Find the security group by name to get its ID
		listOpts := groups.ListOpts{Name: testSecurityGroupName}
		allPages, err := groups.List(networkClient, listOpts).AllPages(ctx)
		if err == nil {
			sgs, err := groups.ExtractGroups(allPages)
			if err == nil && len(sgs) > 0 {
				for _, sg := range sgs {
					err := groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
					if err != nil {
						fmt.Printf("WARNING: Failed to cleanup test security group %s: %v\n", sg.ID, err)
					} else {
						fmt.Printf("✓ Cleaned up test security group: %s\n", sg.Name)
					}
				}
			}
		}
	}

	os.Exit(code)
}

func TestOVHKeypairCreate_Integration(t *testing.T) {
	ctx := context.Background()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s"
	}`, testKeypairName))

	// Create request
	req := &resource.CreateRequest{
		Resource: &model.Resource{
			Type:       "OVH::Compute::Keypair",
			Label:      testKeypairName,
			Stack:      "test-stack",
			Properties: properties,
		},
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Create operation
	result, err := ovhPlugin.Create(ctx, req)

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify keypair was actually created in OpenStack
	kp, err := keypairs.Get(ctx, computeClient, testKeypairName, nil).Extract()
	require.NoError(t, err, "Should be able to get keypair from OpenStack")
	assert.Equal(t, testKeypairName, kp.Name)
	assert.NotEmpty(t, kp.PublicKey, "Public key should be generated")
	assert.NotEmpty(t, kp.Fingerprint, "Fingerprint should be set")

	t.Logf("✓ Keypair created successfully:")
	t.Logf("  Name: %s", kp.Name)
	t.Logf("  Fingerprint: %s", kp.Fingerprint)
	t.Logf("  NativeID: %s", result.ProgressResult.NativeID)
}

func TestOVHKeypairRead_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a keypair using gophercloud directly
	timestamp := time.Now().Unix()
	keypairName := fmt.Sprintf("formae-test-keypair-read-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name: keypairName,
	}
	_, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")
	defer func() {
		_ = keypairs.Delete(ctx, computeClient, keypairName, nil).ExtractErr()
	}()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: "OVH::Compute::Keypair",
		NativeID:     keypairName,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Read operation
	result, err := ovhPlugin.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.Equal(t, "OVH::Compute::Keypair", result.ResourceType)
	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, keypairName, props["name"])
	assert.NotEmpty(t, props["fingerprint"], "Fingerprint should be present")
	assert.NotEmpty(t, props["publicKey"], "Public key should be present")

	t.Logf("✓ Keypair read successfully:")
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Fingerprint: %s", props["fingerprint"])
}

func TestOVHKeypairDelete_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a keypair using gophercloud directly
	timestamp := time.Now().Unix()
	keypairName := fmt.Sprintf("formae-test-keypair-delete-%d", timestamp)

	createOpts := keypairs.CreateOpts{
		Name: keypairName,
	}
	_, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test keypair")

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: "OVH::Compute::Keypair",
		NativeID:     &keypairName,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Delete operation
	result, err := ovhPlugin.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	// Verify deletion was successful
	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify keypair was actually deleted from OpenStack
	_, err = keypairs.Get(ctx, computeClient, keypairName, nil).Extract()
	assert.Error(t, err, "Keypair should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted keypair")

	t.Logf("✓ Keypair deleted successfully:")
	t.Logf("  Name: %s", keypairName)
}

func TestOVHKeypairDelete_NotFound_Integration(t *testing.T) {
	ctx := context.Background()

	// Use a non-existent keypair name
	keypairName := "formae-test-keypair-nonexistent"

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: "OVH::Compute::Keypair",
		NativeID:     &keypairName,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Delete operation
	result, err := ovhPlugin.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestOVHKeypairList_Integration(t *testing.T) {
	ctx := context.Background()

	// Create a few test keypairs using gophercloud directly
	timestamp := time.Now().Unix()
	testPrefix := fmt.Sprintf("formae-test-list-%d", timestamp)

	keypairNames := []string{
		fmt.Sprintf("%s-1", testPrefix),
		fmt.Sprintf("%s-2", testPrefix),
		fmt.Sprintf("%s-3", testPrefix),
	}

	// Create test keypairs
	for _, name := range keypairNames {
		createOpts := keypairs.CreateOpts{
			Name: name,
		}
		_, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
		require.NoError(t, err, "Failed to create test keypair: %s", name)
	}

	// Cleanup after test
	defer func() {
		for _, name := range keypairNames {
			_ = keypairs.Delete(ctx, computeClient, name, nil).ExtractErr()
		}
	}()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create List request
	req := &resource.ListRequest{
		ResourceType: "OVH::Compute::Keypair",
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin List operation
	result, err := ovhPlugin.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.Resources, "Resources should not be empty")

	// Verify the resource type is set correctly
	assert.Equal(t, "OVH::Compute::Keypair", result.ResourceType)

	// Verify our test keypairs are in the list
	foundCount := 0
	for _, res := range result.Resources {
		assert.NotEmpty(t, res.NativeID, "NativeID should be set")
		assert.NotEmpty(t, res.Properties, "Properties should be set")

		// Parse properties to check names
		var props map[string]interface{}
		err := json.Unmarshal([]byte(res.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal properties")

		name, ok := props["name"].(string)
		assert.True(t, ok, "name property should be a string")

		// Count how many of our test keypairs we found
		for _, testName := range keypairNames {
			if name == testName {
				foundCount++
				t.Logf("✓ Found test keypair in list: %s", name)
			}
		}
	}

	assert.Equal(t, len(keypairNames), foundCount, "Should find all %d test keypairs", len(keypairNames))

	t.Logf("✓ List operation successful:")
	t.Logf("  Total resources found: %d", len(result.Resources))
	t.Logf("  Test resources found: %d/%d", foundCount, len(keypairNames))
}

// ============================================================================
// SecurityGroup Tests
// ============================================================================

func TestOVHSecurityGroup_Create_Integration(t *testing.T) {
	ctx := context.Background()

	// Use a unique name for this test
	timestamp := time.Now().Unix()
	secGroupName := fmt.Sprintf("formae-test-sg-create-%d", timestamp)

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Prepare resource properties
	properties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Test security group created by formae integration tests"
	}`, secGroupName))

	// Create request
	req := &resource.CreateRequest{
		Resource: &model.Resource{
			Type:       "OVH::Network::SecurityGroup",
			Label:      secGroupName,
			Stack:      "test-stack",
			Properties: properties,
		},
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Create operation
	result, err := ovhPlugin.Create(ctx, req)

	// Cleanup immediately after test
	if result != nil && result.ProgressResult != nil && result.ProgressResult.NativeID != "" {
		defer func() {
			_ = groups.Delete(ctx, networkClient, result.ProgressResult.NativeID).ExtractErr()
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
	assert.Equal(t, secGroupName, sg.Name)
	assert.Equal(t, "Test security group created by formae integration tests", sg.Description)

	t.Logf("✓ Security group created successfully:")
	t.Logf("  Name: %s", sg.Name)
	t.Logf("  ID: %s", sg.ID)
	t.Logf("  Description: %s", sg.Description)
	t.Logf("  NativeID: %s", result.ProgressResult.NativeID)
}

func TestOVHSecurityGroup_Read_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	secGroupName := fmt.Sprintf("formae-test-secgroup-read-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        secGroupName,
		Description: "Test security group for read operation",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: "OVH::Network::SecurityGroup",
		NativeID:     sg.ID,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Read operation
	result, err := ovhPlugin.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.Equal(t, "OVH::Network::SecurityGroup", result.ResourceType)
	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, sg.ID, props["id"])
	assert.Equal(t, secGroupName, props["name"])
	assert.Equal(t, "Test security group for read operation", props["description"])

	t.Logf("✓ Security group read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Name: %s", props["name"])
	t.Logf("  Description: %s", props["description"])
}

func TestOVHSecurityGroup_Update_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	secGroupName := fmt.Sprintf("formae-test-secgroup-update-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        secGroupName,
		Description: "Initial description",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Prepare updated properties (mutable field: description)
	updatedProperties := []byte(fmt.Sprintf(`{
		"name": "%s",
		"description": "Updated description"
	}`, secGroupName))

	// Create Update request
	req := &resource.UpdateRequest{
		NativeID: &sg.ID,
		Resource: &model.Resource{
			Type:       "OVH::Network::SecurityGroup",
			Label:      secGroupName,
			Stack:      "test-stack",
			Properties: updatedProperties,
		},
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Update operation
	result, err := ovhPlugin.Update(ctx, req)

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
	assert.Equal(t, "Updated description", updatedSG.Description, "Description should be updated")
	assert.Equal(t, secGroupName, updatedSG.Name, "Name should remain unchanged")

	t.Logf("✓ Security group updated successfully:")
	t.Logf("  ID: %s", updatedSG.ID)
	t.Logf("  Name: %s", updatedSG.Name)
	t.Logf("  Old Description: Initial description")
	t.Logf("  New Description: %s", updatedSG.Description)
}

func TestOVHSecurityGroup_Delete_Integration(t *testing.T) {
	ctx := context.Background()

	// First, create a security group using gophercloud directly
	timestamp := time.Now().Unix()
	secGroupName := fmt.Sprintf("formae-test-secgroup-delete-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        secGroupName,
		Description: "Test security group for delete operation",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: "OVH::Network::SecurityGroup",
		NativeID:     &sg.ID,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Delete operation
	result, err := ovhPlugin.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	// Verify deletion was successful
	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify security group was actually deleted from OpenStack
	_, err = groups.Get(ctx, networkClient, sg.ID).Extract()
	assert.Error(t, err, "Security group should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted security group")

	t.Logf("✓ Security group deleted successfully:")
	t.Logf("  Name: %s", secGroupName)
	t.Logf("  ID: %s", sg.ID)
}

func TestOVHSecurityGroup_Delete_NotFound_Integration(t *testing.T) {
	ctx := context.Background()

	// Use a non-existent security group ID (valid UUID format)
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: "OVH::Network::SecurityGroup",
		NativeID:     &nonExistentID,
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin Delete operation
	result, err := ovhPlugin.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestOVHSecurityGroup_List_Integration(t *testing.T) {
	ctx := context.Background()

	// Create ONE test security group (due to quota constraints)
	timestamp := time.Now().Unix()
	secGroupName := fmt.Sprintf("formae-test-list-sg-%d", timestamp)

	createOpts := groups.CreateOpts{
		Name:        secGroupName,
		Description: "Test security group for list operation",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group: %s", secGroupName)

	// Cleanup after test
	defer func() {
		_ = groups.Delete(ctx, networkClient, sg.ID).ExtractErr()
	}()

	// Prepare target config (JSON)
	targetConfig := []byte(fmt.Sprintf(`{
		"authURL": "%s",
		"username": "%s",
		"password": "%s",
		"projectID": "%s",
		"region": "%s",
		"domainName": "%s"
	}`, testAuthURL, testUsername, testPassword, testProjectID, testRegion, testDomainName))

	// Create List request
	req := &resource.ListRequest{
		ResourceType: "OVH::Network::SecurityGroup",
		Target: &model.Target{
			Namespace: "OVH",
			Config:    targetConfig,
		},
	}

	// Execute plugin List operation
	result, err := ovhPlugin.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.Resources, "Resources should not be empty")

	// Verify the resource type is set correctly
	assert.Equal(t, "OVH::Network::SecurityGroup", result.ResourceType)

	// Verify our test security group is in the list
	found := false
	for _, res := range result.Resources {
		assert.NotEmpty(t, res.NativeID, "NativeID should be set")
		assert.NotEmpty(t, res.Properties, "Properties should be set")

		// Parse properties to check names
		var props map[string]interface{}
		err := json.Unmarshal([]byte(res.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal properties")

		name, ok := props["name"].(string)
		assert.True(t, ok, "name property should be a string")

		// Check if this is our test security group
		if name == secGroupName {
			found = true
			t.Logf("✓ Found test security group in list: %s", name)
		}
	}

	assert.True(t, found, "Should find test security group in the list")

	t.Logf("✓ List operation successful:")
	t.Logf("  Total resources found: %d", len(result.Resources))
	t.Logf("  Test security group found: %v", found)
}
