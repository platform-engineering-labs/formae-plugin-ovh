package prov

import (
	"context"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Provisioner interface for resource operations
type Provisioner interface {
	Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error)
	Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error)
	Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error)
	Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error)
	List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error)
	Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error)
}
