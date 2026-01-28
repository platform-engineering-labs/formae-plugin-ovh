// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloud

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/stretchr/testify/assert"
)

func TestCloudPathBuilder(t *testing.T) {
	tests := []struct {
		name     string
		ctx      base.PathContext
		wantPath string
	}{
		{
			name: "project-scoped resource",
			ctx: base.PathContext{
				Project:      "my-project",
				ResourceType: "network/private",
			},
			wantPath: "/cloud/project/my-project/network/private",
		},
		{
			name: "project-scoped resource with ID",
			ctx: base.PathContext{
				Project:      "my-project",
				ResourceType: "network/private",
				ResourceName: "network-123",
			},
			wantPath: "/cloud/project/my-project/network/private/network-123",
		},
		{
			name: "regional resource (Network)",
			ctx: base.PathContext{
				Project:      "my-project",
				Region:       "GRA7",
				ResourceType: "network",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/network",
		},
		{
			name: "regional resource with ID",
			ctx: base.PathContext{
				Project:      "my-project",
				Region:       "GRA7",
				ResourceType: "network",
				ResourceName: "network-456",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/network/network-456",
		},
		{
			name: "nested regional resource (Subnet) - collection",
			ctx: base.PathContext{
				Project:        "my-project",
				Region:         "GRA7",
				ParentType:     "network",
				ParentResource: "network-456",
				ResourceType:   "subnet",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/network/network-456/subnet",
		},
		{
			name: "nested regional resource (Subnet) - with ID for Delete/Read",
			ctx: base.PathContext{
				Project:        "my-project",
				Region:         "GRA7",
				ParentType:     "network",
				ParentResource: "network-456",
				ResourceType:   "subnet",
				ResourceName:   "subnet-789",
			},
			wantPath: "/cloud/project/my-project/region/GRA7/network/network-456/subnet/subnet-789",
		},
		{
			name: "nested project-scoped resource (SubnetPrivate) - collection",
			ctx: base.PathContext{
				Project:        "my-project",
				ParentType:     "network/private",
				ParentResource: "network-123",
				ResourceType:   "subnet",
			},
			wantPath: "/cloud/project/my-project/network/private/network-123/subnet",
		},
		{
			name: "nested project-scoped resource (SubnetPrivate) - with ID",
			ctx: base.PathContext{
				Project:        "my-project",
				ParentType:     "network/private",
				ParentResource: "network-123",
				ResourceType:   "subnet",
				ResourceName:   "subnet-456",
			},
			wantPath: "/cloud/project/my-project/network/private/network-123/subnet/subnet-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath := cloudPathBuilder(tt.ctx)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestCloudNativeIDExtractor(t *testing.T) {
	extractor := CloudOperations.NativeIDExtractor

	tests := []struct {
		name       string
		response   map[string]interface{}
		ctx        base.PathContext
		wantNative string
	}{
		{
			name:       "top-level resource with id field",
			response:   map[string]interface{}{"id": "network-123"},
			ctx:        base.PathContext{Project: "my-project"},
			wantNative: "my-project/network-123",
		},
		{
			name:       "top-level resource with resourceId (async operation)",
			response:   map[string]interface{}{"resourceId": "network-456"},
			ctx:        base.PathContext{Project: "my-project"},
			wantNative: "my-project/network-456",
		},
		{
			name:     "nested resource (Subnet) - includes parentId in native ID",
			response: map[string]interface{}{"id": "subnet-789"},
			ctx: base.PathContext{
				Project:        "my-project",
				ParentResource: "network-456",
			},
			wantNative: "my-project/network-456/subnet-789",
		},
		{
			name:     "nested resource with resourceId (async operation)",
			response: map[string]interface{}{"resourceId": "subnet-abc"},
			ctx: base.PathContext{
				Project:        "my-project",
				ParentResource: "network-def",
			},
			wantNative: "my-project/network-def/subnet-abc",
		},
		{
			name:       "empty response",
			response:   map[string]interface{}{},
			ctx:        base.PathContext{Project: "my-project"},
			wantNative: "",
		},
		{
			name:       "no project",
			response:   map[string]interface{}{"id": "resource-123"},
			ctx:        base.PathContext{},
			wantNative: "resource-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNative := extractor(tt.response, tt.ctx)
			assert.Equal(t, tt.wantNative, gotNative)
		})
	}
}
