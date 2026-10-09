#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Resolve region-specific OVH IDs (s1-2 / d2-2 flavors, Ubuntu 24.04 image)
# for OS_REGION_NAME and export them as OVH_FLAVOR_ID,
# OVH_REPLACEMENT_FLAVOR_ID and OVH_IMAGE_ID, which testdata/config/vars.pkl
# reads in place of its hard-coded defaults.
#
# Values are appended to $GITHUB_ENV when set; otherwise printed.
#
# Fails soft: any value that cannot be resolved is left unset (with a
# warning) so vars.pkl falls back to its defaults. The script always exits 0.
#
# Read-only: issues GET requests only.

set -uo pipefail

warn() {
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        echo "::warning::resolve-region-defaults: $*"
    else
        echo "resolve-region-defaults: warning: $*" >&2
    fi
}

ak="${OVH_APPLICATION_KEY:-}"
as="${OVH_APPLICATION_SECRET:-}"
ck="${OVH_CONSUMER_KEY:-}"
endpoint="${OVH_ENDPOINT:-}"
project="${OVH_CLOUD_PROJECT_ID:-}"
region="${OS_REGION_NAME:-}"

if [[ -z "${ak}" || -z "${as}" || -z "${ck}" || -z "${project}" || -z "${region}" ]]; then
    warn "missing OVH credentials or OS_REGION_NAME; keeping vars.pkl defaults"
    exit 0
fi

for tool in curl jq; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        warn "${tool} not found; keeping vars.pkl defaults"
        exit 0
    fi
done

case "${endpoint}" in
    ovh-eu|soyoustart-eu|kimsufi-eu) base_url="https://eu.api.ovh.com/1.0" ;;
    ovh-us)                          base_url="https://api.us.ovhcloud.com/1.0" ;;
    ovh-ca|soyoustart-ca|kimsufi-ca) base_url="https://ca.api.ovh.com/1.0" ;;
    https://*)                       base_url="${endpoint%/}" ;;
    *)                               base_url="https://eu.api.ovh.com/1.0" ;;
esac

sha1() {
    if command -v sha1sum >/dev/null 2>&1; then
        sha1sum | awk '{print $1}'
    else
        shasum -a 1 | awk '{print $1}'
    fi
}

# ovh_get PATH: signed GET against the OVH API; prints the body, non-zero on
# any HTTP or transport failure.
ovh_get() {
    local url="${base_url}$1"
    local timestamp sig
    timestamp=$(curl -fsS "${base_url}/auth/time") || return 1
    sig="\$1\$$(printf '%s' "${as}+${ck}+GET+${url}++${timestamp}" | sha1)"
    curl -fsS -X GET \
        -H "X-Ovh-Application: ${ak}" \
        -H "X-Ovh-Consumer: ${ck}" \
        -H "X-Ovh-Timestamp: ${timestamp}" \
        -H "X-Ovh-Signature: ${sig}" \
        "${url}"
}

# A flavor is usable unless OVH explicitly marks it unavailable.
# (`.available // true` would wrongly turn an explicit false into true.)
flavor_filter='[.[] | select(.name == $n and .region == $r and .available != false)] | .[0].id // empty'
image_filter='[.[] | select(.name == "Ubuntu 24.04" and .region == $r and .status == "active")] | .[0].id // empty'

flavor_id=""
replacement_flavor_id=""
image_id=""

if flavors=$(ovh_get "/cloud/project/${project}/flavor?region=${region}"); then
    flavor_id=$(jq -r --arg r "${region}" --arg n "s1-2" "${flavor_filter}" <<<"${flavors}" 2>/dev/null) || flavor_id=""
    replacement_flavor_id=$(jq -r --arg r "${region}" --arg n "d2-2" "${flavor_filter}" <<<"${flavors}" 2>/dev/null) || replacement_flavor_id=""
else
    warn "failed to list flavors in ${region}"
fi

if images=$(ovh_get "/cloud/project/${project}/image?region=${region}&osType=linux"); then
    image_id=$(jq -r --arg r "${region}" "${image_filter}" <<<"${images}" 2>/dev/null) || image_id=""
else
    warn "failed to list images in ${region}"
fi

# export_or_warn NAME VALUE DESCRIPTION
export_or_warn() {
    if [[ -z "$2" ]]; then
        warn "no $3 resolved in ${region}; keeping vars.pkl default"
        return
    fi
    echo "$1=$2"
    if [[ -n "${GITHUB_ENV:-}" ]]; then
        echo "$1=$2" >> "${GITHUB_ENV}"
    fi
}

export_or_warn OVH_FLAVOR_ID "${flavor_id}" "s1-2 flavor"
export_or_warn OVH_REPLACEMENT_FLAVOR_ID "${replacement_flavor_id}" "d2-2 flavor"
export_or_warn OVH_IMAGE_ID "${image_id}" "active Ubuntu 24.04 image"

exit 0
