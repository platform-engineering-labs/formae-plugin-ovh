#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Setup Credentials Hook for OVH/OpenStack
# =========================================
# This script is called before running conformance tests to verify
# that OpenStack credentials are properly configured.
#
# For local development, source your OpenStack credentials file:
#   source ~/.ovh-openstack-credentials
#
# For CI environments (GitHub Actions), credentials are configured
# via secrets in the workflow file.

set -euo pipefail

# Required OpenStack environment variables
REQUIRED_VARS=("OS_AUTH_URL" "OS_USERNAME" "OS_PASSWORD" "OS_PROJECT_ID")
MISSING_VARS=()

for var in "${REQUIRED_VARS[@]}"; do
    if [[ -z "${!var:-}" ]]; then
        MISSING_VARS+=("$var")
    fi
done

if [[ ${#MISSING_VARS[@]} -gt 0 ]]; then
    echo "Error: Missing required environment variables: ${MISSING_VARS[*]}"
    echo ""
    echo "For local development, source your OpenStack credentials file:"
    echo "  source ~/.ovh-openstack-credentials"
    echo ""
    echo "Required variables:"
    echo "  OS_AUTH_URL      - OpenStack authentication URL"
    echo "  OS_USERNAME      - OpenStack username"
    echo "  OS_PASSWORD      - OpenStack password"
    echo "  OS_PROJECT_ID    - OpenStack project/tenant ID"
    echo ""
    echo "Optional variables:"
    echo "  OS_REGION_NAME       - Region name (e.g., GRA7, DE1)"
    echo "  OS_USER_DOMAIN_NAME  - User domain (default: Default)"
    exit 1
fi

echo "OpenStack credentials configured:"
echo "  OS_AUTH_URL: ${OS_AUTH_URL}"
echo "  OS_USERNAME: ${OS_USERNAME}"
echo "  OS_PROJECT_ID: ${OS_PROJECT_ID}"
echo "  OS_REGION_NAME: ${OS_REGION_NAME:-not set}"
