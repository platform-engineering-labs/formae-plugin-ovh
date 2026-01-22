#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Clean Environment Hook for OVH/OpenStack
# =========================================
# This script cleans up test resources created during conformance tests.
# Called before AND after tests to ensure a clean environment.
#
# The script is idempotent - safe to run multiple times.
# Missing resources (already cleaned) do not cause failures.

set -euo pipefail

# Prefix used for test resources
TEST_PREFIX="${TEST_PREFIX:-formae-plugin-sdk-test-}"

echo "Cleaning OVH/OpenStack resources with prefix '${TEST_PREFIX}'..."
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

# Helper function to safely delete resources
delete_resources() {
    local resource_type="$1"
    local list_cmd="$2"
    local delete_cmd="$3"

    echo "Cleaning ${resource_type}..."
    local ids
    ids=$(eval "${list_cmd}" 2>/dev/null || echo "")

    if [[ -n "${ids}" ]]; then
        while IFS= read -r id; do
            if [[ -n "${id}" ]]; then
                echo "  Deleting ${resource_type}: ${id}"
                eval "${delete_cmd} ${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
            fi
        done <<< "${ids}"
    else
        echo "  No ${resource_type} found with prefix '${TEST_PREFIX}'"
    fi
}

# Clean resources in dependency order (most dependent first)

# 1. Instances (depend on networks, security groups, keypairs)
delete_resources "instances" \
    "openstack server list --name '^${TEST_PREFIX}' -f value -c ID" \
    "openstack server delete --wait"

# 2. Floating IPs (delete all unattached - they don't have names, only IPs)
# In CI environments, unattached floating IPs are orphaned test resources
echo "Cleaning floating IPs (unattached)..."
floating_ip_ids=$(openstack floating ip list --status DOWN -f value -c ID 2>/dev/null || echo "")
if [[ -n "${floating_ip_ids}" ]]; then
    while IFS= read -r fip_id; do
        if [[ -n "${fip_id}" ]]; then
            echo "  Deleting floating IP: ${fip_id}"
            openstack floating ip delete "${fip_id}" 2>/dev/null || echo "  Warning: Failed to delete ${fip_id}"
        fi
    done <<< "${floating_ip_ids}"
else
    echo "  No unattached floating IPs found"
fi

# 3. Routers (need to remove interfaces first)
echo "Cleaning routers..."
router_ids=$(openstack router list --name "^${TEST_PREFIX}" -f value -c ID 2>/dev/null || echo "")
if [[ -n "${router_ids}" ]]; then
    while IFS= read -r router_id; do
        if [[ -n "${router_id}" ]]; then
            echo "  Removing interfaces from router: ${router_id}"
            # Get subnet ports attached to router and remove them
            port_ids=$(openstack port list --router "${router_id}" -f value -c ID 2>/dev/null || echo "")
            while IFS= read -r port_id; do
                if [[ -n "${port_id}" ]]; then
                    openstack router remove port "${router_id}" "${port_id}" 2>/dev/null || true
                fi
            done <<< "${port_ids}"

            # Clear external gateway
            openstack router unset --external-gateway "${router_id}" 2>/dev/null || true

            echo "  Deleting router: ${router_id}"
            openstack router delete "${router_id}" 2>/dev/null || echo "  Warning: Failed to delete ${router_id}"
        fi
    done <<< "${router_ids}"
else
    echo "  No routers found with prefix '${TEST_PREFIX}'"
fi

# 4. Ports (orphaned ports)
delete_resources "ports" \
    "openstack port list --name '^${TEST_PREFIX}' -f value -c ID" \
    "openstack port delete"

# 5. Subnets
delete_resources "subnets" \
    "openstack subnet list --name '^${TEST_PREFIX}' -f value -c ID" \
    "openstack subnet delete"

# 6. Networks
delete_resources "networks" \
    "openstack network list --name '^${TEST_PREFIX}' -f value -c ID" \
    "openstack network delete"

# 7. Security groups (can't delete default)
delete_resources "security groups" \
    "openstack security group list --format value -c ID -c Name | grep '${TEST_PREFIX}' | awk '{print \$1}'" \
    "openstack security group delete"

# 8. Volumes
delete_resources "volumes" \
    "openstack volume list --name '^${TEST_PREFIX}' -f value -c ID" \
    "openstack volume delete --force"

# 9. Keypairs
delete_resources "keypairs" \
    "openstack keypair list -f value -c Name | grep '^${TEST_PREFIX}'" \
    "openstack keypair delete"

echo ""
echo "Cleanup complete."
