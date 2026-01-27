// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package database

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/testutil"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// Shared test variables for all database package tests
var (
	testOVHClient   *ovhtransport.Client
	testTargetConfig json.RawMessage

	// Provisioners for each resource type
	testDatabaseProvisioner prov.Provisioner
	testServiceProvisioner  prov.Provisioner
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

	// Initialize Database provisioner
	if factory, ok := registry.GetOVHFactory(DatabaseResourceType); ok {
		testDatabaseProvisioner = factory(testOVHClient)
	} else {
		fmt.Printf("Database resource type not registered: %s\n", DatabaseResourceType)
		os.Exit(1)
	}

	// Initialize Service provisioner
	if factory, ok := registry.GetOVHFactory(ServiceResourceType); ok {
		testServiceProvisioner = factory(testOVHClient)
	} else {
		fmt.Printf("Service resource type not registered: %s\n", ServiceResourceType)
		os.Exit(1)
	}

	// Build target config with project ID
	testTargetConfig, _ = json.Marshal(map[string]interface{}{
		"projectId": testutil.OVHCloudProjectID,
	})

	os.Exit(m.Run())
}
