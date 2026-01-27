// pkg/cfres/base/resource_config.go
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

// ResourceConfig defines the resource metadata and behavior
type ResourceConfig struct {
	ResourceType      string
	Scope             *ScopeConfig
	ParentResource    *ParentResourceConfig
	SupportsUpdate    bool
	UpdateMethod      UpdateMethod
	UpdateQueryParams map[string]string
	OptimisticLocking *OptimisticLockingConfig
	RequestWrapper    string
}
