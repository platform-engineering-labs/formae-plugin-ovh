// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package registry

import (
	"sync"

	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/client"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/prov"
)

// Registry holds factory functions for creating resource provisioners
type Registry struct {
	mu           sync.RWMutex
	provisioners map[string]func(*client.Client, *config.Config) prov.Provisioner
	descriptors  map[string]plugin.ResourceDescriptor
	schemas      map[string]model.Schema
}

var registry = &Registry{
	provisioners: make(map[string]func(*client.Client, *config.Config) prov.Provisioner),
	descriptors:  make(map[string]plugin.ResourceDescriptor),
	schemas:      make(map[string]model.Schema),
}

// Register adds a resource type to the registry
// Called by resource packages in their init() functions
func Register(
	name string,
	descriptor plugin.ResourceDescriptor,
	schema model.Schema,
	factory func(*client.Client, *config.Config) prov.Provisioner,
) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.provisioners[name] = factory
	registry.descriptors[name] = descriptor
	registry.schemas[name] = schema
}

// Get retrieves a provisioner for the given resource type
func Get(name string, client *client.Client, cfg *config.Config) prov.Provisioner {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	factory, ok := registry.provisioners[name]
	if !ok {
		return nil
	}

	return factory(client, cfg)
}

// HasProvisioner checks if a provisioner is registered for the given resource type
func HasProvisioner(name string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	_, ok := registry.provisioners[name]
	return ok
}

// GetDescriptor retrieves the resource descriptor for a given resource type
func GetDescriptor(name string) (plugin.ResourceDescriptor, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	desc, ok := registry.descriptors[name]
	return desc, ok
}

// GetSchema retrieves the schema for a given resource type
func GetSchema(name string) (model.Schema, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	schema, ok := registry.schemas[name]
	return schema, ok
}

// ListResourceTypes returns all registered resource types
func ListResourceTypes() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	types := make([]string, 0, len(registry.provisioners))
	for name := range registry.provisioners {
		types = append(types, name)
	}
	return types
}

// GetAllDescriptors returns all registered resource descriptors
func GetAllDescriptors() []plugin.ResourceDescriptor {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	descriptors := make([]plugin.ResourceDescriptor, 0, len(registry.descriptors))
	for _, desc := range registry.descriptors {
		descriptors = append(descriptors, desc)
	}
	return descriptors
}
