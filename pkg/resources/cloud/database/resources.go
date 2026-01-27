// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package database

import (
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// Resource type constants for database resources.
const (
	DatabaseResourceType             = "OVH::Database::Database"
	UserResourceType                 = "OVH::Database::User"
	IntegrationResourceType          = "OVH::Database::Integration"
	IpRestrictionResourceType        = "OVH::Database::IpRestriction"
	KafkaAclResourceType             = "OVH::Database::KafkaAcl"
	KafkaTopicResourceType           = "OVH::Database::KafkaTopic"
	PostgresqlConnectionPoolResourceType = "OVH::Database::PostgresqlConnectionPool"
)

func init() {
	// Database (schema within a service)
	// POST /cloud/project/{serviceName}/database/{engine}/{clusterId}/database
	// No Update support
	registry.Register(
		DatabaseResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "database",
				SupportsUpdate: false,
			})
		},
	)

	// User
	// POST /cloud/project/{serviceName}/database/{engine}/{clusterId}/user
	// Supports Update (PUT) for roles
	registry.Register(
		UserResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "user",
				SupportsUpdate: true,
			})
		},
	)

	// Integration
	// POST /cloud/project/{serviceName}/database/{engine}/{clusterId}/integration
	// No Update support
	registry.Register(
		IntegrationResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "integration",
				SupportsUpdate: false,
			})
		},
	)

	// IpRestriction
	// POST /cloud/project/{serviceName}/database/{engine}/{clusterId}/ipRestriction
	// Identifier is "ip" not "id"
	// Supports Update (PUT)
	registry.Register(
		IpRestrictionResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "ipRestriction",
				IDField:        "ip",
				SupportsUpdate: true,
			})
		},
	)

	// KafkaAcl
	// POST /cloud/project/{serviceName}/database/kafka/{clusterId}/acl
	// Fixed engine: kafka
	// No Update support (permission can be updated via new ACL)
	registry.Register(
		KafkaAclResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "acl",
				FixedEngine:    "kafka",
				SupportsUpdate: false,
				StripFields:    []string{"serviceName", "clusterId"}, // No engine field needed
			})
		},
	)

	// KafkaTopic
	// POST /cloud/project/{serviceName}/database/kafka/{clusterId}/topic
	// Fixed engine: kafka
	// Supports Update (PUT)
	registry.Register(
		KafkaTopicResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "topic",
				FixedEngine:    "kafka",
				SupportsUpdate: true,
				StripFields:    []string{"serviceName", "clusterId"},
			})
		},
	)

	// PostgresqlConnectionPool
	// POST /cloud/project/{serviceName}/database/postgresql/{clusterId}/connectionPool
	// Fixed engine: postgresql
	// Supports Update (PUT)
	registry.Register(
		PostgresqlConnectionPoolResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(client *ovhtransport.Client) prov.Provisioner {
			return newNestedProvisioner(client, NestedResourceConfig{
				PathSegment:    "connectionPool",
				FixedEngine:    "postgresql",
				SupportsUpdate: true,
				StripFields:    []string{"serviceName", "clusterId"},
			})
		},
	)
}
