// pkg/cfres/dns/resources.go
package dns

import (
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
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
		{
			ResourceType: RecordResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "record",
				Scope:          &base.ScopeConfig{Type: base.ScopeZone},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
		},

		// DNS Redirection
		{
			ResourceType: RedirectionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "redirection",
				Scope:          &base.ScopeConfig{Type: base.ScopeZone},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
		},
	})

	if err != nil {
		panic(err)
	}
}
