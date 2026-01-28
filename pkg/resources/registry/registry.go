// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package registry

import (
	"sync"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/prov"
	openstacktransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/openstack"
	ovhtransport "github.com/platform-engineering-labs/formae-plugin-ovh/pkg/transport/ovh"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// TransportType indicates which transport a resource uses
type TransportType string

const (
	TransportOVH       TransportType = "ovh"
	TransportOpenStack TransportType = "openstack"
)

// OVHProvisionerFactory creates a provisioner using OVH transport
type OVHProvisionerFactory func(client *ovhtransport.Client) prov.Provisioner

// OpenStackProvisionerFactory creates a provisioner using OpenStack transport
type OpenStackProvisionerFactory func(client *openstacktransport.Client, cfg *openstacktransport.Config) prov.Provisioner

type registration struct {
	transportType    TransportType
	operations       []resource.Operation
	ovhFactory       OVHProvisionerFactory
	openstackFactory OpenStackProvisionerFactory
}

var (
	mu            sync.RWMutex
	registrations = make(map[string]*registration)
)

// Register registers a resource type with an OVH provisioner factory
func Register(resourceType string, operations []resource.Operation, factory OVHProvisionerFactory) {
	mu.Lock()
	defer mu.Unlock()
	registrations[resourceType] = &registration{
		transportType: TransportOVH,
		operations:    operations,
		ovhFactory:    factory,
	}
}

// RegisterOpenStack registers a resource type with an OpenStack provisioner factory
func RegisterOpenStack(resourceType string, operations []resource.Operation, factory OpenStackProvisionerFactory) {
	mu.Lock()
	defer mu.Unlock()
	registrations[resourceType] = &registration{
		transportType:    TransportOpenStack,
		operations:       operations,
		openstackFactory: factory,
	}
}

// GetTransportType returns the transport type for a resource
func GetTransportType(resourceType string) TransportType {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := registrations[resourceType]
	if !ok {
		return ""
	}
	return reg.transportType
}

// GetOVHFactory returns the OVH provisioner factory for a resource type
func GetOVHFactory(resourceType string) (OVHProvisionerFactory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := registrations[resourceType]
	if !ok || reg.transportType != TransportOVH {
		return nil, false
	}
	return reg.ovhFactory, true
}

// GetOpenStackFactory returns the OpenStack provisioner factory for a resource type
func GetOpenStackFactory(resourceType string) (OpenStackProvisionerFactory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := registrations[resourceType]
	if !ok || reg.transportType != TransportOpenStack {
		return nil, false
	}
	return reg.openstackFactory, true
}

// GetOperations returns supported operations for a resource type
func GetOperations(resourceType string) []resource.Operation {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := registrations[resourceType]
	if !ok {
		return nil
	}
	return reg.operations
}

// HasProvisioner checks if a resource type is registered
func HasProvisioner(resourceType string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := registrations[resourceType]
	return ok
}

// ResourceTypes returns all registered resource types
func ResourceTypes() []string {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]string, 0, len(registrations))
	for t := range registrations {
		types = append(types, t)
	}
	return types
}

// OVHResourceTypes returns resource types using OVH transport
func OVHResourceTypes() []string {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]string, 0)
	for t, reg := range registrations {
		if reg.transportType == TransportOVH {
			types = append(types, t)
		}
	}
	return types
}

// OpenStackResourceTypes returns resource types using OpenStack transport
func OpenStackResourceTypes() []string {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]string, 0)
	for t, reg := range registrations {
		if reg.transportType == TransportOpenStack {
			types = append(types, t)
		}
	}
	return types
}
