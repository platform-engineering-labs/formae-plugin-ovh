// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// instanceRequestTransformer adapts the request body per operation. Create
// passes through; Update reduces to {instanceName} because PUT
// /cloud/project/{}/instance/{id} only accepts cloud.ProjectInstanceUpdate
// (single field: instanceName) and rejects the full schema body.
type instanceRequestTransformer struct{}

func (t *instanceRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	if ctx.Operation != resource.OperationUpdate {
		return props, nil
	}
	name, _ := props["name"].(string)
	return map[string]interface{}{"instanceName": name}, nil
}

var instanceRequestTransformer_ = &instanceRequestTransformer{}

// instanceResponseTransformer normalizes the OVH instance Read response so it
// matches the schema shape expected by formae:
//   - Drops nullable optional fields the API echoes as null when unset
//     (sshKeyId, availabilityZone, monthlyBilling). Without this, formae sees
//     "field present" for fields the user never set.
//   - Synthesizes the `networks` array from `ipAddresses`. The OVH API accepts
//     `networks` on create but never returns it; instead it returns
//     `ipAddresses[]`. Each unique networkId becomes one NetworkParams entry.
//     This is required so update diffs don't see `networks` as missing — which
//     would force a replace (createOnly) and change the NativeID.
type instanceResponseTransformer struct{}

func (t *instanceResponseTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{}, len(props))
	for k, v := range props {
		if v == nil {
			continue
		}
		result[k] = v
	}

	if ips, ok := props["ipAddresses"].([]interface{}); ok {
		seen := make(map[string]bool)
		var networks []interface{}
		for _, raw := range ips {
			entry, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			netID, _ := entry["networkId"].(string)
			if netID == "" || seen[netID] {
				continue
			}
			seen[netID] = true
			networks = append(networks, map[string]interface{}{"networkId": netID})
		}
		if len(networks) > 0 {
			result["networks"] = networks
		}
	}

	return result
}

var instanceResponseTransformer_ = &instanceResponseTransformer{}
