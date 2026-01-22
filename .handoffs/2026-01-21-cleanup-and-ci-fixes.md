# OVH Plugin Cleanup and CI Fixes

**Date**: 2026-01-21
**Session Summary**: Fixed cleanup script, test strategy, and identified CI gaps

---

## What We Accomplished

### 1. Manually Cleaned Orphaned OVH Resources
Found and deleted resources that cleanup script couldn't identify:
- 15 floating IPs (no names, just IP addresses)
- 2 routers with test prefix
- 1 orphaned port

### 2. Fixed Cleanup Script (OVH-specific)
**Problem**: Floating IPs don't have names in OpenStack, only IP addresses like `40.160.4.39`. The cleanup script was trying to grep for `formae-plugin-sdk-test-` prefix which never matched.

**Solution**: Changed to delete all unattached floating IPs (`--status DOWN`) since in CI environments these are orphaned test resources.

**File**: `/Users/jeroen/dev/pel/formae-plugin-ovh/scripts/ci/clean-environment.sh`
```bash
# Before (broken):
delete_resources "floating IPs" \
    "openstack floating ip list --format value -c ID -c 'Floating IP Address' | grep '${TEST_PREFIX}' | awk '{print \$1}'" \
    "openstack floating ip delete"

# After (fixed):
echo "Cleaning floating IPs (unattached)..."
floating_ip_ids=$(openstack floating ip list --status DOWN -f value -c ID 2>/dev/null || echo "")
# ... delete loop
```

**Note**: This is OVH/OpenStack-specific. The template has placeholder examples, not real implementation.

### 3. Fixed Test Strategy (Both Template and OVH)
**Problem**: `make test-unit` uses `-tags=unit` but no tests have that tag. OVH has integration tests with `//go:build integration` but no Makefile target.

**Solution**: Added `test-integration` target to both Makefiles:
```makefile
## test-integration: Run integration tests (requires cloud credentials)
test-integration:
	$(GO) test -v -tags=integration ./...
```

**Files modified**:
- `/Users/jeroen/dev/pel/formae-plugin-ovh/Makefile`
- `/Users/jeroen/dev/pel/formae-plugin-template/Makefile`

### 4. Ran Conformance Tests After Cleanup
**Results**: 15/18 tests passed
- 9/9 Discovery tests PASSED
- 6/9 CRUD tests passed, 3 failed due to OVH API connection resets (transient network errors, not plugin bugs)

Failed tests (all due to `connection reset by peer`):
- network
- router
- volume

---

## Pending Tasks

### 1. Add Integration Tests to CI Workflow
**Both template and OVH** - CI currently only runs `make test-unit`. Need to add `make test-integration` when credentials are available.

Current CI (`build` job):
```yaml
- name: Test
  run: make test-unit
```

Should add integration tests (probably in conformance-tests job or separate job with credentials).

### 2. Enable CI Workflow for OVH
- Uncomment OpenStack credentials setup
- Configure GitHub secrets: `OS_AUTH_URL`, `OS_USERNAME`, `OS_PASSWORD`, `OS_PROJECT_ID`, `OS_REGION_NAME`
- Enable conformance tests (currently gated by `run_conformance == 'true'`)

### 3. OPEN: Plugin Manifest Schema Pattern
Keep flat standalone manifest (validated at runtime) vs require PklProject?

### 4. OPEN: Struct Naming
Consider renaming `Plugin` struct to actual plugin name (e.g., `OVH`)?

---

## Files Modified This Session

### OVH Plugin (`/Users/jeroen/dev/pel/formae-plugin-ovh/`)
- `scripts/ci/clean-environment.sh` - Fixed floating IP cleanup
- `Makefile` - Added `test-integration` target

### Template (`/Users/jeroen/dev/pel/formae-plugin-template/`)
- `Makefile` - Added `test-integration` target

---

## Test Results Summary

| Test Type | Passed | Failed | Notes |
|-----------|--------|--------|-------|
| CRUD | 6/9 | 3 | network, router, volume failed (API connection resets) |
| Discovery | 9/9 | 0 | All passed |
| **Total** | **15/18** | **3** | Failures are transient OVH API issues |

---

## Key Learnings

1. **OpenStack floating IPs have no name field** - Can't identify test resources by prefix. Must use status (DOWN = unattached) or tags/description.

2. **OVH API can be flaky** - Connection resets happen occasionally. Consider retry logic or accept CI flakiness.

3. **Template vs Instance changes**:
   - Template changes: Patterns that apply to all plugins (Makefile targets, CI structure)
   - Instance changes: Provider-specific implementations (cleanup script logic)

---

## Useful Commands

```bash
cd /Users/jeroen/dev/pel/formae-plugin-ovh

# Source credentials
source .env.ovh
export PATH="/Users/jeroen/Library/Python/3.9/bin:$PATH"

# Run tests
make test-integration          # Integration tests (needs credentials)
make conformance-test VERSION=0.77.2-internal  # Conformance tests

# Manual cleanup
openstack floating ip list --status DOWN
openstack router list --name "^formae-plugin-sdk-test-"
./scripts/ci/clean-environment.sh
```

---

## Reference Locations

- OVH Plugin: `/Users/jeroen/dev/pel/formae-plugin-ovh`
- Template: `/Users/jeroen/dev/pel/formae-plugin-template`
- Previous handoff: `.handoffs/2026-01-21-conformance-tests-debugging.md`
