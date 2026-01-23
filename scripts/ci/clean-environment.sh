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

# Helper function to safely delete resources by prefix
delete_resources_by_prefix() {
    local resource_type="$1"
    local list_cmd="$2"
    local delete_cmd="$3"
    local count=0

    echo "Cleaning ${resource_type}..."

    # Get the list output and filter by prefix
    local output
    output=$(eval "${list_cmd}" 2>/dev/null) || output=""

    if [[ -z "${output}" ]]; then
        echo "  No ${resource_type} found with prefix '${TEST_PREFIX}'"
        return
    fi

    # Process each line, looking for the prefix in the name column
    while IFS= read -r line; do
        [[ -z "${line}" ]] && continue

        # Check if this line contains our prefix
        if echo "${line}" | grep -q "${TEST_PREFIX}"; then
            # Extract the ID (first column)
            local id
            id=$(echo "${line}" | awk '{print $1}')
            if [[ -n "${id}" ]]; then
                echo "  Deleting ${resource_type}: ${id}"
                eval "${delete_cmd} '${id}'" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
                ((count++)) || true
            fi
        fi
    done <<< "${output}"

    if [[ ${count} -eq 0 ]]; then
        echo "  No ${resource_type} found with prefix '${TEST_PREFIX}'"
    else
        echo "  Deleted ${count} ${resource_type}"
    fi
}

# Clean resources in dependency order (most dependent first)

# 1. Instances (depend on networks, security groups, keypairs)
delete_resources_by_prefix "instances" \
    "openstack server list -f value -c ID -c Name" \
    "openstack server delete --wait"

# 2. Floating IPs - delete ALL unattached floating IPs
# In CI environments, unattached floating IPs are orphaned test resources
# They consume port quota and should be cleaned up aggressively
echo "Cleaning floating IPs (all unattached)..."
fip_count=0
# Get all floating IPs and check which ones have no fixed IP (unattached)
while IFS='|' read -r id floating_ip fixed_ip port_id rest; do
    # Trim whitespace
    id=$(echo "${id}" | xargs)
    fixed_ip=$(echo "${fixed_ip}" | xargs)
    port_id=$(echo "${port_id}" | xargs)

    [[ -z "${id}" ]] && continue
    [[ "${id}" == "ID" ]] && continue  # Skip header

    # If no port attached, it's orphaned
    if [[ -z "${port_id}" || "${port_id}" == "None" ]]; then
        echo "  Deleting unattached floating IP: ${id} (${floating_ip:-unknown})"
        openstack floating ip delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
        ((fip_count++)) || true
    fi
done < <(openstack floating ip list -f value -c ID -c "Floating IP Address" -c "Fixed IP Address" -c Port 2>/dev/null || echo "")

if [[ ${fip_count} -eq 0 ]]; then
    echo "  No unattached floating IPs found"
else
    echo "  Deleted ${fip_count} floating IPs"
fi

# 3. Routers (need to remove interfaces first)
echo "Cleaning routers..."
router_count=0
while IFS= read -r line; do
    [[ -z "${line}" ]] && continue

    if echo "${line}" | grep -q "${TEST_PREFIX}"; then
        router_id=$(echo "${line}" | awk '{print $1}')
        router_name=$(echo "${line}" | awk '{print $2}')

        if [[ -n "${router_id}" ]]; then
            echo "  Removing interfaces from router: ${router_id} (${router_name})"

            # Get all ports attached to this router and remove them
            while IFS= read -r port_id; do
                [[ -z "${port_id}" ]] && continue
                echo "    Removing port: ${port_id}"
                openstack router remove port "${router_id}" "${port_id}" 2>/dev/null || true
            done < <(openstack port list --router "${router_id}" -f value -c ID 2>/dev/null || echo "")

            # Clear external gateway
            openstack router unset --external-gateway "${router_id}" 2>/dev/null || true

            echo "  Deleting router: ${router_id}"
            openstack router delete "${router_id}" 2>/dev/null || echo "  Warning: Failed to delete ${router_id}"
            ((router_count++)) || true
        fi
    fi
done < <(openstack router list -f value -c ID -c Name 2>/dev/null || echo "")

if [[ ${router_count} -eq 0 ]]; then
    echo "  No routers found with prefix '${TEST_PREFIX}'"
else
    echo "  Deleted ${router_count} routers"
fi

# 4. Ports (orphaned ports with test prefix)
delete_resources_by_prefix "ports" \
    "openstack port list -f value -c ID -c Name" \
    "openstack port delete"

# 5. Subnets
delete_resources_by_prefix "subnets" \
    "openstack subnet list -f value -c ID -c Name" \
    "openstack subnet delete"

# 6. Networks
delete_resources_by_prefix "networks" \
    "openstack network list -f value -c ID -c Name" \
    "openstack network delete"

# 7. Security groups (can't delete default)
delete_resources_by_prefix "security groups" \
    "openstack security group list -f value -c ID -c Name" \
    "openstack security group delete"

# 8. Volumes
delete_resources_by_prefix "volumes" \
    "openstack volume list -f value -c ID -c Name" \
    "openstack volume delete"

# 9. Keypairs (name is the identifier, not ID)
echo "Cleaning keypairs..."
kp_count=0
while IFS= read -r name; do
    [[ -z "${name}" ]] && continue

    if [[ "${name}" == ${TEST_PREFIX}* ]]; then
        echo "  Deleting keypair: ${name}"
        openstack keypair delete "${name}" 2>/dev/null || echo "  Warning: Failed to delete ${name}"
        ((kp_count++)) || true
    fi
done < <(openstack keypair list -f value -c Name 2>/dev/null || echo "")

if [[ ${kp_count} -eq 0 ]]; then
    echo "  No keypairs found with prefix '${TEST_PREFIX}'"
else
    echo "  Deleted ${kp_count} keypairs"
fi

echo ""
echo "Cleanup complete."
