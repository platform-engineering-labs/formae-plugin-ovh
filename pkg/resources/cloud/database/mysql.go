// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	MySQLResourceType              = "OVH::Database::MySQL"
	MySQLUserResourceType          = "OVH::Database::MySQLUser"
	MySQLDatabaseResourceType      = "OVH::Database::MySQLDatabase"
	MySQLIpRestrictionResourceType = "OVH::Database::MySQLIpRestriction"
	MySQLIntegrationResourceType   = "OVH::Database::MySQLIntegration"
)

const mysqlEngine = "mysql"

func init() {
	registry.Register(
		MySQLResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newEngineClusterProvisioner(client, mysqlEngine)
		},
	)

	registry.Register(
		MySQLDatabaseResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newEngineNestedProvisioner(client, EngineNestedConfig{
				Engine:         mysqlEngine,
				PathSegment:    "database",
				SupportsUpdate: false,
			})
		},
	)

	registry.Register(
		MySQLUserResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newEngineNestedProvisioner(client, EngineNestedConfig{
				Engine:         mysqlEngine,
				PathSegment:    "user",
				SupportsUpdate: false,
			})
		},
	)

	registry.Register(
		MySQLIpRestrictionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newEngineNestedProvisioner(client, EngineNestedConfig{
				Engine:         mysqlEngine,
				PathSegment:    "ipRestriction",
				IDField:        "ip",
				SupportsUpdate: true,
			})
		},
	)

	registry.Register(
		MySQLIntegrationResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newEngineNestedProvisioner(client, EngineNestedConfig{
				Engine:         mysqlEngine,
				PathSegment:    "integration",
				SupportsUpdate: false,
			})
		},
	)
}
