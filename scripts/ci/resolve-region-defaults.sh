#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Resolve region-specific OVH defaults (instance flavors, Ubuntu image)
# for the configured OS_REGION_NAME and export them so testdata picks
# them up via env-driven defaults in testdata/config/vars.pkl.
#
# Outputs are written to $GITHUB_ENV when set; otherwise printed.

set -euo pipefail

ak="${OVH_APPLICATION_KEY:-}"
as="${OVH_APPLICATION_SECRET:-}"
ck="${OVH_CONSUMER_KEY:-}"
endpoint="${OVH_ENDPOINT:-}"
project="${OVH_CLOUD_PROJECT_ID:-}"
region="${OS_REGION_NAME:-}"

if [[ -z "${ak}" || -z "${as}" || -z "${ck}" || -z "${project}" || -z "${region}" ]]; then
    echo "resolve-region-defaults: missing OVH/region credentials, skipping" >&2
    exit 0
fi

case "${endpoint}" in
    ovh-eu|soyoustart-eu|kimsufi-eu)  base_url="https://eu.api.ovh.com/1.0" ;;
    ovh-us)                            base_url="https://api.us.ovhcloud.com/1.0" ;;
    ovh-ca|soyoustart-ca|kimsufi-ca)  base_url="https://ca.api.ovh.com/1.0" ;;
    https://*)                         base_url="${endpoint}" ;;
    *)                                 base_url="https://eu.api.ovh.com/1.0" ;;
esac

ovh_get() {
    local path="$1"
    local url="${base_url}${path}"
    local timestamp
    timestamp=$(curl -s "${base_url}/auth/time")
    local sig="\$1\$$(echo -n "${as}+${ck}+GET+${url}++${timestamp}" | sha1sum | awk '{print $1}')"
    curl -s -X GET \
        -H "X-Ovh-Application: ${ak}" \
        -H "X-Ovh-Consumer: ${ck}" \
        -H "X-Ovh-Timestamp: ${timestamp}" \
        -H "X-Ovh-Signature: ${sig}" \
        "${url}"
}

# Flavors: pick the first available flavor with matching name in the region.
flavors=$(ovh_get "/cloud/project/${project}/flavor?region=${region}")
flavor_id=$(echo "${flavors}" | jq -r --arg r "${region}" '[.[] | select(.name=="s1-2" and .region==$r and (.available // true))] | .[0].id // empty')
replacement_flavor_id=$(echo "${flavors}" | jq -r --arg r "${region}" '[.[] | select(.name=="d2-2" and .region==$r and (.available // true))] | .[0].id // empty')

# Image: pick the first active Ubuntu 24.04 image in the region.
images=$(ovh_get "/cloud/project/${project}/image?region=${region}&osType=linux")
image_id=$(echo "${images}" | jq -r --arg r "${region}" '[.[] | select(.name=="Ubuntu 24.04" and .region==$r and .status=="active")] | .[0].id // empty')

if [[ -z "${flavor_id}" ]]; then
    echo "resolve-region-defaults: no s1-2 flavor found in ${region}" >&2
    exit 1
fi
if [[ -z "${replacement_flavor_id}" ]]; then
    echo "resolve-region-defaults: no d2-2 flavor found in ${region}" >&2
    exit 1
fi
if [[ -z "${image_id}" ]]; then
    echo "resolve-region-defaults: no active Ubuntu 24.04 image found in ${region}" >&2
    exit 1
fi

echo "OVH_FLAVOR_ID=${flavor_id}"
echo "OVH_REPLACEMENT_FLAVOR_ID=${replacement_flavor_id}"
echo "OVH_IMAGE_ID=${image_id}"

if [[ -n "${GITHUB_ENV:-}" ]]; then
    {
        echo "OVH_FLAVOR_ID=${flavor_id}"
        echo "OVH_REPLACEMENT_FLAVOR_ID=${replacement_flavor_id}"
        echo "OVH_IMAGE_ID=${image_id}"
    } >> "${GITHUB_ENV}"
fi
