#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Clean Environment Hook for OVH/OpenStack
# =========================================
# This script NUKES all test resources in the OpenStack project.
# Called before AND after tests to ensure a clean environment.
#
# The script is idempotent - safe to run multiple times.
# Missing resources (already cleaned) do not cause failures.
#
# WARNING: This deletes ALL user-created resources in the project!
# Only the "default" security group (which OpenStack protects) will remain.

set -euo pipefail

echo "=== NUKING OVH/OpenStack environment ==="
echo ""

# ---- OVH API helper (signed requests) ----
# The OVH API uses HMAC-SHA1 signatures. We need this to clean up
# private networks which are NOT visible through the OpenStack Neutron API.
ovh_api() {
    local method="$1"
    local path="$2"
    local body="${3:-}"

    local ak="${OVH_APPLICATION_KEY:-}"
    local as="${OVH_APPLICATION_SECRET:-}"
    local ck="${OVH_CONSUMER_KEY:-}"
    local endpoint="${OVH_ENDPOINT:-}"

    if [[ -z "${ak}" || -z "${as}" || -z "${ck}" ]]; then
        return 1
    fi

    # Resolve endpoint alias to base URL (go-ovh aliases)
    case "${endpoint}" in
        ovh-eu|soyoustart-eu|kimsufi-eu)  local base_url="https://eu.api.ovh.com/1.0" ;;
        ovh-us)                            local base_url="https://api.us.ovhcloud.com/1.0" ;;
        ovh-ca|soyoustart-ca|kimsufi-ca)  local base_url="https://ca.api.ovh.com/1.0" ;;
        https://*)                         local base_url="${endpoint}" ;;
        *)                                 local base_url="https://eu.api.ovh.com/1.0" ;;
    esac

    local url="${base_url}${path}"
    local timestamp
    timestamp=$(curl -s "${base_url}/auth/time")
    local sig_data="\$1\$$(echo -n "${as}+${ck}+${method}+${url}+${body}+${timestamp}" | sha1sum | awk '{print $1}')"

    local -a curl_args=(
        -s -X "${method}"
        -H "X-Ovh-Application: ${ak}"
        -H "X-Ovh-Consumer: ${ck}"
        -H "X-Ovh-Timestamp: ${timestamp}"
        -H "X-Ovh-Signature: ${sig_data}"
        -H "Content-Type: application/json"
    )

    if [[ -n "${body}" ]]; then
        curl_args+=(-d "${body}")
    fi

    curl "${curl_args[@]}" "${url}"
}

# ---- OVH API: Clean Kubernetes clusters ----
# Managed Kubernetes clusters are not visible through OpenStack — they have
# their own OVH-side endpoints. Delete them before private networks since a
# cluster may attach to a private network.
if [[ -n "${OVH_APPLICATION_KEY:-}" && -n "${OVH_CLOUD_PROJECT_ID:-}" ]]; then
    echo "Cleaning OVH Kubernetes clusters via OVH API..."
    raw_response=$(ovh_api GET "/cloud/project/${OVH_CLOUD_PROJECT_ID}/kube" 2>/dev/null || true)
    if echo "${raw_response}" | jq empty 2>/dev/null; then
        kube_ids=$(echo "${raw_response}" | jq -r '.[] // empty' 2>/dev/null || true)
    else
        echo "  Warning: OVH API returned unexpected response: ${raw_response:0:200}"
        kube_ids=""
    fi
    if [[ -n "${kube_ids}" ]]; then
        echo "${kube_ids}" | while read -r kube_id; do
            [[ -z "${kube_id}" ]] && continue
            echo "  Deleting OVH kube cluster: ${kube_id}"
            ovh_api DELETE "/cloud/project/${OVH_CLOUD_PROJECT_ID}/kube/${kube_id}" 2>/dev/null || echo "  Warning: Failed to delete ${kube_id}"
        done
    else
        echo "  No OVH kube clusters found"
    fi
else
    echo "Skipping OVH kube cluster cleanup (OVH API credentials not set)"
fi

# ---- OVH API: Clean private networks ----
# OVH private networks are NOT visible through OpenStack Neutron,
# so we must clean them via the OVH REST API.
if [[ -n "${OVH_APPLICATION_KEY:-}" && -n "${OVH_CLOUD_PROJECT_ID:-}" ]]; then
    echo "Cleaning OVH private networks via OVH API..."
    raw_response=$(ovh_api GET "/cloud/project/${OVH_CLOUD_PROJECT_ID}/network/private" 2>/dev/null || true)
    if echo "${raw_response}" | jq empty 2>/dev/null; then
        network_ids=$(echo "${raw_response}" | jq -r '.[].id // empty' 2>/dev/null || true)
    else
        echo "  Warning: OVH API returned unexpected response: ${raw_response:0:200}"
        network_ids=""
    fi
    if [[ -n "${network_ids}" ]]; then
        echo "${network_ids}" | while read -r net_id; do
            [[ -z "${net_id}" ]] && continue
            echo "  Deleting OVH private network: ${net_id}"
            ovh_api DELETE "/cloud/project/${OVH_CLOUD_PROJECT_ID}/network/private/${net_id}" 2>/dev/null || echo "  Warning: Failed to delete ${net_id}"
        done
    else
        echo "  No OVH private networks found"
    fi
else
    echo "Skipping OVH private network cleanup (OVH API credentials not set)"
fi

# Check if openstack CLI is available
if ! command -v openstack &> /dev/null; then
    echo "Warning: openstack CLI not found. Skipping cleanup."
    echo "Install with: pip install python-openstackclient"
    exit 0
fi

# Check if credentials are configured
if [[ -z "${OS_AUTH_URL:-}" ]]; then
    echo "Warning: OS_AUTH_URL not set. Skipping cleanup."
    exit 0
fi

# Clean resources in dependency order (most dependent first)

# 1. Instances (servers)
echo "Cleaning ALL instances..."
instance_ids=$(openstack server list -f value -c ID 2>/dev/null || true)
if [[ -n "${instance_ids}" ]]; then
    echo "${instance_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting instance: ${id}"
        openstack server delete --wait "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No instances found"
fi

# 2. Floating IPs - delete ALL
echo "Cleaning ALL floating IPs..."
fip_ids=$(openstack floating ip list -f value -c ID 2>/dev/null || true)
if [[ -n "${fip_ids}" ]]; then
    echo "${fip_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting floating IP: ${id}"
        openstack floating ip delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No floating IPs found"
fi

# 3. Routers (need to remove interfaces and gateway first)
echo "Cleaning ALL routers..."
router_ids=$(openstack router list -f value -c ID 2>/dev/null || true)
if [[ -n "${router_ids}" ]]; then
    echo "${router_ids}" | while read -r router_id; do
        [[ -z "${router_id}" ]] && continue
        echo "  Processing router: ${router_id}"

        # Remove all subnet interfaces from router
        subnet_ids=$(openstack router show "${router_id}" -f json 2>/dev/null | jq -r '.interfaces_info[]?.subnet_id // empty' 2>/dev/null || true)
        if [[ -n "${subnet_ids}" ]]; then
            echo "${subnet_ids}" | while read -r subnet_id; do
                [[ -z "${subnet_id}" ]] && continue
                echo "    Removing subnet interface: ${subnet_id}"
                openstack router remove subnet "${router_id}" "${subnet_id}" 2>/dev/null || true
            done
        fi

        # Clear external gateway
        openstack router unset --external-gateway "${router_id}" 2>/dev/null || true

        echo "  Deleting router: ${router_id}"
        openstack router delete "${router_id}" 2>/dev/null || echo "  Warning: Failed to delete ${router_id}"
    done
else
    echo "  No routers found"
fi

# 4. Ports (excluding network:dhcp and network:router_interface which are auto-managed)
echo "Cleaning ALL user ports..."
port_ids=$(openstack port list -f value -c ID -c "Device Owner" 2>/dev/null | grep -v "network:" | awk '{print $1}' || true)
if [[ -n "${port_ids}" ]]; then
    echo "${port_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting port: ${id}"
        openstack port delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No user ports found"
fi

# 5. Subnets (only from internal/private networks, skip external/provider-managed subnets like Ext-Net)
# Must delete subnets before their parent networks
echo "Cleaning subnets from private networks..."
internal_network_ids=$(openstack network list --internal -f value -c ID 2>/dev/null || true)
if [[ -n "${internal_network_ids}" ]]; then
    for net_id in ${internal_network_ids}; do
        subnet_ids=$(openstack subnet list --network "${net_id}" -f value -c ID 2>/dev/null || true)
        if [[ -n "${subnet_ids}" ]]; then
            echo "${subnet_ids}" | while read -r id; do
                [[ -z "${id}" ]] && continue
                echo "  Deleting subnet: ${id} (from network ${net_id})"
                openstack subnet delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
            done
        fi
    done
else
    echo "  No private networks found"
fi

# 6. Networks (excluding external/public networks)
echo "Cleaning ALL private networks..."
network_ids=$(openstack network list --internal -f value -c ID 2>/dev/null || true)
if [[ -n "${network_ids}" ]]; then
    echo "${network_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting network: ${id}"
        openstack network delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No private networks found"
fi

# 7. Security groups (except "default" which OpenStack protects)
echo "Cleaning ALL security groups (except default)..."
sg_ids=$(openstack security group list -f value -c ID -c Name 2>/dev/null | grep -v " default$" | awk '{print $1}' || true)
if [[ -n "${sg_ids}" ]]; then
    echo "${sg_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting security group: ${id}"
        openstack security group delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No security groups found (except default)"
fi

# 8. Volumes
echo "Cleaning ALL volumes..."
volume_ids=$(openstack volume list -f value -c ID 2>/dev/null || true)
if [[ -n "${volume_ids}" ]]; then
    echo "${volume_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting volume: ${id}"
        openstack volume delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No volumes found"
fi

# 9. Keypairs
echo "Cleaning ALL keypairs..."
keypair_names=$(openstack keypair list -f value -c Name 2>/dev/null || true)
if [[ -n "${keypair_names}" ]]; then
    echo "${keypair_names}" | while read -r name; do
        [[ -z "${name}" ]] && continue
        echo "  Deleting keypair: ${name}"
        openstack keypair delete "${name}" 2>/dev/null || echo "  Warning: Failed to delete ${name}"
    done
else
    echo "  No keypairs found"
fi

echo ""
echo "=== Environment nuked ==="
