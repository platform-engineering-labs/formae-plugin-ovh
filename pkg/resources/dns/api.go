// pkg/cfres/dns/api.go
package dns

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// DNSAPI defines the API configuration for OVH DNS
var DNSAPI = base.APIConfig{
	BaseURL:     "", // go-ovh handles endpoint
	APIVersion:  "1.0",
	PathBuilder: dnsPathBuilder,
	Pagination:  &base.PaginationConfig{Disabled: true},
}

// DNSOperations defines operation behavior
var DNSOperations = base.OperationConfig{
	Synchronous: true,
	NativeIDExtractor: func(response map[string]interface{}, ctx base.PathContext) string {
		if id, ok := response["id"]; ok {
			return fmt.Sprintf("%s/%v", ctx.Zone, id)
		}
		return ""
	},
	PostMutationHook: nil, // Set in init() after client is available
}

// DNSNativeID defines native ID format: "zone/recordId"
var DNSNativeID = base.NativeIDConfig{
	Format: base.HierarchicalFormat,
	Parser: parseDNSNativeID,
}

// dnsPathBuilder builds paths for DNS resources
func dnsPathBuilder(ctx base.PathContext) string {
	// /domain/zone/{zoneName}/{resourceType}/{id}
	path := fmt.Sprintf("/domain/zone/%s/%s", ctx.Zone, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseDNSNativeID parses "zone/id" format
func parseDNSNativeID(nativeID string) (base.PathContext, error) {
	return base.ParseNativeID(base.NativeIDConfig{Format: base.HierarchicalFormat}, nativeID)
}

// RefreshZone calls the zone refresh endpoint
func RefreshZone(ctx context.Context, client *ovhtransport.Client, zoneName string) error {
	path := fmt.Sprintf("/domain/zone/%s/refresh", zoneName)
	_, err := client.Do(ctx, ovhtransport.RequestOptions{
		Method: "POST",
		Path:   path,
	})
	return err
}
