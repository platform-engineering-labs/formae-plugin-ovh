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
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SecurityGroupRule Tests
// ============================================================================

// createTestSecurityGroup creates a security group for testing and returns it.
// Caller is responsible for cleanup.
func createTestSecurityGroup(t *testing.T, ctx context.Context, name string) *groups.SecGroup {
	createOpts := groups.CreateOpts{
		Name:        name,
		Description: "Test security group for rule tests",
	}
	sg, err := groups.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group")
	return sg
}

// cleanupSecurityGroup deletes a security group (and all its rules).
func cleanupSecurityGroup(ctx context.Context, sgID string) {
	_ = groups.Delete(ctx, networkClient, sgID).ExtractErr()
}

func TestSecurityGroupRule_Create_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a parent security group first
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-for-rule-%d", timestamp)
	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare resource properties for an SSH rule
	properties := []byte(fmt.Sprintf(`{
		"security_group_id": "%s",
		"direction": "ingress",
		"ethertype": "IPv4",
		"protocol": "tcp",
		"port_range_min": 22,
		"port_range_max": 22,
		"remote_ip_prefix": "0.0.0.0/0",
		"description": "Allow SSH from anywhere"
	}`, sg.ID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		Label:        "test-ssh-rule",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := ruleProvisioner.Create(ctx, req)

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify rule was actually created in OpenStack
	rule, err := rules.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get security group rule from OpenStack")
	assert.Equal(t, sg.ID, rule.SecGroupID)
	assert.Equal(t, "ingress", rule.Direction)
	assert.Equal(t, "IPv4", rule.EtherType)
	assert.Equal(t, "tcp", rule.Protocol)
	assert.Equal(t, 22, rule.PortRangeMin)
	assert.Equal(t, 22, rule.PortRangeMax)
	assert.Equal(t, "0.0.0.0/0", rule.RemoteIPPrefix)

	t.Logf("✓ SecurityGroupRule created successfully:")
	t.Logf("  ID: %s", rule.ID)
	t.Logf("  Direction: %s", rule.Direction)
	t.Logf("  Protocol: %s", rule.Protocol)
	t.Logf("  Port: %d-%d", rule.PortRangeMin, rule.PortRangeMax)
}

func TestSecurityGroupRule_Read_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a parent security group first
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-for-rule-read-%d", timestamp)
	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	// Create a rule using gophercloud directly
	createOpts := rules.CreateOpts{
		SecGroupID:     sg.ID,
		Direction:      rules.DirIngress,
		EtherType:      rules.EtherType4,
		Protocol:       rules.ProtocolTCP,
		PortRangeMin:   443,
		PortRangeMax:   443,
		RemoteIPPrefix: "10.0.0.0/8",
		Description:    "Test rule for read test",
	}
	rule, err := rules.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group rule")

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Read request
	req := &resource.ReadRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		NativeID:     rule.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Read operation
	result, err := ruleProvisioner.Read(ctx, req)

	// Assert results
	require.NoError(t, err, "Read should not return an error")
	require.NotNil(t, result, "Read result should not be nil")

	assert.NotEmpty(t, result.Properties, "Properties should not be empty")
	assert.Empty(t, result.ErrorCode, "ErrorCode should be empty on success")

	// Parse and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, rule.ID, props["id"])
	assert.Equal(t, sg.ID, props["security_group_id"])
	assert.Equal(t, "ingress", props["direction"])
	assert.Equal(t, "IPv4", props["ethertype"])
	assert.Equal(t, "tcp", props["protocol"])
	assert.Equal(t, float64(443), props["port_range_min"])
	assert.Equal(t, float64(443), props["port_range_max"])
	assert.Equal(t, "10.0.0.0/8", props["remote_ip_prefix"])

	t.Logf("✓ SecurityGroupRule read successfully:")
	t.Logf("  ID: %s", props["id"])
	t.Logf("  Direction: %s", props["direction"])
	t.Logf("  Protocol: %s", props["protocol"])
}

func TestSecurityGroupRule_Update_ReturnsError_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a parent security group first
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-for-rule-update-%d", timestamp)
	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	// Create a rule using gophercloud directly
	createOpts := rules.CreateOpts{
		SecGroupID:   sg.ID,
		Direction:    rules.DirIngress,
		EtherType:    rules.EtherType4,
		Protocol:     rules.ProtocolTCP,
		PortRangeMin: 80,
		PortRangeMax: 80,
	}
	rule, err := rules.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group rule")

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Try to update the rule (should fail - rules are immutable)
	updatedProperties := []byte(fmt.Sprintf(`{
		"security_group_id": "%s",
		"direction": "ingress",
		"ethertype": "IPv4",
		"protocol": "tcp",
		"port_range_min": 8080,
		"port_range_max": 8080,
		"description": "Updated description"
	}`, sg.ID))

	// Create Update request
	req := &resource.UpdateRequest{
		ResourceType:      ResourceTypeSecurityGroupRule,
		NativeID:          rule.ID,
		Label:             "test-rule",
		DesiredProperties: updatedProperties,
		TargetConfig:      testutil.TargetConfig,
	}

	// Execute plugin Update operation
	result, err := ruleProvisioner.Update(ctx, req)

	// Assert results - should fail because rules are immutable
	require.NoError(t, err, "Update should not return a Go error")
	require.NotNil(t, result, "Update result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationUpdate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus)
	assert.Equal(t, resource.OperationErrorCodeInvalidRequest, result.ProgressResult.ErrorCode)
	assert.Contains(t, result.ProgressResult.StatusMessage, "immutable")

	t.Logf("✓ Update correctly rejected (rules are immutable):")
	t.Logf("  ErrorCode: %s", result.ProgressResult.ErrorCode)
	t.Logf("  Message: %s", result.ProgressResult.StatusMessage)
}

func TestSecurityGroupRule_Delete_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a parent security group first
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-for-rule-delete-%d", timestamp)
	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	// Create a rule using gophercloud directly
	// Use specific protocol/port to avoid conflict with default egress rules
	createOpts := rules.CreateOpts{
		SecGroupID:   sg.ID,
		Direction:    rules.DirIngress,
		EtherType:    rules.EtherType4,
		Protocol:     rules.ProtocolTCP,
		PortRangeMin: 9999,
		PortRangeMax: 9999,
	}
	rule, err := rules.Create(ctx, networkClient, createOpts).Extract()
	require.NoError(t, err, "Failed to create test security group rule")

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		NativeID:     rule.ID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := ruleProvisioner.Delete(ctx, req)

	// Assert results
	require.NoError(t, err, "Delete should not return an error")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty on success")

	// Verify rule was actually deleted from OpenStack
	_, err = rules.Get(ctx, networkClient, rule.ID).Extract()
	assert.Error(t, err, "SecurityGroupRule should not exist after deletion")
	assert.Contains(t, err.Error(), "404", "Should get 404 error for deleted rule")

	t.Logf("✓ SecurityGroupRule deleted successfully:")
	t.Logf("  ID: %s", rule.ID)
}

func TestSecurityGroupRule_Delete_NotFound_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Use a non-existent rule ID
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create Delete request
	req := &resource.DeleteRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Delete operation
	result, err := ruleProvisioner.Delete(ctx, req)

	// Assert results - deletion should be idempotent (success even if not found)
	require.NoError(t, err, "Delete should not return an error for non-existent resource")
	require.NotNil(t, result, "Delete result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationDelete, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Empty(t, result.ProgressResult.ErrorCode, "ErrorCode should be empty for idempotent delete")

	t.Logf("✓ Idempotent delete test passed (resource already gone)")
}

func TestSecurityGroupRule_List_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create a parent security group first
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-for-rule-list-%d", timestamp)
	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	// Create a few test rules using gophercloud directly
	testRules := []rules.CreateOpts{
		{
			SecGroupID:   sg.ID,
			Direction:    rules.DirIngress,
			EtherType:    rules.EtherType4,
			Protocol:     rules.ProtocolTCP,
			PortRangeMin: 22,
			PortRangeMax: 22,
		},
		{
			SecGroupID:   sg.ID,
			Direction:    rules.DirIngress,
			EtherType:    rules.EtherType4,
			Protocol:     rules.ProtocolTCP,
			PortRangeMin: 80,
			PortRangeMax: 80,
		},
		{
			SecGroupID:   sg.ID,
			Direction:    rules.DirIngress,
			EtherType:    rules.EtherType4,
			Protocol:     rules.ProtocolTCP,
			PortRangeMin: 443,
			PortRangeMax: 443,
		},
	}

	createdRuleIDs := make([]string, 0, len(testRules))
	for _, opts := range testRules {
		rule, err := rules.Create(ctx, networkClient, opts).Extract()
		require.NoError(t, err, "Failed to create test security group rule")
		createdRuleIDs = append(createdRuleIDs, rule.ID)
	}

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Create List request
	req := &resource.ListRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin List operation
	result, err := ruleProvisioner.List(ctx, req)

	// Assert results
	require.NoError(t, err, "List should not return an error")
	require.NotNil(t, result, "List result should not be nil")

	assert.NotEmpty(t, result.NativeIDs, "NativeIDs should not be empty")

	// Verify our test rules are in the list
	foundCount := 0
	for _, nativeID := range result.NativeIDs {
		for _, createdID := range createdRuleIDs {
			if nativeID == createdID {
				foundCount++
				t.Logf("✓ Found test rule in list: %s", nativeID)
			}
		}
	}

	assert.Equal(t, len(createdRuleIDs), foundCount, "Should find all %d test rules", len(createdRuleIDs))

	t.Logf("✓ List operation successful:")
	t.Logf("  Total rules found: %d", len(result.NativeIDs))
	t.Logf("  Test rules found: %d/%d", foundCount, len(createdRuleIDs))
}

func TestSecurityGroupRule_Create_RemoteGroupID_Integration(t *testing.T) {
	testutil.SkipIfNotConfigured(t)
	ctx := context.Background()

	// Create two security groups - one source, one target
	timestamp := time.Now().Unix()
	sgName := fmt.Sprintf("formae-plugin-sdk-test-sg-rule-remote-%d", timestamp)
	remoteSgName := fmt.Sprintf("formae-plugin-sdk-test-sg-rule-remote-source-%d", timestamp)

	sg := createTestSecurityGroup(t, ctx, sgName)
	defer cleanupSecurityGroup(ctx, sg.ID)

	remoteSg := createTestSecurityGroup(t, ctx, remoteSgName)
	defer cleanupSecurityGroup(ctx, remoteSg.ID)

	// Create SecurityGroupRule provisioner
	ruleProvisioner := &SecurityGroupRule{
		Client: testClient,
		Config: testClient.Config,
	}

	// Prepare resource properties with remote_group_id instead of remote_ip_prefix
	properties := []byte(fmt.Sprintf(`{
		"security_group_id": "%s",
		"direction": "ingress",
		"ethertype": "IPv4",
		"protocol": "tcp",
		"port_range_min": 3306,
		"port_range_max": 3306,
		"remote_group_id": "%s",
		"description": "Allow MySQL from other security group"
	}`, sg.ID, remoteSg.ID))

	// Create request
	req := &resource.CreateRequest{
		ResourceType: ResourceTypeSecurityGroupRule,
		Label:        "test-mysql-rule",
		Properties:   properties,
		TargetConfig: testutil.TargetConfig,
	}

	// Execute plugin Create operation
	result, err := ruleProvisioner.Create(ctx, req)

	// Assert results
	require.NoError(t, err, "Create should not return an error")
	require.NotNil(t, result, "Create result should not be nil")
	require.NotNil(t, result.ProgressResult, "ProgressResult should not be nil")

	assert.Equal(t, resource.OperationCreate, result.ProgressResult.Operation)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.NotEmpty(t, result.ProgressResult.NativeID, "NativeID should be set")

	// Verify rule was created with remote_group_id
	rule, err := rules.Get(ctx, networkClient, result.ProgressResult.NativeID).Extract()
	require.NoError(t, err, "Should be able to get security group rule from OpenStack")
	assert.Equal(t, remoteSg.ID, rule.RemoteGroupID)
	assert.Empty(t, rule.RemoteIPPrefix, "RemoteIPPrefix should be empty when using RemoteGroupID")

	t.Logf("✓ SecurityGroupRule with remote_group_id created successfully:")
	t.Logf("  ID: %s", rule.ID)
	t.Logf("  RemoteGroupID: %s", rule.RemoteGroupID)
}
