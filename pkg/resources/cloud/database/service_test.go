// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration for database service tests
const (
	testDBEngine  = "mysql"
	testDBVersion = "8"
	testDBPlan    = "essential"
	testDBFlavor  = "db1-4"
	testDBRegion  = "DE"
)

// TestService_ListCapabilities_Integration is a helper test to list available database capabilities.
// Run this first to get valid engine, version, plan, and flavor values for your region.
func TestService_ListCapabilities_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// List database capabilities
	t.Log("Database Capabilities:")
	capResp, err := testOVHClient.Do(ctx, ovhtransport.RequestOptions{
		Method: "GET",
		Path:   fmt.Sprintf("/cloud/project/%s/database/capabilities", testutil.OVHCloudProjectID),
	})
	if err != nil {
		t.Fatalf("Failed to get capabilities: %v", err)
	}

	// Show engines
	if engines, ok := capResp.Body["engines"].([]interface{}); ok {
		t.Log("\nAvailable Engines:")
		for _, e := range engines {
			if engine, ok := e.(map[string]interface{}); ok {
				name := engine["name"]
				t.Logf("  - %s", name)
				if versions, ok := engine["versions"].([]interface{}); ok {
					for _, v := range versions {
						if version, ok := v.(map[string]interface{}); ok {
							t.Logf("      version: %s (status: %s)", version["version"], version["status"])
						}
					}
				}
			}
		}
	}

	// Show plans
	if plans, ok := capResp.Body["plans"].([]interface{}); ok {
		t.Log("\nAvailable Plans:")
		for _, p := range plans {
			if plan, ok := p.(map[string]interface{}); ok {
				t.Logf("  - %s: %s", plan["name"], plan["description"])
			}
		}
	}

	// Show flavors (first 10)
	if flavors, ok := capResp.Body["flavors"].([]interface{}); ok {
		t.Log("\nAvailable Flavors (first 10):")
		count := 0
		for _, f := range flavors {
			if count >= 10 {
				break
			}
			if flavor, ok := f.(map[string]interface{}); ok {
				t.Logf("  - %s: %v vCPU, %v MB RAM", flavor["name"], flavor["core"], flavor["memory"])
				count++
			}
		}
	}

	t.Log("\nSet OVH_TEST_DB_SERVICE_ENGINE, OVH_TEST_DB_SERVICE_VERSION, OVH_TEST_DB_SERVICE_PLAN,")
	t.Log("OVH_TEST_DB_SERVICE_FLAVOR, and OVH_TEST_DB_SERVICE_REGION in .env with valid values from above")
}

// TestService_Create_Read_Integration tests creating a database service and reading it back.
// WARNING: This test creates an actual database cluster which may incur costs.
// The cluster takes several minutes to provision.
func TestService_Create_Read_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	serviceName := fmt.Sprintf("formae-test-svc-%d", time.Now().Unix())

	// Create a database service
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"engine":      testDBEngine,
		"version":     testDBVersion,
		"plan":        testDBPlan,
		"description": serviceName,
		"nodesPattern": map[string]interface{}{
			"flavor": testDBFlavor,
			"number": 1,
			"region": testDBRegion,
		},
	})

	t.Logf("Creating database service: %s (engine=%s, version=%s, plan=%s, flavor=%s, region=%s)",
		serviceName, testDBEngine, testDBVersion,
		testDBPlan, testDBFlavor, testDBRegion)

	createResult, err := testServiceProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ServiceResourceType,
		Label:        serviceName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Service created with NativeID: %s", nativeID)

	// Clean up after test
	defer func() {
		t.Log("Cleaning up test service...")
		deleteReq := &resource.DeleteRequest{
			ResourceType: ServiceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testServiceProvisioner.Delete(ctx, deleteReq)
		// Wait for delete to complete
		_ = testutil.WaitForDeleteComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, ServiceResourceType)
		t.Logf("✓ Cleaned up test service: %s", nativeID)
	}()

	// Service creation is async - returns InProgress
	assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus,
		"Service creation should return InProgress")

	// Wait for service to be READY (this can take 5-15 minutes)
	t.Log("Waiting for service to be READY (this may take several minutes)...")
	pollConfig := testutil.NewPollConfig().
		ForLongRunningCreate(). // 200 attempts, 6s interval = ~20 min max
		WithResourceType(ServiceResourceType).
		Build()

	_, err = testutil.PollUntilComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Service should become READY")

	// Now test Read
	readReq := &resource.ReadRequest{
		ResourceType: ServiceResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testServiceProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")
	assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

	// Verify the properties contain expected fields
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, serviceName, props["description"], "Service description should match")
	assert.NotEmpty(t, props["id"], "Service should have an ID")
	assert.Equal(t, "READY", props["status"], "Service status should be READY")
	assert.Equal(t, testDBEngine, props["engine"], "Engine should match")

	t.Logf("✓ Created and read service: %s", nativeID)
}

func TestService_Read_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// Use a non-existent service ID
	nonExistentNativeID := fmt.Sprintf("%s/%s/00000000-0000-0000-0000-000000000000",
		testutil.OVHCloudProjectID, testDBEngine)

	readReq := &resource.ReadRequest{
		ResourceType: ServiceResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testServiceProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")

	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"ErrorCode should be NotFound for non-existent service")

	t.Logf("✓ Read of non-existent service returned NotFound")
}

func TestService_Update_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	serviceName := fmt.Sprintf("formae-test-svc-update-%d", time.Now().Unix())

	// First create a service
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"engine":      testDBEngine,
		"version":     testDBVersion,
		"plan":        testDBPlan,
		"description": serviceName,
		"nodesPattern": map[string]interface{}{
			"flavor": testDBFlavor,
			"number": 1,
			"region": testDBRegion,
		},
	})

	t.Logf("Creating database service for update test: %s", serviceName)

	createResult, err := testServiceProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ServiceResourceType,
		Label:        serviceName,
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
		t.Log("Cleaning up test service...")
		deleteReq := &resource.DeleteRequest{
			ResourceType: ServiceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testServiceProvisioner.Delete(ctx, deleteReq)
		_ = testutil.WaitForDeleteComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, ServiceResourceType)
		t.Logf("✓ Cleaned up test service: %s", nativeID)
	}()

	// Wait for service to be READY before updating
	t.Log("Waiting for service to be READY...")
	pollConfig := testutil.NewPollConfig().
		ForLongRunningCreate().
		WithResourceType(ServiceResourceType).
		Build()

	_, err = testutil.PollUntilComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Service should become READY")

	// Now test Update (update description)
	updatedDescription := serviceName + "-updated"
	updateProps, _ := json.Marshal(map[string]interface{}{
		"description": updatedDescription,
	})

	updateReq := &resource.UpdateRequest{
		ResourceType:      ServiceResourceType,
		NativeID:          nativeID,
		Label:             updatedDescription,
		DesiredProperties: updateProps,
		TargetConfig:      testTargetConfig,
	}

	updateResult, err := testServiceProvisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "UpdateResult should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus,
		"Update operation should succeed, got: %s - %s",
		updateResult.ProgressResult.OperationStatus, updateResult.ProgressResult.StatusMessage)

	// Verify the update by reading back
	readResult, err := testServiceProvisioner.Read(ctx, &resource.ReadRequest{
		ResourceType: ServiceResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, updatedDescription, props["description"], "Description should be updated")

	t.Logf("✓ Updated service: %s", nativeID)
}

func TestService_Delete_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	serviceName := fmt.Sprintf("formae-test-svc-delete-%d", time.Now().Unix())

	// First create a service
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"engine":      testDBEngine,
		"version":     testDBVersion,
		"plan":        testDBPlan,
		"description": serviceName,
		"nodesPattern": map[string]interface{}{
			"flavor": testDBFlavor,
			"number": 1,
			"region": testDBRegion,
		},
	})

	t.Logf("Creating database service for delete test: %s", serviceName)

	createResult, err := testServiceProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ServiceResourceType,
		Label:        serviceName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Wait for service to be READY before deleting
	t.Log("Waiting for service to be READY...")
	pollConfig := testutil.NewPollConfig().
		ForLongRunningCreate().
		WithResourceType(ServiceResourceType).
		Build()

	_, err = testutil.PollUntilComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Service should become READY")

	// Now test Delete
	deleteReq := &resource.DeleteRequest{
		ResourceType: ServiceResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testServiceProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete operation should succeed")

	// Wait for service to be fully deleted
	t.Log("Waiting for service to be fully deleted...")
	err = testutil.WaitForDeleteComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, ServiceResourceType)
	require.NoError(t, err, "Service should be fully deleted")

	t.Logf("✓ Deleted service: %s", nativeID)
}

func TestService_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	// Use a non-existent service ID
	nonExistentNativeID := fmt.Sprintf("%s/%s/00000000-0000-0000-0000-000000000000",
		testutil.OVHCloudProjectID, testDBEngine)

	deleteReq := &resource.DeleteRequest{
		ResourceType: ServiceResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testServiceProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	// Delete should be idempotent - 404 is treated as success
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent resource should succeed (idempotent)")

	t.Logf("✓ Delete of non-existent service returned success (idempotent)")
}

func TestService_List_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	serviceName := fmt.Sprintf("formae-test-svc-list-%d", time.Now().Unix())

	// First create a service
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"engine":      testDBEngine,
		"version":     testDBVersion,
		"plan":        testDBPlan,
		"description": serviceName,
		"nodesPattern": map[string]interface{}{
			"flavor": testDBFlavor,
			"number": 1,
			"region": testDBRegion,
		},
	})

	t.Logf("Creating database service for list test: %s", serviceName)

	createResult, err := testServiceProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ServiceResourceType,
		Label:        serviceName,
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
		t.Log("Cleaning up test service...")
		deleteReq := &resource.DeleteRequest{
			ResourceType: ServiceResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testServiceProvisioner.Delete(ctx, deleteReq)
		_ = testutil.WaitForDeleteComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, ServiceResourceType)
		t.Logf("✓ Cleaned up test service: %s", nativeID)
	}()

	// Wait for service to be READY
	t.Log("Waiting for service to be READY...")
	pollConfig := testutil.NewPollConfig().
		ForLongRunningCreate().
		WithResourceType(ServiceResourceType).
		Build()

	_, err = testutil.PollUntilComplete(t, ctx, testServiceProvisioner, nativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Service should become READY")

	// Now test List
	listReq := &resource.ListRequest{
		ResourceType: ServiceResourceType,
		TargetConfig: testTargetConfig,
		AdditionalProperties: map[string]string{
			"engine": testDBEngine,
		},
	}

	listResult, err := testServiceProvisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, listResult, "ListResult should not be nil")

	// The created service should be in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created service should be in the list. NativeID: %s, List: %v",
		nativeID, listResult.NativeIDs)

	t.Logf("✓ List returned %d services, including test service", len(listResult.NativeIDs))
}

// TestService_WithDatabases_Integration creates a database cluster and tests the
// OVH::Database::Database resource operations against it.
// This eliminates the need for a pre-existing cluster for database tests.
func TestService_WithDatabases_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	serviceName := fmt.Sprintf("formae-test-db-svc-%d", time.Now().Unix())

	// Create a database service/cluster to test databases against
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"engine":      testDBEngine,
		"version":     testDBVersion,
		"plan":        testDBPlan,
		"description": serviceName,
		"nodesPattern": map[string]interface{}{
			"flavor": testDBFlavor,
			"number": 1,
			"region": testDBRegion,
		},
	})

	t.Logf("Creating database service for database tests: %s (engine=%s)",
		serviceName, testDBEngine)

	createResult, err := testServiceProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ServiceResourceType,
		Label:        serviceName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	serviceNativeID := createResult.ProgressResult.NativeID
	t.Logf("Service created with NativeID: %s", serviceNativeID)

	// Clean up the service after all database tests
	defer func() {
		t.Log("Cleaning up test service...")
		deleteReq := &resource.DeleteRequest{
			ResourceType: ServiceResourceType,
			NativeID:     serviceNativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = testServiceProvisioner.Delete(ctx, deleteReq)
		_ = testutil.WaitForDeleteComplete(t, ctx, testServiceProvisioner, serviceNativeID, testTargetConfig, ServiceResourceType)
		t.Logf("✓ Cleaned up test service: %s", serviceNativeID)
	}()

	// Wait for service to be READY
	t.Log("Waiting for service to be READY (this may take several minutes)...")
	pollConfig := testutil.NewPollConfig().
		ForLongRunningCreate().
		WithResourceType(ServiceResourceType).
		Build()

	_, err = testutil.PollUntilComplete(t, ctx, testServiceProvisioner, serviceNativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Service should become READY")

	// Parse the service native ID to get engine and cluster ID
	// Format: projectId/engine/clusterId
	parts := strings.Split(serviceNativeID, "/")
	require.Len(t, parts, 3, "Service NativeID should have 3 parts")
	engine := parts[1]
	clusterID := parts[2]

	t.Logf("Running database tests against cluster: engine=%s, clusterId=%s", engine, clusterID)

	// --- Database Tests ---

	t.Run("Database_Create_Read", func(t *testing.T) {
		databaseName := fmt.Sprintf("formae_test_db_%d", time.Now().Unix())

		// Create a database within the test cluster
		dbCreateProps, _ := json.Marshal(map[string]interface{}{
			"name":      databaseName,
			"engine":    engine,
			"clusterId": clusterID,
		})

		dbCreateResult, err := testDatabaseProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: DatabaseResourceType,
			Label:        databaseName,
			Properties:   dbCreateProps,
			TargetConfig: testTargetConfig,
		})
		require.NoError(t, err)
		require.NotNil(t, dbCreateResult)
		require.NotNil(t, dbCreateResult.ProgressResult)
		require.NotEmpty(t, dbCreateResult.ProgressResult.NativeID)

		dbNativeID := dbCreateResult.ProgressResult.NativeID

		// Clean up the database after test
		defer func() {
			deleteReq := &resource.DeleteRequest{
				ResourceType: DatabaseResourceType,
				NativeID:     dbNativeID,
				TargetConfig: testTargetConfig,
			}
			_, _ = testDatabaseProvisioner.Delete(ctx, deleteReq)
			t.Logf("✓ Cleaned up test database: %s", dbNativeID)
		}()

		// Database creation is synchronous - should be Success immediately
		assert.Equal(t, resource.OperationStatusSuccess, dbCreateResult.ProgressResult.OperationStatus,
			"Database creation should succeed")

		// Test Read
		readReq := &resource.ReadRequest{
			ResourceType: DatabaseResourceType,
			NativeID:     dbNativeID,
			TargetConfig: testTargetConfig,
		}

		readResult, err := testDatabaseProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return an error")
		require.NotNil(t, readResult, "ReadResult should not be nil")
		assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
		assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

		// Verify the properties contain expected fields
		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal properties")

		assert.Equal(t, databaseName, props["name"], "Database name should match")

		t.Logf("✓ Created and read database: %s", dbNativeID)
	})

	t.Run("Database_Read_NotFound", func(t *testing.T) {
		// Use a non-existent database ID with valid project/engine/cluster format
		nonExistentNativeID := fmt.Sprintf("%s/%s/%s/non_existent_db_00000000",
			testutil.OVHCloudProjectID, engine, clusterID)

		readReq := &resource.ReadRequest{
			ResourceType: DatabaseResourceType,
			NativeID:     nonExistentNativeID,
			TargetConfig: testTargetConfig,
		}

		readResult, err := testDatabaseProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return an error")
		require.NotNil(t, readResult, "ReadResult should not be nil")

		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"ErrorCode should be NotFound for non-existent database")

		t.Logf("✓ Read of non-existent database returned NotFound")
	})

	t.Run("Database_Delete", func(t *testing.T) {
		databaseName := fmt.Sprintf("formae_test_db_del_%d", time.Now().Unix())

		// First create a database
		dbCreateProps, _ := json.Marshal(map[string]interface{}{
			"name":      databaseName,
			"engine":    engine,
			"clusterId": clusterID,
		})

		dbCreateResult, err := testDatabaseProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: DatabaseResourceType,
			Label:        databaseName,
			Properties:   dbCreateProps,
			TargetConfig: testTargetConfig,
		})
		require.NoError(t, err)
		require.NotNil(t, dbCreateResult)
		require.NotNil(t, dbCreateResult.ProgressResult)
		require.NotEmpty(t, dbCreateResult.ProgressResult.NativeID)

		dbNativeID := dbCreateResult.ProgressResult.NativeID

		// Test Delete
		deleteReq := &resource.DeleteRequest{
			ResourceType: DatabaseResourceType,
			NativeID:     dbNativeID,
			TargetConfig: testTargetConfig,
		}

		deleteResult, err := testDatabaseProvisioner.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return an error")
		require.NotNil(t, deleteResult, "DeleteResult should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

		assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
			"Delete operation should succeed")

		// Verify database no longer exists
		readReq := &resource.ReadRequest{
			ResourceType: DatabaseResourceType,
			NativeID:     dbNativeID,
			TargetConfig: testTargetConfig,
		}

		readResult, err := testDatabaseProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error after delete")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Database should not exist after deletion")

		t.Logf("✓ Deleted database: %s", dbNativeID)
	})

	t.Run("Database_Delete_NotFound", func(t *testing.T) {
		// Use a non-existent database ID
		nonExistentNativeID := fmt.Sprintf("%s/%s/%s/non_existent_db_00000000",
			testutil.OVHCloudProjectID, engine, clusterID)

		deleteReq := &resource.DeleteRequest{
			ResourceType: DatabaseResourceType,
			NativeID:     nonExistentNativeID,
			TargetConfig: testTargetConfig,
		}

		deleteResult, err := testDatabaseProvisioner.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return an error")
		require.NotNil(t, deleteResult, "DeleteResult should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

		// Delete should be idempotent - 404 is treated as success
		assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
			"Delete of non-existent resource should succeed (idempotent)")

		t.Logf("✓ Delete of non-existent database returned success (idempotent)")
	})

	t.Run("Database_List", func(t *testing.T) {
		databaseName := fmt.Sprintf("formae_test_db_list_%d", time.Now().Unix())

		// First create a database
		dbCreateProps, _ := json.Marshal(map[string]interface{}{
			"name":      databaseName,
			"engine":    engine,
			"clusterId": clusterID,
		})

		dbCreateResult, err := testDatabaseProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: DatabaseResourceType,
			Label:        databaseName,
			Properties:   dbCreateProps,
			TargetConfig: testTargetConfig,
		})
		require.NoError(t, err)
		require.NotNil(t, dbCreateResult)
		require.NotNil(t, dbCreateResult.ProgressResult)
		require.NotEmpty(t, dbCreateResult.ProgressResult.NativeID)

		dbNativeID := dbCreateResult.ProgressResult.NativeID

		// Clean up after test
		defer func() {
			deleteReq := &resource.DeleteRequest{
				ResourceType: DatabaseResourceType,
				NativeID:     dbNativeID,
				TargetConfig: testTargetConfig,
			}
			_, _ = testDatabaseProvisioner.Delete(ctx, deleteReq)
			t.Logf("✓ Cleaned up test database: %s", dbNativeID)
		}()

		// Test List
		listReq := &resource.ListRequest{
			ResourceType: DatabaseResourceType,
			TargetConfig: testTargetConfig,
			AdditionalProperties: map[string]string{
				"engine":    engine,
				"clusterId": clusterID,
			},
		}

		listResult, err := testDatabaseProvisioner.List(ctx, listReq)
		require.NoError(t, err, "List should not return an error")
		require.NotNil(t, listResult, "ListResult should not be nil")

		// The created database should be in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == dbNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Created database should be in the list. NativeID: %s, List: %v",
			dbNativeID, listResult.NativeIDs)

		t.Logf("✓ List returned %d databases, including test database", len(listResult.NativeIDs))
	})
}
