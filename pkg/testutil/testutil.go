// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/require"
)

var (
	// OVH REST API configuration - read from environment variables
	OVHEndpoint          = getEnvOrDefault("OVH_ENDPOINT", "ovh-eu")
	OVHApplicationKey    = os.Getenv("OVH_APPLICATION_KEY")
	OVHApplicationSecret = os.Getenv("OVH_APPLICATION_SECRET")
	OVHConsumerKey       = os.Getenv("OVH_CONSUMER_KEY")
	OVHCloudProjectID    = os.Getenv("OVH_CLOUD_PROJECT_ID")

	// OVHConfig returns the OVH REST API client configuration
	OVHConfig = &ovhtransport.OVHConfig{
		Endpoint:          OVHEndpoint,
		ApplicationKey:    OVHApplicationKey,
		ApplicationSecret: OVHApplicationSecret,
		ConsumerKey:       OVHConsumerKey,
	}

	// Region for testing (e.g., DE1, GRA9, UK1, BHS5)
	// Note: GRA11 only has GPU flavors - use DE1/GRA9 for standard compute
	Region = getEnvOrDefault("OS_REGION_NAME", "DE1")

	// Compute test configuration for DE1 region
	// TestFlavorID - Small flavor for testing (OVH b2-7: 2 vCPU, 7GB RAM)
	TestFlavorID = getEnvOrDefault("OS_TEST_FLAVOR_ID", "3be7c73a-735a-4ee1-b8d4-83feb080109d") // b2-7 in DE1
	// TestImageID - Image for testing (AlmaLinux 9)
	TestImageID = getEnvOrDefault("OS_TEST_IMAGE_ID", "720fbd6e-6edb-4983-bfc6-dfc22ab23656") // AlmaLinux 9 in DE1

	// Database cluster test configuration (for nested resources that need an existing cluster)
	// Set these environment variables with an existing database cluster for testing
	// Get cluster IDs from: GET /cloud/project/{serviceName}/database/{engine}
	TestDatabaseEngine    = getEnvOrDefault("OVH_TEST_DATABASE_ENGINE", "")    // e.g., "mysql", "postgresql"
	TestDatabaseClusterID = getEnvOrDefault("OVH_TEST_DATABASE_CLUSTER_ID", "") // cluster UUID

	// Database service creation test configuration
	// These are used when creating new database services in tests
	// Run TestService_ListCapabilities_Integration to see available options for your region
	TestDBServiceEngine  = getEnvOrDefault("OVH_TEST_DB_SERVICE_ENGINE", "mysql")     // Engine to test
	TestDBServiceVersion = getEnvOrDefault("OVH_TEST_DB_SERVICE_VERSION", "8")        // Engine version
	TestDBServicePlan    = getEnvOrDefault("OVH_TEST_DB_SERVICE_PLAN", "essential")   // Plan tier
	TestDBServiceFlavor  = getEnvOrDefault("OVH_TEST_DB_SERVICE_FLAVOR", "db1-4")     // Smallest flavor
	TestDBServiceRegion  = getEnvOrDefault("OVH_TEST_DB_SERVICE_REGION", "DE")        // Region for nodes
)

// getEnvOrDefault returns the environment variable value or the default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsOVHConfigured returns true if the required OVH REST API environment variables are set
func IsOVHConfigured() bool {
	return OVHApplicationKey != "" && OVHApplicationSecret != "" && OVHConsumerKey != "" && OVHCloudProjectID != ""
}

// SkipIfOVHNotConfigured skips the test if required OVH REST API environment variables are not set
func SkipIfOVHNotConfigured(t interface{ Skip(...any) }) {
	if !IsOVHConfigured() {
		t.Skip("Skipping test: OVH REST API credentials not configured. Set OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET, OVH_CONSUMER_KEY, and OVH_CLOUD_PROJECT_ID environment variables.")
	}
}

// IsDatabaseConfigured returns true if database cluster test configuration is set
func IsDatabaseConfigured() bool {
	return IsOVHConfigured() && TestDatabaseEngine != "" && TestDatabaseClusterID != ""
}

// SkipIfDatabaseNotConfigured skips the test if database cluster configuration is not set
func SkipIfDatabaseNotConfigured(t interface{ Skip(...any) }) {
	if !IsDatabaseConfigured() {
		t.Skip("Skipping test: Database cluster not configured. Set OVH_TEST_DATABASE_ENGINE and OVH_TEST_DATABASE_CLUSTER_ID environment variables.")
	}
}

// NewOVHClient creates a new OVH REST API client from environment configuration
func NewOVHClient() (*ovhtransport.Client, error) {
	return ovhtransport.NewClient(OVHConfig)
}

// StatusChecker defines the interface for checking operation status
type StatusChecker interface {
	Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error)
}

// PollConfig configures the polling behavior
type PollConfig struct {
	MaxAttempts   int
	CheckInterval time.Duration
	ResourceType  string
	OperationName string // "Create", "Delete", "Update" for better logging
}

// DefaultPollConfig returns sensible defaults for polling
func DefaultPollConfig() PollConfig {
	return PollConfig{
		MaxAttempts:   100,
		CheckInterval: 5 * time.Second,
		OperationName: "Operation",
	}
}

// PollConfigBuilder provides a fluent API for building PollConfig
type PollConfigBuilder struct {
	config PollConfig
}

// NewPollConfig creates a new PollConfigBuilder with defaults
func NewPollConfig() *PollConfigBuilder {
	return &PollConfigBuilder{
		config: DefaultPollConfig(),
	}
}

// WithMaxAttempts sets the maximum number of polling attempts
func (b *PollConfigBuilder) WithMaxAttempts(attempts int) *PollConfigBuilder {
	b.config.MaxAttempts = attempts
	return b
}

// WithCheckInterval sets the interval between polling attempts
func (b *PollConfigBuilder) WithCheckInterval(interval time.Duration) *PollConfigBuilder {
	b.config.CheckInterval = interval
	return b
}

// WithResourceType sets the resource type
func (b *PollConfigBuilder) WithResourceType(resourceType string) *PollConfigBuilder {
	b.config.ResourceType = resourceType
	return b
}

// WithOperationName sets the operation name for logging
func (b *PollConfigBuilder) WithOperationName(name string) *PollConfigBuilder {
	b.config.OperationName = name
	return b
}

// ForCreate configures for a create operation (default settings)
func (b *PollConfigBuilder) ForCreate() *PollConfigBuilder {
	b.config.OperationName = "Create"
	return b
}

// ForDelete configures for a delete operation (default settings)
func (b *PollConfigBuilder) ForDelete() *PollConfigBuilder {
	b.config.OperationName = "Delete"
	return b
}

// ForUpdate configures for an update operation (default settings)
func (b *PollConfigBuilder) ForUpdate() *PollConfigBuilder {
	b.config.OperationName = "Update"
	return b
}

// ForLongRunningCreate configures for long-running create operations (e.g., instances)
func (b *PollConfigBuilder) ForLongRunningCreate() *PollConfigBuilder {
	b.config.OperationName = "Create"
	b.config.MaxAttempts = 200 // ~20 minutes with 6s intervals
	b.config.CheckInterval = 6 * time.Second
	return b
}

// ForLongRunningDelete configures for long-running delete operations
func (b *PollConfigBuilder) ForLongRunningDelete() *PollConfigBuilder {
	b.config.OperationName = "Delete"
	b.config.MaxAttempts = 200 // ~20 minutes with 6s intervals
	b.config.CheckInterval = 6 * time.Second
	return b
}

// Build returns the final PollConfig
func (b *PollConfigBuilder) Build() PollConfig {
	return b.config
}

// PollUntilComplete polls the status until the operation completes or times out
func PollUntilComplete(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	nativeID string,
	targetConfig json.RawMessage,
	config PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if config.MaxAttempts == 0 {
		config.MaxAttempts = 30
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 2 * time.Second
	}

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		time.Sleep(config.CheckInterval)

		statusReq := &resource.StatusRequest{
			NativeID:     nativeID,
			ResourceType: config.ResourceType,
			TargetConfig: targetConfig,
		}

		statusResult, err := checker.Status(ctx, statusReq)
		require.NoError(t, err, "%s status check should not return error", config.OperationName)
		require.NotNil(t, statusResult, "%s status result should not be nil", config.OperationName)
		require.NotNil(t, statusResult.ProgressResult, "%s progress result should not be nil", config.OperationName)

		t.Logf("%s status check attempt %d/%d: %s (status: %s)",
			config.OperationName,
			attempt+1,
			config.MaxAttempts,
			statusResult.ProgressResult.StatusMessage,
			statusResult.ProgressResult.OperationStatus)

		switch statusResult.ProgressResult.OperationStatus {
		case resource.OperationStatusSuccess:
			t.Logf("%s completed successfully with native ID: %s",
				config.OperationName,
				statusResult.ProgressResult.NativeID)
			return statusResult, nil

		case resource.OperationStatusFailure:
			return statusResult, fmt.Errorf("%s operation failed: %s (error code: %s)",
				config.OperationName,
				statusResult.ProgressResult.StatusMessage,
				statusResult.ProgressResult.ErrorCode)

		case resource.OperationStatusInProgress:
			// Continue polling
			if attempt == config.MaxAttempts-1 {
				return statusResult, fmt.Errorf("%s operation timed out after %d attempts",
					config.OperationName,
					config.MaxAttempts)
			}
		}
	}

	return nil, fmt.Errorf("%s operation timed out", config.OperationName)
}

// WaitForCreate is a convenience wrapper for Create operations
func WaitForCreate(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	createResult *resource.CreateResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForCreate().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, createResult.ProgressResult.NativeID, targetConfig, config)
}

// WaitForCreateWithConfig is a convenience wrapper with custom config
func WaitForCreateWithConfig(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	createResult *resource.CreateResult,
	targetConfig json.RawMessage,
	resourceType string,
	pollConfig PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if pollConfig.ResourceType == "" {
		pollConfig.ResourceType = resourceType
	}
	if pollConfig.OperationName == "" {
		pollConfig.OperationName = "Create"
	}

	return PollUntilComplete(t, ctx, checker, createResult.ProgressResult.NativeID, targetConfig, pollConfig)
}

// WaitForDelete is a convenience wrapper for Delete operations
func WaitForDelete(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	deleteResult *resource.DeleteResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForDelete().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, deleteResult.ProgressResult.NativeID, targetConfig, config)
}

// WaitForUpdate is a convenience wrapper for Update operations
func WaitForUpdate(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	updateResult *resource.UpdateResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForUpdate().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, updateResult.ProgressResult.NativeID, targetConfig, config)
}

// Reader defines the interface for reading resources
type Reader interface {
	Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error)
}

// WaitForDeleteComplete polls until a Read returns NotFound (resource fully deleted)
func WaitForDeleteComplete(
	t *testing.T,
	ctx context.Context,
	reader Reader,
	nativeID string,
	targetConfig json.RawMessage,
	resourceType string,
) error {
	t.Helper()

	config := DefaultPollConfig()

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		time.Sleep(config.CheckInterval)

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			ResourceType: resourceType,
			TargetConfig: targetConfig,
		}

		readResult, err := reader.Read(ctx, readReq)
		if err != nil {
			// API error - might be deleted
			t.Logf("Delete check attempt %d/%d: error (may be deleted): %v", attempt+1, config.MaxAttempts, err)
			return nil
		}

		if readResult.ErrorCode == resource.OperationErrorCodeNotFound {
			t.Logf("Delete completed - resource no longer exists")
			return nil
		}

		t.Logf("Delete check attempt %d/%d: resource still exists (status may be DELETING)", attempt+1, config.MaxAttempts)
	}

	return fmt.Errorf("resource still exists after %d attempts", config.MaxAttempts)
}
