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

# Debug: Show we have credentials
echo "DEBUG: Using OS_AUTH_URL=${OS_AUTH_URL}"
echo "DEBUG: Using OS_PROJECT_ID=${OS_PROJECT_ID:-not set}"
echo ""

# Clean resources in dependency order (most dependent first)

# 1. Instances
echo "Cleaning instances..."
instance_output=$(openstack server list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw instance output: '${instance_output}'"
instance_ids=$(echo "${instance_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${instance_ids}" ]]; then
    echo "${instance_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting instance: ${id}"
        openstack server delete --wait "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No instances found with prefix '${TEST_PREFIX}'"
fi

# 2. Floating IPs - delete ALL unattached
echo "Cleaning floating IPs (all unattached)..."
fip_output=$(openstack floating ip list -f value -c ID -c "Floating IP Address" -c Port 2>&1) || true
echo "DEBUG: Raw floating IP output: '${fip_output}'"
fip_count=0
echo "${fip_output}" | while read -r id fip port rest; do
    [[ -z "${id}" ]] && continue
    [[ "${id}" == "ID" ]] && continue
    # If port is "None" or empty, it's unattached
    if [[ "${port}" == "None" || -z "${port}" ]]; then
        echo "  Deleting unattached floating IP: ${id} (${fip})"
        openstack floating ip delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
        ((fip_count++)) || true
    fi
done
if [[ ${fip_count} -eq 0 ]]; then
    echo "  No unattached floating IPs found"
fi

# 3. Routers (need to remove interfaces first)
echo "Cleaning routers..."
router_output=$(openstack router list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw router output: '${router_output}'"
router_ids=$(echo "${router_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${router_ids}" ]]; then
    echo "${router_ids}" | while read -r router_id; do
        [[ -z "${router_id}" ]] && continue
        echo "  Processing router: ${router_id}"

        # Remove all ports from router
        port_ids=$(openstack port list --router "${router_id}" -f value -c ID 2>/dev/null || true)
        echo "${port_ids}" | while read -r port_id; do
            [[ -z "${port_id}" ]] && continue
            echo "    Removing port: ${port_id}"
            openstack router remove port "${router_id}" "${port_id}" 2>/dev/null || true
        done

        # Clear external gateway
        openstack router unset --external-gateway "${router_id}" 2>/dev/null || true

        echo "  Deleting router: ${router_id}"
        openstack router delete "${router_id}" 2>/dev/null || echo "  Warning: Failed to delete ${router_id}"
    done
else
    echo "  No routers found with prefix '${TEST_PREFIX}'"
fi

# 4. Ports
echo "Cleaning ports..."
port_output=$(openstack port list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw port output (first 500 chars): '${port_output:0:500}'"
port_ids=$(echo "${port_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${port_ids}" ]]; then
    echo "${port_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting port: ${id}"
        openstack port delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No ports found with prefix '${TEST_PREFIX}'"
fi

# 5. Subnets
echo "Cleaning subnets..."
subnet_output=$(openstack subnet list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw subnet output: '${subnet_output}'"
subnet_ids=$(echo "${subnet_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${subnet_ids}" ]]; then
    echo "${subnet_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting subnet: ${id}"
        openstack subnet delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No subnets found with prefix '${TEST_PREFIX}'"
fi

# 6. Networks
echo "Cleaning networks..."
network_output=$(openstack network list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw network output: '${network_output}'"
network_ids=$(echo "${network_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${network_ids}" ]]; then
    echo "${network_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting network: ${id}"
        openstack network delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No networks found with prefix '${TEST_PREFIX}'"
fi

# 7. Security groups
echo "Cleaning security groups..."
sg_output=$(openstack security group list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw security group output: '${sg_output}'"
sg_ids=$(echo "${sg_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${sg_ids}" ]]; then
    echo "${sg_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting security group: ${id}"
        openstack security group delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No security groups found with prefix '${TEST_PREFIX}'"
fi

# 8. Volumes
echo "Cleaning volumes..."
volume_output=$(openstack volume list -f value -c ID -c Name 2>&1) || true
echo "DEBUG: Raw volume output: '${volume_output}'"
volume_ids=$(echo "${volume_output}" | grep "${TEST_PREFIX}" | awk '{print $1}' || true)
if [[ -n "${volume_ids}" ]]; then
    echo "${volume_ids}" | while read -r id; do
        [[ -z "${id}" ]] && continue
        echo "  Deleting volume: ${id}"
        openstack volume delete "${id}" 2>/dev/null || echo "  Warning: Failed to delete ${id}"
    done
else
    echo "  No volumes found with prefix '${TEST_PREFIX}'"
fi

# 9. Keypairs
echo "Cleaning keypairs..."
keypair_output=$(openstack keypair list -f value -c Name 2>&1) || true
echo "DEBUG: Raw keypair output: '${keypair_output}'"
keypair_names=$(echo "${keypair_output}" | grep "^${TEST_PREFIX}" || true)
if [[ -n "${keypair_names}" ]]; then
    echo "${keypair_names}" | while read -r name; do
        [[ -z "${name}" ]] && continue
        echo "  Deleting keypair: ${name}"
        openstack keypair delete "${name}" 2>/dev/null || echo "  Warning: Failed to delete ${name}"
    done
else
    echo "  No keypairs found with prefix '${TEST_PREFIX}'"
fi

echo ""
echo "Cleanup complete."
