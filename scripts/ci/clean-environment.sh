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

# 5. Subnets
echo "Cleaning ALL subnets..."
subnet_ids=$(openstack subnet list -f value -c ID 2>/dev/null || true)
if [[ -n "${subnet_ids}" ]]; then
    echo "${subnet_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting subnet: ${id}"
        openstack subnet delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No subnets found"
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
