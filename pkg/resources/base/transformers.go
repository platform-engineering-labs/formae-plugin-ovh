// pkg/cfres/base/transformers.go
package base

import "github.com/platform-engineering-labs/formae/pkg/plugin/resource"

// TransformContext provides context for transformers
type TransformContext struct {
	Project      string
	Region       string
	Zone         string
	Location     string
	ResourceType string
	Operation    resource.Operation
}

// RequestTransformer transforms request properties before sending to API
type RequestTransformer interface {
	Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error)
}

// ResponseTransformer transforms API response properties before returning
type ResponseTransformer interface {
	Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{}
}

// RequestTransformerFunc is a function adapter for RequestTransformer
type RequestTransformerFunc func(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error)

func (f RequestTransformerFunc) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	return f(props, ctx)
}

// ResponseTransformerFunc is a function adapter for ResponseTransformer
type ResponseTransformerFunc func(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{}

func (f ResponseTransformerFunc) Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{} {
	return f(apiResponse, ctx)
}

// PassThroughTransformer returns properties unchanged
type PassThroughTransformer struct{}

func (t *PassThroughTransformer) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	return props, nil
}
