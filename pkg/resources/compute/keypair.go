// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/registry"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources"
)

const (
	ResourceTypeKeypair = "OVH::Compute::Keypair"
)

// Keypair schema and descriptor
var (
	KeypairDescriptor = plugin.ResourceDescriptor{
		Type:         ResourceTypeKeypair,
		Discoverable: true,
	}

	KeypairSchema = model.Schema{
		Identifier:   "name",
		Discoverable: true,
		Fields:       []string{"name", "publicKey"},
		Hints: map[string]model.FieldHint{
			"name": {
				Required:   true,
				CreateOnly: true,
			},
			"publicKey": {
				CreateOnly: true,
			},
		},
	}
)

// Keypair provisioner
type Keypair struct {
	Client *client.Client
	Config *config.Config
}

// Register the Keypair resource type
func init() {
	registry.Register(
		ResourceTypeKeypair,
		KeypairDescriptor,
		KeypairSchema,
		func(client *client.Client, cfg *config.Config) prov.Provisioner {
			return &Keypair{
				Client: client,
				Config: cfg,
			}
		},
	)
}

// Create creates a new keypair
func (k *Keypair) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse request properties
	props, err := resources.ParseProperties(request.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeKeypair, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	// Extract keypair name (required)
	name, ok := props["name"].(string)
	if !ok || name == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "name is required",
			},
		}, nil
	}

	// Build create options
	createOpts := keypairs.CreateOpts{
		Name: name,
	}

	// Add optional public key
	if publicKey, ok := props["publicKey"].(string); ok && publicKey != "" {
		createOpts.PublicKey = publicKey
	}

	// Create the keypair via OpenStack
	kp, err := keypairs.Create(ctx, k.Client.ComputeClient, createOpts).Extract()
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationCreate, ResourceTypeKeypair, resources.MapOpenStackErrorToOperationErrorCode(err), "", fmt.Sprintf("failed to create keypair: %v", err)),
		}, nil
	}

	// Return success - keypairs are created synchronously
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        kp.Name, // OpenStack keypair identifier is the name
		},
	}, nil
}

// Read retrieves the current state of a keypair
func (k *Keypair) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get the keypair name from NativeID
	name := request.NativeID
	if name == "" {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, fmt.Errorf("nativeID is required")
	}

	// Get the keypair from OpenStack
	kp, err := keypairs.Get(ctx, k.Client.ComputeClient, name, nil).Extract()
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resources.MapOpenStackErrorToOperationErrorCode(err),
		}, fmt.Errorf("failed to read keypair: %w", err)
	}

	// Convert keypair to properties
	props := map[string]interface{}{
		"name":        kp.Name,
		"fingerprint": kp.Fingerprint,
		"publicKey":   kp.PublicKey,
	}

	// Marshal properties to JSON
	propsJSON, err := resources.MarshalProperties(props)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeGeneralServiceException,
		}, err
	}

	return &resource.ReadResult{
		Properties: propsJSON,
	}, nil
}

// Update is not supported for keypairs (they are immutable)
func (k *Keypair) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeInvalidRequest,
			StatusMessage:   "keypairs are immutable and cannot be updated",
		},
	}, nil
}

// Delete removes a keypair
func (k *Keypair) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Validate NativeID
	if err := resources.ValidateNativeID(request.NativeID); err != nil {
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeKeypair, resource.OperationErrorCodeInvalidRequest, "", err.Error()),
		}, nil
	}

	name := request.NativeID

	// Delete the keypair from OpenStack
	err := keypairs.Delete(ctx, k.Client.ComputeClient, name, nil).ExtractErr()
	if err != nil {
		// Check if the error is NotFound - if so, consider it a success (idempotent delete)
		errCode := resources.MapOpenStackErrorToOperationErrorCode(err)
		if errCode == resource.OperationErrorCodeNotFound {
			// Resource already deleted - this is a success
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        name,
				},
			}, nil
		}

		// Other errors are actual failures
		return &resource.DeleteResult{
			ProgressResult: resources.NewFailureResultWithMessage(resource.OperationDelete, ResourceTypeKeypair, errCode, name, fmt.Sprintf("failed to delete keypair: %v", err)),
		}, nil
	}

	// Return success
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        name,
		},
	}, nil
}

// Status checks the status of a long-running operation (keypairs are synchronous, so not used)
func (k *Keypair) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// List discovers keypairs
func (k *Keypair) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	// List all keypairs using pagination
	allPages, err := keypairs.List(k.Client.ComputeClient, nil).AllPages(ctx)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to list keypairs: %w", err)
	}

	// Extract keypairs from pages
	kps, err := keypairs.ExtractKeyPairs(allPages)
	if err != nil {
		return &resource.ListResult{}, fmt.Errorf("failed to extract keypairs: %w", err)
	}

	// Collect NativeIDs for discovery
	nativeIDs := make([]string, 0, len(kps))
	for _, kp := range kps {
		nativeIDs = append(nativeIDs, kp.Name)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}
