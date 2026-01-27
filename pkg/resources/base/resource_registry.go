// pkg/cfres/base/resource_registry.go
package base

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/registry"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
)

// StatusChecker checks if a resource is ready/active.
// It receives the resource data from a Read operation.
// Returns true if the resource is ready, false if still pending.
type StatusChecker func(resourceData map[string]interface{}) (ready bool, err error)

// ResourceDefinition defines a complete resource registration
type ResourceDefinition struct {
	ResourceType        string
	APIConfig           APIConfig
	OperationConfig     OperationConfig
	ResourceConfig      ResourceConfig
	NativeIDConfig      NativeIDConfig
	RequestTransformer  RequestTransformer
	ResponseTransformer ResponseTransformer
	StatusChecker       StatusChecker // Optional: checks if resource is ready after creation
	Operations          []resource.Operation
}

// StandardOperations is the default set of operations
var StandardOperations = []resource.Operation{
	resource.OperationCreate,
	resource.OperationRead,
	resource.OperationUpdate,
	resource.OperationDelete,
	resource.OperationList,
	resource.OperationCheckStatus,
}

// ResourceRegistry manages resource definitions for an API
type ResourceRegistry struct {
	apiConfig       APIConfig
	operationConfig OperationConfig
	nativeIDConfig  NativeIDConfig
	Definitions     map[string]*ResourceDefinition
}

// NewResourceRegistry creates a new resource registry
func NewResourceRegistry(
	apiConfig APIConfig,
	operationConfig OperationConfig,
	nativeIDConfig NativeIDConfig,
) *ResourceRegistry {
	return &ResourceRegistry{
		apiConfig:       apiConfig,
		operationConfig: operationConfig,
		nativeIDConfig:  nativeIDConfig,
		Definitions:     make(map[string]*ResourceDefinition),
	}
}

// Register registers a resource definition
func (r *ResourceRegistry) Register(def ResourceDefinition) error {
	if def.ResourceType == "" {
		return fmt.Errorf("resource type cannot be empty")
	}

	// Use common configurations if not specified
	if def.APIConfig.PathBuilder == nil {
		def.APIConfig = r.apiConfig
	}
	if def.OperationConfig.NativeIDExtractor == nil {
		def.OperationConfig = r.operationConfig
	}
	if def.NativeIDConfig.Format == "" {
		def.NativeIDConfig = r.nativeIDConfig
	}
	if def.Operations == nil {
		def.Operations = StandardOperations
	}

	r.Definitions[def.ResourceType] = &def

	// Register with global registry
	registry.Register(
		def.ResourceType,
		def.Operations,
		func(client *ovhtransport.Client) prov.Provisioner {
			return r.CreateProvisioner(client, def.ResourceType)
		},
	)

	return nil
}

// RegisterAll registers multiple definitions
func (r *ResourceRegistry) RegisterAll(definitions []ResourceDefinition) error {
	for _, def := range definitions {
		if err := r.Register(def); err != nil {
			return fmt.Errorf("failed to register %s: %w", def.ResourceType, err)
		}
	}
	return nil
}

// CreateProvisioner creates a provisioner for a resource type
func (r *ResourceRegistry) CreateProvisioner(client *ovhtransport.Client, resourceType string) prov.Provisioner {
	def, ok := r.Definitions[resourceType]
	if !ok {
		panic(fmt.Sprintf("no definition found for resource type: %s", resourceType))
	}

	baseResource := &BaseResource{
		APIConfig:           def.APIConfig,
		OperationConfig:     def.OperationConfig,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      def.NativeIDConfig,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
		StatusChecker:       def.StatusChecker,
		Client:              client,
	}

	return &UnifiedProvisioner{base: baseResource}
}

// UnifiedProvisioner wraps BaseResource to implement Provisioner
type UnifiedProvisioner struct {
	base *BaseResource
}

var _ prov.Provisioner = &UnifiedProvisioner{}

func (p *UnifiedProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	return p.base.Create(ctx, request)
}

func (p *UnifiedProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	return p.base.Read(ctx, request)
}

func (p *UnifiedProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return p.base.Update(ctx, request)
}

func (p *UnifiedProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.base.Delete(ctx, request)
}

func (p *UnifiedProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return p.base.List(ctx, request)
}

func (p *UnifiedProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return p.base.Status(ctx, request)
}
