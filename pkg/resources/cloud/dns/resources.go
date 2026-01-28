package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants
const (
	ZoneResourceType        = "OVH::DNS::Zone"
	RecordResourceType      = "OVH::DNS::Record"
	RedirectionResourceType = "OVH::DNS::Redirection"
)

var dnsRegistry *base.ResourceRegistry

func init() {
	dnsRegistry = base.NewResourceRegistry(DNSAPI, DNSOperations, DNSNativeID)

	err := dnsRegistry.RegisterAll([]base.ResourceDefinition{
		// DNS Zone (read-only)
		{
			ResourceType: ZoneResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "zone",
				Scope:          &base.ScopeConfig{Type: base.ScopeNone},
				SupportsUpdate: false,
			},
			// Override to use zone name as native ID
			NativeIDConfig: base.NativeIDConfig{
				Format: base.SimpleNameFormat,
			},
			Operations: []resource.Operation{
				resource.OperationRead,
				resource.OperationList,
			},
		},

		// DNS Record
		// Note: List is excluded because records require a zone - you can't list all records across all zones
		{
			ResourceType: RecordResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "record",
				Scope:          &base.ScopeConfig{Type: base.ScopeZone},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
			},
		},

		// DNS Redirection
		// Note: List is excluded because redirections require a zone
		{
			ResourceType: RedirectionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "redirection",
				Scope:          &base.ScopeConfig{Type: base.ScopeZone},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
			},
		},
	})

	if err != nil {
		panic(err)
	}
}
