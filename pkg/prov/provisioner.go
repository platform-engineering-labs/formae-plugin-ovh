// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package prov

import (
	"context"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Provisioner is the interface that all OVH resource provisioners must implement
type Provisioner interface {
	// Create creates a new resource
	Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error)

	// Read retrieves the current state of a resource
	Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error)

	// Update modifies an existing resource
	Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error)

	// Delete removes a resource
	Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error)

	// Status checks the status of a long-running operation
	Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error)

	// List discovers resources of this type
	List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error)
}
