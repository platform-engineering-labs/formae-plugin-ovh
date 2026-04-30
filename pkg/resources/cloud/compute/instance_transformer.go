// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
)

// instanceResponseTransformer maps the OVH instance response into the shape
// declared by schema/pkl/compute/instance.pkl.
//
// OVH returns `ipAddresses` for the instance's network attachments
// ([{networkId, ip, type, version, gatewayIp}]) but the schema declares
// `networks: Listing<NetworkParams>` ([{networkId, ip}]). The conformance
// test verifies the `networks` field, so we synthesize it from
// `ipAddresses` whenever the response does not already contain it.
type instanceResponseTransformer struct{}

func (instanceResponseTransformer) Transform(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{}, len(props))
	for k, v := range props {
		result[k] = v
	}

	if _, alreadySet := result["networks"]; !alreadySet {
		if networks := networksFromIPAddresses(result["ipAddresses"]); networks != nil {
			result["networks"] = networks
		}
	}

	return result
}

// networksFromIPAddresses derives [{networkId, ip}] entries from the OVH
// `ipAddresses` array, deduplicating on networkId so an instance with
// multiple addresses on the same network produces a single network entry.
func networksFromIPAddresses(raw interface{}) []map[string]interface{} {
	addresses, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	seen := make(map[string]int, len(addresses))
	var networks []map[string]interface{}
	for _, entry := range addresses {
		addr, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		networkID, _ := addr["networkId"].(string)
		if networkID == "" {
			continue
		}
		ip, _ := addr["ip"].(string)
		if idx, exists := seen[networkID]; exists {
			if ip != "" {
				if existingIP, _ := networks[idx]["ip"].(string); existingIP == "" {
					networks[idx]["ip"] = ip
				}
			}
			continue
		}
		entry := map[string]interface{}{"networkId": networkID}
		if ip != "" {
			entry["ip"] = ip
		}
		seen[networkID] = len(networks)
		networks = append(networks, entry)
	}
	return networks
}

var instanceTransformer base.ResponseTransformer = instanceResponseTransformer{}
