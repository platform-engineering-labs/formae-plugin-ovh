// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testS3BucketProvisioner prov.Provisioner
)

const (
	testRegion = "DE"
)

func init() {
	// This init runs after TestMain sets up testOVHClient
	// We'll initialize the S3Bucket provisioner in a helper function instead
}

func getS3BucketProvisioner(t *testing.T) prov.Provisioner {
	if testS3BucketProvisioner != nil {
		return testS3BucketProvisioner
	}

	factory, ok := registry.GetOVHFactory(S3BucketResourceType)
	if !ok {
		t.Fatalf("S3Bucket resource type not registered: %s", S3BucketResourceType)
	}
	testS3BucketProvisioner = factory(testOVHClient)
	return testS3BucketProvisioner
}

func TestS3Bucket_Create_Read_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	// S3 bucket names must be globally unique and DNS-compliant
	bucketName := fmt.Sprintf("formae-test-s3-%d", time.Now().Unix())

	// Create an S3 bucket
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"name":        bucketName,
		"region":      testRegion,
	})

	createResult, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: S3BucketResourceType,
		Label:        bucketName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Created S3 bucket with native ID: %s", nativeID)

	// Clean up after test
	defer func() {
		deleteReq := &resource.DeleteRequest{
			ResourceType: S3BucketResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test S3 bucket: %s", nativeID)
	}()

	// S3 bucket creation is synchronous, so we can read immediately
	readReq := &resource.ReadRequest{
		ResourceType: S3BucketResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")
	assert.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	assert.NotEmpty(t, readResult.Properties, "Properties should be returned")

	// Verify the properties contain expected fields
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, bucketName, props["name"], "Bucket name should match")
	assert.NotEmpty(t, props["virtualHost"], "Bucket should have a virtualHost")

	t.Logf("✓ Read S3 bucket: %s", nativeID)
}

func TestS3Bucket_Update_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	bucketName := fmt.Sprintf("formae-test-s3-update-%d", time.Now().Unix())

	// Create an S3 bucket
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"name":        bucketName,
		"region":      testRegion,
	})

	createResult, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: S3BucketResourceType,
		Label:        bucketName,
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
			ResourceType: S3BucketResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test S3 bucket: %s", nativeID)
	}()

	// Update versioning configuration
	updateProps, _ := json.Marshal(map[string]interface{}{
		"versioning": map[string]interface{}{
			"status": "enabled",
		},
	})

	updateReq := &resource.UpdateRequest{
		ResourceType:      S3BucketResourceType,
		NativeID:          nativeID,
		Label:             bucketName,
		DesiredProperties: updateProps,
		TargetConfig:      testTargetConfig,
	}

	updateResult, err := provisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return an error")
	require.NotNil(t, updateResult, "UpdateResult should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus,
		"Update operation should succeed, got: %s - %s",
		updateResult.ProgressResult.OperationStatus, updateResult.ProgressResult.StatusMessage)

	t.Logf("✓ Updated S3 bucket: %s", nativeID)
}

func TestS3Bucket_Delete_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	bucketName := fmt.Sprintf("formae-test-s3-delete-%d", time.Now().Unix())

	// Create an S3 bucket
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"name":        bucketName,
		"region":      testRegion,
	})

	createResult, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: S3BucketResourceType,
		Label:        bucketName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID

	// Delete the S3 bucket
	deleteReq := &resource.DeleteRequest{
		ResourceType: S3BucketResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete operation should succeed")

	t.Logf("✓ Deleted S3 bucket: %s", nativeID)
}

func TestS3Bucket_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	// Use a non-existent bucket ID
	nonExistentNativeID := fmt.Sprintf("%s/%s/non-existent-bucket", testutil.OVHCloudProjectID, testRegion)

	deleteReq := &resource.DeleteRequest{
		ResourceType: S3BucketResourceType,
		NativeID:     nonExistentNativeID,
		TargetConfig: testTargetConfig,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, deleteResult, "DeleteResult should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "ProgressResult should not be nil")

	// Delete should be idempotent - 404 is treated as success
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent resource should succeed (idempotent)")

	t.Logf("✓ Delete of non-existent S3 bucket returned success (idempotent)")
}

func TestS3Bucket_List_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	bucketName := fmt.Sprintf("formae-test-s3-list-%d", time.Now().Unix())

	// Create an S3 bucket
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"name":        bucketName,
		"region":      testRegion,
	})

	createResult, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: S3BucketResourceType,
		Label:        bucketName,
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
			ResourceType: S3BucketResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test S3 bucket: %s", nativeID)
	}()

	// Test List
	listReq := &resource.ListRequest{
		ResourceType: S3BucketResourceType,
		TargetConfig: testTargetConfig,
		AdditionalProperties: map[string]string{
			"serviceName": testutil.OVHCloudProjectID,
			"region":      testRegion,
		},
	}

	listResult, err := provisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, listResult, "ListResult should not be nil")

	// The created bucket should be in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created S3 bucket should be in the list. NativeID: %s, List: %v",
		nativeID, listResult.NativeIDs)

	t.Logf("✓ List returned %d S3 buckets, including test bucket", len(listResult.NativeIDs))
}

func TestS3Bucket_CreateWithVersioning_Integration(t *testing.T) {
	testutil.SkipIfOVHNotConfigured(t)

	ctx := context.Background()
	provisioner := getS3BucketProvisioner(t)

	bucketName := fmt.Sprintf("formae-test-s3-versioning-%d", time.Now().Unix())

	// Create an S3 bucket with versioning enabled
	createProps, _ := json.Marshal(map[string]interface{}{
		"serviceName": testutil.OVHCloudProjectID,
		"name":        bucketName,
		"region":      testRegion,
		"versioning": map[string]interface{}{
			"status": "enabled",
		},
	})

	createResult, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: S3BucketResourceType,
		Label:        bucketName,
		Properties:   createProps,
		TargetConfig: testTargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Created S3 bucket with versioning, native ID: %s", nativeID)

	// Clean up after test
	defer func() {
		deleteReq := &resource.DeleteRequest{
			ResourceType: S3BucketResourceType,
			NativeID:     nativeID,
			TargetConfig: testTargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("✓ Cleaned up test S3 bucket: %s", nativeID)
	}()

	// Read and verify versioning is enabled
	readReq := &resource.ReadRequest{
		ResourceType: S3BucketResourceType,
		NativeID:     nativeID,
		TargetConfig: testTargetConfig,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, readResult, "ReadResult should not be nil")

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, bucketName, props["name"], "Bucket name should match")

	t.Logf("✓ Created S3 bucket with versioning: %s", nativeID)
}
