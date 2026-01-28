// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package kube

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

// Kubernetes clusters use OpenStack regions (DE1, GRA9, etc.)
const testKubeRegion = "DE1"

var (
	testOVHClient         *ovhtransport.Client
	testClusterProvisioner prov.Provisioner
	testTargetConfig      json.RawMessage
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

	// Get the Cluster provisioner factory from registry
	clusterFactory, ok := registry.GetOVHFactory(ClusterResourceType)
	if !ok {
		fmt.Printf("Cluster resource type not registered: %s\n", ClusterResourceType)
		os.Exit(1)
	}
	testClusterProvisioner = clusterFactory(testOVHClient)

	// Build target config with project ID
	testTargetConfig, _ = json.Marshal(map[string]interface{}{
		"projectId": testutil.OVHCloudProjectID,
		"region":    testKubeRegion,
	})

	os.Exit(m.Run())
}

// TestCluster_Create_Read_Delete_Integration tests the full lifecycle of a Kubernetes cluster.
// WARNING: This test creates a real Kubernetes cluster which takes ~10-15 minutes and incurs costs.
// Run with: go test -v -tags=integration -run TestCluster_Create_Read_Delete_Integration ./pkg/resources/cloud/kube/...
func TestCluster_Create_Read_Delete_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	// Skip by default - cluster creation is expensive and slow
	if os.Getenv("OVH_TEST_KUBE_CLUSTER") != "true" {
		t.Skip("Skipping Kubernetes cluster test. Set OVH_TEST_KUBE_CLUSTER=true to run.")
	}

	ctx := context.Background()
	clusterName := fmt.Sprintf("formae-test-%d", time.Now().Unix())

	// Create a cluster (minimal config - just region and name)
	createProps, _ := json.Marshal(map[string]interface{}{
		"region": testKubeRegion,
		"name":   clusterName,
	})

	t.Logf("Creating Kubernetes cluster: %s in region %s", clusterName, testKubeRegion)

	createResult, err := testClusterProvisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: ClusterResourceType,
		Label:        clusterName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Cluster creation initiated with native ID: %s", nativeID)

	// Clean up after test (regardless of success/failure)
	defer func() {
		t.Logf("Cleaning up cluster: %s", nativeID)
		deleteReq := &resource.DeleteRequest{
			ResourceType: ClusterResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		deleteResult, err := testClusterProvisioner.Delete(ctx, deleteReq)
		if err != nil {
			t.Logf("Warning: Delete returned error: %v", err)
		} else if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			t.Logf("Warning: Delete failed: %s", deleteResult.ProgressResult.StatusMessage)
		} else {
			t.Logf("Delete initiated for cluster: %s", nativeID)
		}
	}()

	// Poll until cluster is READY (can take 10-15 minutes)
	pollConfig := testutil.NewPollConfig().
		WithMaxAttempts(180). // 30 minutes with 10s interval
		WithCheckInterval(10 * time.Second).
		WithResourceType(ClusterResourceType).
		ForCreate().
		Build()

	statusResult, err := testutil.PollUntilComplete(t, ctx, testClusterProvisioner, nativeID, testTargetConfig, pollConfig)
	require.NoError(t, err, "Cluster should reach READY status")
	require.NotNil(t, statusResult)
	assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)

	t.Logf("Cluster is READY: %s", nativeID)

	// Test Read
	readReq := &resource.ReadRequest{
		ResourceType: ClusterResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testClusterProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")
	assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

	// Verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, clusterName, props["name"], "Cluster name should match")
	assert.Equal(t, testKubeRegion, props["region"], "Region should match")
	assert.Equal(t, "READY", props["status"], "Status should be READY")

	t.Logf("Read cluster: %s (version: %v)", nativeID, props["version"])
}

// TestCluster_List_Integration tests listing Kubernetes clusters.
func TestCluster_List_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()

	listReq := &resource.ListRequest{
		ResourceType: ClusterResourceType,
		TargetConfig: testTargetConfig,
		AdditionalProperties: map[string]string{
			"serviceName": testutil.OVHCloudProjectID,
		},
	}

	listResult, err := testClusterProvisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, listResult, "ListResult should not be nil")

	t.Logf("Found %d Kubernetes clusters in project", len(listResult.NativeIDs))
	for i, id := range listResult.NativeIDs {
		t.Logf("  [%d] %s", i+1, id)
	}
}

// TestCluster_Read_Existing_Integration tests reading an existing cluster.
// Set OVH_TEST_KUBE_CLUSTER_ID environment variable to test with an existing cluster.
func TestCluster_Read_Existing_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	existingClusterID := os.Getenv("OVH_TEST_KUBE_CLUSTER_ID")
	if existingClusterID == "" {
		t.Skip("Skipping: Set OVH_TEST_KUBE_CLUSTER_ID to test reading an existing cluster")
	}

	ctx := context.Background()
	nativeID := fmt.Sprintf("%s/%s", testutil.OVHCloudProjectID, existingClusterID)

	readReq := &resource.ReadRequest{
		ResourceType: ClusterResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := testClusterProvisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")

	if readResult.ErrorCode != "" {
		t.Fatalf("Read failed with error code: %s", readResult.ErrorCode)
	}

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	t.Logf("Cluster: %s", props["name"])
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Region: %s", props["region"])
	t.Logf("  Version: %s", props["version"])
	t.Logf("  Status: %s", props["status"])
	t.Logf("  Plan: %v", props["plan"])
}

// TestCluster_Update_Integration tests updating an existing cluster.
// Set OVH_TEST_KUBE_CLUSTER_ID environment variable to test with an existing cluster.
func TestCluster_Update_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	existingClusterID := os.Getenv("OVH_TEST_KUBE_CLUSTER_ID")
	if existingClusterID == "" {
		t.Skip("Skipping: Set OVH_TEST_KUBE_CLUSTER_ID to test updating an existing cluster")
	}

	ctx := context.Background()
	nativeID := fmt.Sprintf("%s/%s", testutil.OVHCloudProjectID, existingClusterID)

	// Read current state
	readReq := &resource.ReadRequest{
		ResourceType: ClusterResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}
	readResult, err := testClusterProvisioner.Read(ctx, readReq)
	require.NoError(t, err)
	require.Empty(t, readResult.ErrorCode)

	var currentProps map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &currentProps)
	require.NoError(t, err)

	currentName := currentProps["name"].(string)
	t.Logf("Current cluster name: %s", currentName)

	// Update name (append timestamp)
	newName := fmt.Sprintf("%s-updated-%d", currentName, time.Now().Unix()%1000)
	updateProps, _ := json.Marshal(map[string]interface{}{
		"name": newName,
	})

	updateReq := &resource.UpdateRequest{
		ResourceType:      ClusterResourceType,
		NativeID:          nativeID,
		Label:             currentName,
		DesiredProperties: updateProps,
		TargetConfig:      testTargetConfig,
	}

	updateResult, err := testClusterProvisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "UpdateResult should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus,
		"Update should succeed, got: %s - %s",
		updateResult.ProgressResult.OperationStatus, updateResult.ProgressResult.StatusMessage)

	t.Logf("Updated cluster name from %s to %s", currentName, newName)

	// Revert name back
	revertProps, _ := json.Marshal(map[string]interface{}{
		"name": currentName,
	})

	revertReq := &resource.UpdateRequest{
		ResourceType:      ClusterResourceType,
		NativeID:          nativeID,
		Label:             newName,
		DesiredProperties: revertProps,
		TargetConfig:      testTargetConfig,
	}

	revertResult, err := testClusterProvisioner.Update(ctx, revertReq)
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusSuccess, revertResult.ProgressResult.OperationStatus)

	t.Logf("Reverted cluster name back to %s", currentName)
}

// TestCluster_Delete_NotFound_Integration tests deleting a non-existent cluster.
func TestCluster_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	nonExistentNativeID := fmt.Sprintf("%s/non-existent-cluster-id", testutil.OVHCloudProjectID)

	deleteReq := &resource.DeleteRequest{
		ResourceType: ClusterResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := testClusterProvisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	// Delete should be idempotent - 404 is treated as success
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent resource should succeed (idempotent)")

	t.Logf("Delete of non-existent cluster returned success (idempotent)")
}

// TestCluster_Status_Integration tests the status check for an existing cluster.
// Set OVH_TEST_KUBE_CLUSTER_ID environment variable to test with an existing cluster.
func TestCluster_Status_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	existingClusterID := os.Getenv("OVH_TEST_KUBE_CLUSTER_ID")
	if existingClusterID == "" {
		t.Skip("Skipping: Set OVH_TEST_KUBE_CLUSTER_ID to test status of an existing cluster")
	}

	ctx := context.Background()
	nativeID := fmt.Sprintf("%s/%s", testutil.OVHCloudProjectID, existingClusterID)

	statusReq := &resource.StatusRequest{
		ResourceType: ClusterResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	statusResult, err := testClusterProvisioner.Status(ctx, statusReq)
	require.NoError(t, err, "Status should not return an error")
	require.NotNil(t, statusResult, "StatusResult should not be nil")
	require.NotNil(t, statusResult.ProgressResult, "ProgressResult should not be nil")

	t.Logf("Cluster status: %s", statusResult.ProgressResult.OperationStatus)
	if statusResult.ProgressResult.StatusMessage != "" {
		t.Logf("Status message: %s", statusResult.ProgressResult.StatusMessage)
	}

	if statusResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		t.Logf("Cluster is READY")
	}
}
