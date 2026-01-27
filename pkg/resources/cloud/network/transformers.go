// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package network

import (
	"fmt"
	"net"

	"github.com/platform-engineering-labs/formae-plugin-ovh/pkg/resources/base"
)

// subnetRequestTransformer transforms schema fields to OVH API field names.
// The OVH API at POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
// uses different field names than our schema:
//   - enableDhcp → dhcp
//   - cidr → network
//   - enableGatewayIp → noGateway (inverted!)
//   - start/end are REQUIRED and calculated from cidr
type subnetRequestTransformer struct{}

func (t *subnetRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// region - keep as-is (required in body)
	if region, ok := props["region"].(string); ok {
		result["region"] = region
	}

	// cidr → network, and calculate start/end
	if cidr, ok := props["cidr"].(string); ok {
		result["network"] = cidr

		// Calculate start/end from CIDR (required fields!)
		start, end, err := calculateDefaultAllocationRange(cidr)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate allocation range from cidr %q: %w", cidr, err)
		}
		result["start"] = start
		result["end"] = end
	}

	// enableDhcp → dhcp
	if enableDhcp, ok := props["enableDhcp"].(bool); ok {
		result["dhcp"] = enableDhcp
	}

	// enableGatewayIp → noGateway (INVERTED!)
	if enableGatewayIp, ok := props["enableGatewayIp"].(bool); ok {
		result["noGateway"] = !enableGatewayIp
	}

	return result, nil
}

// calculateDefaultAllocationRange calculates the default IP allocation range from a CIDR.
// For example, 10.0.3.0/24 → start=10.0.3.2, end=10.0.3.254
// Reserves .0 (network), .1 (gateway), and .255 (broadcast) for /24.
func calculateDefaultAllocationRange(cidr string) (start, end string, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("invalid CIDR: %w", err)
	}

	// Get network address as 4-byte slice
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", "", fmt.Errorf("only IPv4 is supported")
	}

	// Get the mask size to calculate usable range
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", "", fmt.Errorf("only IPv4 is supported")
	}

	// Calculate number of host addresses
	hostBits := bits - ones
	if hostBits < 2 {
		return "", "", fmt.Errorf("CIDR block too small for allocation")
	}

	// Start IP: network + 2 (skip network address and gateway)
	startIP := make(net.IP, 4)
	copy(startIP, ip)
	startIP[3] = ip[3] + 2

	// End IP: broadcast - 1
	// For /24: broadcast is .255, so end is .254
	numHosts := uint32(1) << hostBits
	endOffset := numHosts - 2 // -1 for broadcast, -1 more for last usable

	endIP := make(net.IP, 4)
	copy(endIP, ip)
	// Add the offset to the base network address
	ipUint := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	endUint := ipUint + endOffset
	endIP[0] = byte(endUint >> 24)
	endIP[1] = byte(endUint >> 16)
	endIP[2] = byte(endUint >> 8)
	endIP[3] = byte(endUint)

	return startIP.String(), endIP.String(), nil
}

// subnetPrivateRequestTransformer transforms subnet properties for the private network API.
// The API at POST /cloud/project/{serviceName}/network/private/{networkId}/subnet
// expects: network, dhcp, noGateway, start, end, region.
// Strips network_id (used in URL path).
type subnetPrivateRequestTransformer struct{}

func (t *subnetPrivateRequestTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy all fields except network_id (used in URL path)
	for k, v := range props {
		if k == "network_id" {
			// Used in URL path, not in body
			continue
		}
		result[k] = v
	}

	return result, nil
}

var subnetPrivateTransformer = &subnetPrivateRequestTransformer{}

// privateNetworkResponseTransformer simplifies the regions field in the response.
// OVH API returns regions as [{openstackId, region, status}, ...] but we only need ["DE1", ...]
type privateNetworkResponseTransformer struct{}

func (t *privateNetworkResponseTransformer) Transform(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range props {
		result[k] = v
	}

	// Transform regions from [{region: "DE1", ...}, ...] to ["DE1", ...]
	if regions, ok := props["regions"].([]interface{}); ok {
		var regionStrings []string
		for _, r := range regions {
			if regionObj, ok := r.(map[string]interface{}); ok {
				if regionName, ok := regionObj["region"].(string); ok {
					regionStrings = append(regionStrings, regionName)
				}
			}
		}
		result["regions"] = regionStrings
	}

	if id, ok := props["id"].([]interface{}); ok {
		result["ovhId"] = fmt.Sprintf("%v", id)
	}
	return result
}

var privateNetworkResponseTransformer_ = &privateNetworkResponseTransformer{}
