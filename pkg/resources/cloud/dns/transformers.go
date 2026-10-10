// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
)

// renameKey moves props[from] to props[to] if present, deleting the original.
// No-op if from is absent.
func renameKey(props map[string]interface{}, from, to string) {
	if v, ok := props[from]; ok {
		props[to] = v
		delete(props, from)
	}
}

// The OVH DNS API uses `target` as the property name, but Formae reserves
// `target` on every Resource for the deployment-target reference. To avoid a
// collision in pkl, DNS resources declare `recordTarget` / `redirectionTarget`
// in their schema; these transformers map to/from the OVH API payload.

var recordRequestTransformer = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		renameKey(props, "recordTarget", "target")
		return props, nil
	},
)

var recordResponseTransformer = base.ResponseTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		renameKey(props, "target", "recordTarget")
		return props
	},
)

var redirectionRequestTransformer = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		renameKey(props, "redirectionTarget", "target")
		return props, nil
	},
)

var redirectionResponseTransformer = base.ResponseTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		renameKey(props, "target", "redirectionTarget")
		return props
	},
)
