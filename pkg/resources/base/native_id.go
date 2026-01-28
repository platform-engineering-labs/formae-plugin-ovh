package base

import (
	"fmt"
	"strings"
)

// NativeIDFormat defines the format of native IDs
type NativeIDFormat string

const (
	SimpleNameFormat            NativeIDFormat = "name"
	FullPathFormat              NativeIDFormat = "path"
	FullURLFormat               NativeIDFormat = "url"
	HierarchicalFormat          NativeIDFormat = "hierarchical"            // zone/resourceId
	ProjectHierarchicalFormat   NativeIDFormat = "project_hierarchical"    // project/resourceId
	ProjectNestedFormat         NativeIDFormat = "project_nested"          // project/parentId/resourceId (for nested resources)
	ProjectRegionalFormat       NativeIDFormat = "project_regional"        // project/region/resourceId (for regional resources)
	ProjectRegionalNestedFormat NativeIDFormat = "project_regional_nested" // project/region/parentId/resourceId (for regional nested resources like Subnet)
)

// NativeIDConfig defines how native IDs are formatted and parsed
type NativeIDConfig struct {
	Format       NativeIDFormat
	PathTemplate string
	Parser       func(nativeID string) (PathContext, error)
}

// ParseNativeID parses a native ID using the config
func ParseNativeID(cfg NativeIDConfig, nativeID string) (PathContext, error) {
	if cfg.Parser != nil {
		return cfg.Parser(nativeID)
	}

	// Default parsing based on format
	switch cfg.Format {
	case SimpleNameFormat:
		return PathContext{ResourceName: nativeID}, nil
	case HierarchicalFormat:
		// Expect "zone/resourceId" format
		parts := strings.SplitN(nativeID, "/", 2)
		if len(parts) != 2 {
			return PathContext{}, fmt.Errorf("invalid hierarchical ID: %s", nativeID)
		}
		return PathContext{
			Zone:         parts[0],
			ResourceName: parts[1],
		}, nil
	case ProjectHierarchicalFormat:
		// Expect "project/resourceId" format
		parts := strings.SplitN(nativeID, "/", 2)
		if len(parts) != 2 {
			return PathContext{}, fmt.Errorf("invalid project hierarchical ID: %s", nativeID)
		}
		return PathContext{
			Project:      parts[0],
			ResourceName: parts[1],
		}, nil
	case ProjectNestedFormat:
		// Expect "project/parentId/resourceId" format (for nested resources like Subnet)
		parts := strings.SplitN(nativeID, "/", 3)
		if len(parts) != 3 {
			return PathContext{}, fmt.Errorf("invalid project nested ID: %s", nativeID)
		}
		return PathContext{
			Project:        parts[0],
			ParentResource: parts[1],
			ResourceName:   parts[2],
		}, nil
	case ProjectRegionalFormat:
		// Expect "project/region/resourceId" format (for regional resources like FloatingIP)
		parts := strings.SplitN(nativeID, "/", 3)
		if len(parts) != 3 {
			return PathContext{}, fmt.Errorf("invalid project regional ID: %s", nativeID)
		}
		return PathContext{
			Project:      parts[0],
			Region:       parts[1],
			ResourceName: parts[2],
		}, nil
	case ProjectRegionalNestedFormat:
		// Expect "project/region/parentId/resourceId" format (for regional nested resources like Subnet)
		parts := strings.SplitN(nativeID, "/", 4)
		if len(parts) != 4 {
			return PathContext{}, fmt.Errorf("invalid project regional nested ID: %s", nativeID)
		}
		return PathContext{
			Project:        parts[0],
			Region:         parts[1],
			ParentResource: parts[2],
			ResourceName:   parts[3],
		}, nil
	default:
		return PathContext{ResourceName: nativeID}, nil
	}
}

// BuildNativeID builds a native ID from context
func BuildNativeID(cfg NativeIDConfig, ctx PathContext) string {
	switch cfg.Format {
	case SimpleNameFormat:
		return ctx.ResourceName
	case HierarchicalFormat:
		if ctx.Zone != "" {
			return fmt.Sprintf("%s/%s", ctx.Zone, ctx.ResourceName)
		}
		return ctx.ResourceName
	case ProjectHierarchicalFormat:
		if ctx.Project != "" {
			return fmt.Sprintf("%s/%s", ctx.Project, ctx.ResourceName)
		}
		return ctx.ResourceName
	case ProjectNestedFormat:
		if ctx.Project != "" && ctx.ParentResource != "" {
			return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.ParentResource, ctx.ResourceName)
		}
		if ctx.Project != "" {
			return fmt.Sprintf("%s/%s", ctx.Project, ctx.ResourceName)
		}
		return ctx.ResourceName
	case ProjectRegionalFormat:
		if ctx.Project != "" && ctx.Region != "" {
			return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.Region, ctx.ResourceName)
		}
		if ctx.Project != "" {
			return fmt.Sprintf("%s/%s", ctx.Project, ctx.ResourceName)
		}
		return ctx.ResourceName
	case ProjectRegionalNestedFormat:
		if ctx.Project != "" && ctx.Region != "" && ctx.ParentResource != "" {
			return fmt.Sprintf("%s/%s/%s/%s", ctx.Project, ctx.Region, ctx.ParentResource, ctx.ResourceName)
		}
		if ctx.Project != "" && ctx.ParentResource != "" {
			return fmt.Sprintf("%s/%s/%s", ctx.Project, ctx.ParentResource, ctx.ResourceName)
		}
		if ctx.Project != "" {
			return fmt.Sprintf("%s/%s", ctx.Project, ctx.ResourceName)
		}
		return ctx.ResourceName
	default:
		return ctx.ResourceName
	}
}
