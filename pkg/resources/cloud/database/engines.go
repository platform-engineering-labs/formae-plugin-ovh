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

// engineRegistration pairs the API engine name with its Formae resource type.
type engineRegistration struct {
	resourceType string
	engine       string
}

var engineClusterRegistrations = []engineRegistration{
	{"OVH::Database::PostgreSQL", "postgresql"},
	{"OVH::Database::MongoDB", "mongodb"},
	{"OVH::Database::Redis", "redis"},
	{"OVH::Database::Kafka", "kafka"},
	{"OVH::Database::OpenSearch", "opensearch"},
	{"OVH::Database::Cassandra", "cassandra"},
	{"OVH::Database::M3DB", "m3db"},
	{"OVH::Database::Grafana", "grafana"},
	{"OVH::Database::ClickHouse", "clickhouse"},
	{"OVH::Database::KafkaConnect", "kafkaConnect"},
	{"OVH::Database::KafkaMirrorMaker", "kafkaMirrorMaker"},
}

// nestedRegistration represents a nested resource registration for an engine.
type nestedRegistration struct {
	resourceType   string
	engine         string
	pathSegment    string
	idField        string
	supportsUpdate bool
	stripFields    []string
	ops            []resource.Operation
}

func crudOps(extra ...resource.Operation) []resource.Operation {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationDelete,
		resource.OperationList,
	}
	return append(ops, extra...)
}

var engineNestedRegistrations = []nestedRegistration{
	// PostgreSQL
	{"OVH::Database::PostgreSQLUser", "postgresql", "user", "", false, nil, crudOps()},
	{"OVH::Database::PostgreSQLDatabase", "postgresql", "database", "", false, nil, crudOps()},
	{"OVH::Database::PostgreSQLIpRestriction", "postgresql", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::PostgreSQLIntegration", "postgresql", "integration", "", false, nil, crudOps()},
	{"OVH::Database::PostgreSQLConnectionPool", "postgresql", "connectionPool", "", true, nil, crudOps(resource.OperationUpdate)},

	// MongoDB
	{"OVH::Database::MongoDBUser", "mongodb", "user", "", false, nil, crudOps()},
	{"OVH::Database::MongoDBIpRestriction", "mongodb", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::MongoDBIntegration", "mongodb", "integration", "", false, nil, crudOps()},

	// Redis
	{"OVH::Database::RedisUser", "redis", "user", "", false, nil, crudOps()},
	{"OVH::Database::RedisIpRestriction", "redis", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::RedisIntegration", "redis", "integration", "", false, nil, crudOps()},

	// Kafka
	{"OVH::Database::KafkaUser", "kafka", "user", "", false, nil, crudOps()},
	{"OVH::Database::KafkaIpRestriction", "kafka", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::KafkaIntegration", "kafka", "integration", "", false, nil, crudOps()},
	{"OVH::Database::KafkaTopic", "kafka", "topic", "", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::KafkaAcl", "kafka", "acl", "", false, nil, crudOps()},

	// OpenSearch
	{"OVH::Database::OpenSearchUser", "opensearch", "user", "", false, nil, crudOps()},
	{"OVH::Database::OpenSearchIpRestriction", "opensearch", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::OpenSearchIntegration", "opensearch", "integration", "", false, nil, crudOps()},
	{"OVH::Database::OpenSearchPattern", "opensearch", "pattern", "", false, nil, crudOps()},

	// Cassandra
	{"OVH::Database::CassandraUser", "cassandra", "user", "", false, nil, crudOps()},
	{"OVH::Database::CassandraIpRestriction", "cassandra", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::CassandraIntegration", "cassandra", "integration", "", false, nil, crudOps()},

	// M3DB
	{"OVH::Database::M3DBUser", "m3db", "user", "", false, nil, crudOps()},
	{"OVH::Database::M3DBIpRestriction", "m3db", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::M3DBIntegration", "m3db", "integration", "", false, nil, crudOps()},
	{"OVH::Database::M3DBNamespace", "m3db", "namespace", "", true, nil, crudOps(resource.OperationUpdate)},

	// Grafana
	{"OVH::Database::GrafanaUser", "grafana", "user", "", false, nil, crudOps()},
	{"OVH::Database::GrafanaIpRestriction", "grafana", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::GrafanaIntegration", "grafana", "integration", "", false, nil, crudOps()},

	// ClickHouse (no ipRestriction in spec)
	{"OVH::Database::ClickHouseUser", "clickhouse", "user", "", false, nil, crudOps()},
	{"OVH::Database::ClickHouseDatabase", "clickhouse", "database", "", false, nil, crudOps()},
	{"OVH::Database::ClickHouseIntegration", "clickhouse", "integration", "", false, nil, crudOps()},

	// KafkaConnect
	{"OVH::Database::KafkaConnectUser", "kafkaConnect", "user", "", false, nil, crudOps()},
	{"OVH::Database::KafkaConnectIpRestriction", "kafkaConnect", "ipRestriction", "ip", true, nil, crudOps(resource.OperationUpdate)},
	{"OVH::Database::KafkaConnectIntegration", "kafkaConnect", "integration", "", false, nil, crudOps()},
	{"OVH::Database::KafkaConnectConnector", "kafkaConnect", "connector", "", true, nil, crudOps(resource.OperationUpdate)},

	// KafkaMirrorMaker (integration only per spec)
	{"OVH::Database::KafkaMirrorMakerIntegration", "kafkaMirrorMaker", "integration", "", false, nil, crudOps()},
}

func init() {
	clusterOps := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}

	for _, reg := range engineClusterRegistrations {
		engine := reg.engine
		registry.Register(
			reg.resourceType,
			clusterOps,
			func(client *ovhtransport.Client) prov.Provisioner {
				return newEngineClusterProvisioner(client, engine)
			},
		)
	}

	for _, reg := range engineNestedRegistrations {
		cfg := EngineNestedConfig{
			Engine:         reg.engine,
			PathSegment:    reg.pathSegment,
			IDField:        reg.idField,
			SupportsUpdate: reg.supportsUpdate,
			StripFields:    reg.stripFields,
		}
		registry.Register(
			reg.resourceType,
			reg.ops,
			func(client *ovhtransport.Client) prov.Provisioner {
				return newEngineNestedProvisioner(client, cfg)
			},
		)
	}
}
