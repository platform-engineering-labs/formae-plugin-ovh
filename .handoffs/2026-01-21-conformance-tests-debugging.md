# OVH Plugin Conformance Tests Debugging

**Date**: 2026-01-21
**Session Summary**: Debugging conformance tests and fixing environment issues

---

## What We Accomplished

### 1. Fixed Version Mismatch Issue
- Conformance tests were failing with `malformed EDF: incorrect slice type 0` error
- Root cause: Plugin SDK built against newer formae commits than released v0.76.4
- Solution: Use `make conformance-test VERSION=0.77.2-internal` to match SDK version

### 2. Removed Stale Plugin
- Deleted `/Users/jeroen/.pel/formae/plugins/EXAMPLE` which had old manifest format
- This was causing plugin loading errors during tests

### 3. Fixed Examples Directory
- Removed `/Users/jeroen/dev/pel/formae-plugin-ovh/examples/` directory
- It contained template-specific references (`@example/example.pkl`) that weren't updated for OVH

### 4. Added Plugin Cleanup Before Install
- Updated Makefile `install` target to remove existing plugin versions first:
  ```makefile
  @rm -rf $(PLUGIN_BASE_DIR)/$(PLUGIN_NAMESPACE)
  ```
- Applied same fix to template

### 5. Fixed Region Name for OpenStack CLI
- Changed `.env.ovh` region from `US-EAST-VA` to `US-EAST-VA-1`
- This is required for the openstack CLI to find service endpoints
- The plugin (gophercloud) works with either, but CLI needs the exact region

### 6. Installed OpenStack CLI
- Installed via `pip install python-openstackclient`
- Binary location: `/Users/jeroen/Library/Python/3.9/bin/openstack`
- Requires PATH addition to use cleanup script

---

## Current State

### Conformance Test Results (with VERSION=0.77.2-internal)
**5/9 tests passed**, 4 failed due to OVH quota limits:

| Resource Type | Result | Notes |
|--------------|--------|-------|
| keypair | PASS | |
| network | PASS | |
| securitygroup | PASS | |
| subnet | PASS | |
| volume | PASS | |
| floatingip | FAIL | `Quota exceeded for resources: ['floatingip']` |
| router | FAIL | `Quota exceeded for resources: ['router']` |
| instance | FAIL | `Maximum number of ports exceeded` |
| port | FAIL | `Maximum number of ports exceeded` |

### Issue to Investigate
The cleanup script ran successfully but quotas are still exceeded. User suspects there may be other resources in the account not cleaned up (cleanup only removes resources with `formae-plugin-sdk-test-` prefix).

**Action needed**: Check what resources exist in the OVH account:
```bash
source .env.ovh
PATH="/Users/jeroen/Library/Python/3.9/bin:$PATH"
openstack floating ip list
openstack router list
openstack port list
openstack server list
```

---

## Files Modified This Session

### OVH Plugin (`/Users/jeroen/dev/pel/formae-plugin-ovh/`)
- `.env.ovh` - Fixed region: `US-EAST-VA` -> `US-EAST-VA-1`
- `Makefile` - Added `@rm -rf $(PLUGIN_BASE_DIR)/$(PLUGIN_NAMESPACE)` before install
- Removed `examples/` directory (template remnant)

### Template (`/Users/jeroen/dev/pel/formae-plugin-template/`)
- `Makefile` - Added same plugin cleanup before install

---

## Pending Tasks

1. **Investigate quota issue** - Why are quotas exceeded if cleanup ran?
2. **Enable CI workflow** - Push to GitHub once tests pass
3. **OPEN: Plugin manifest schema pattern** - Keep flat standalone manifest vs require PklProject
4. **OPEN: Struct naming** - Consider renaming `Plugin` to `OVH`
5. **OPEN: Test strategy** - `make test-unit` uses `-tags=unit` but no tests have that tag

---

## Useful Commands

```bash
cd /Users/jeroen/dev/pel/formae-plugin-ovh

# Source credentials
source .env.ovh

# Add openstack CLI to PATH
export PATH="/Users/jeroen/Library/Python/3.9/bin:$PATH"

# Run conformance tests with correct formae version
make conformance-test VERSION=0.77.2-internal

# Run cleanup manually
./scripts/ci/clean-environment.sh

# List resources in OVH
openstack server list
openstack floating ip list
openstack router list
openstack port list
openstack network list
```

---

## Reference Locations

- OVH Plugin: `/Users/jeroen/dev/pel/formae-plugin-ovh`
- Template: `/Users/jeroen/dev/pel/formae-plugin-template`
- Formae: `/Users/jeroen/dev/pel/formae`
- Plan file: `/Users/jeroen/dev/pel/formae-plugin-ovh/.plans/extraction-plan.md`
- Previous handoff: `/Users/jeroen/dev/pel/formae-plugin-ovh/.handoffs/2026-01-21-ovh-plugin-extraction.md`
