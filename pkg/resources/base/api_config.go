package base

import "fmt"

// APIConfig defines the configuration for an API
type APIConfig struct {
	BaseURL     string
	APIVersion  string
	PathBuilder PathBuilderFunc
	Pagination  *PaginationConfig
}

// PaginationConfig defines pagination parameter names
type PaginationConfig struct {
	Disabled       bool
	PageSizeParam  string
	PageTokenParam string
}

// IsPaginationDisabled returns true if pagination is disabled
func (c *APIConfig) IsPaginationDisabled() bool {
	return c.Pagination != nil && c.Pagination.Disabled
}

// PathBuilderFunc constructs a resource path from context
type PathBuilderFunc func(ctx PathContext) string

// PathContext contains all information needed to build a URL path
type PathContext struct {
	Project        string
	Region         string
	Zone           string
	Location       string
	Engine         string // Database engine type (postgresql, mysql, kafka, etc.)
	ResourceType   string
	ResourceName   string
	ParentResource string
	ParentType     string
	CustomSegments []string
}

// URLBuilder builds URLs for API resources
type URLBuilder struct {
	apiConfig APIConfig
	context   PathContext
}

// NewURLBuilder creates a new URL builder
func NewURLBuilder(apiConfig APIConfig, context PathContext) *URLBuilder {
	return &URLBuilder{apiConfig: apiConfig, context: context}
}

// CollectionURL returns the URL for a resource collection
func (b *URLBuilder) CollectionURL() string {
	ctx := b.context
	ctx.ResourceName = ""
	path := b.apiConfig.PathBuilder(ctx)
	if b.apiConfig.BaseURL != "" {
		return fmt.Sprintf("%s%s", b.apiConfig.BaseURL, path)
	}
	return path
}

// ResourceURL returns the URL for a specific resource
func (b *URLBuilder) ResourceURL(name string) string {
	ctx := b.context
	ctx.ResourceName = name
	path := b.apiConfig.PathBuilder(ctx)
	if b.apiConfig.BaseURL != "" {
		return fmt.Sprintf("%s%s", b.apiConfig.BaseURL, path)
	}
	return path
}
