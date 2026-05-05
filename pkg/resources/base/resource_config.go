package base

// ScopeType defines the scoping type for a resource
type ScopeType string

const (
	ScopeNone          ScopeType = "none"
	ScopeGlobal        ScopeType = "global"
	ScopeRegional      ScopeType = "regional"
	ScopeZonal         ScopeType = "zonal"
	ScopeLocationBased ScopeType = "location"
	ScopeZone          ScopeType = "zone"    // OVH: scoped to DNS zone
	ScopeProject       ScopeType = "project" // OVH: scoped to Cloud project (serviceName)
)

// ScopeConfig defines how a resource is scoped
type ScopeConfig struct {
	Type ScopeType
}

// UpdateMethod defines how updates are performed
type UpdateMethod string

const (
	UpdateMethodPatch UpdateMethod = "PATCH"
	UpdateMethodPut   UpdateMethod = "PUT"
)

// OptimisticLockingConfig defines optimistic locking behavior
type OptimisticLockingConfig struct {
	Enabled       bool
	FieldName     string
	LocationInURL bool
}

// ParentResourceConfig defines parent resource for nested resources
type ParentResourceConfig struct {
	RequiresParent bool
	ParentType     string
	PropertyName   string
}

// CustomSegmentsConfig defines custom path segments extracted from properties.
// Used for complex nested paths like /network/{networkId}/subnet/{subnetId}/gateway
type CustomSegmentsConfig struct {
	PropertyNames []string // Property names to extract into CustomSegments, in order
}

// ResourceConfig defines the resource metadata and behavior
type ResourceConfig struct {
	ResourceType         string
	Scope                *ScopeConfig
	ParentResource       *ParentResourceConfig
	CustomSegmentsConfig *CustomSegmentsConfig
	SupportsUpdate       bool
	UpdateMethod         UpdateMethod
	UpdateQueryParams    map[string]string
	OptimisticLocking    *OptimisticLockingConfig
	RequestWrapper       string

	// DeletingStatuses lists status values that indicate the resource is being
	// deleted. When Read encounters one of these in the "status" response field,
	// it returns NotFound so formae's sync correctly tombstones the resource.
	DeletingStatuses []string

	// AsyncDelete makes Delete return OperationStatusInProgress instead of
	// Success once the provider has accepted the DELETE call. Formae then
	// polls Status() until the resource returns 404 (deletion complete).
	// Use this when downstream resources share state with the deleted
	// resource — e.g. an OVH instance keeps a port allocated on its subnet
	// for several seconds after DELETE returns 200; without async-delete,
	// formae fires sibling Deletes (subnet, network) too soon and the
	// provider rejects them with "ports have an IP allocation from this
	// subnet". Async-delete is preferred over a synchronous wait because
	// formae owns the polling cadence and the plugin avoids hard-coded
	// timeouts.
	AsyncDelete bool

	// URLFieldInjection injects PathContext fields back into the response body
	// when those fields are required by the schema but only present in the URL
	// (e.g. serviceName, region). Map keys are body field names; values are
	// PathContext field names ("Project", "Region", "Zone", "Location",
	// "Engine", "ParentResource"). Without this, formae's resource_persister
	// silently drops discovered resources whose required schema fields are
	// URL-only.
	URLFieldInjection map[string]string
}
