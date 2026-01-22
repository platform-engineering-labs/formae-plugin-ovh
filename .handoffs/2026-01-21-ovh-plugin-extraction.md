# OVH Plugin Extraction Handoff

**Date**: 2026-01-21
**Session Summary**: OVH plugin extraction setup

---

## What We Accomplished

### 1. Scaffolded OVH Plugin
- Ran `formae plugin init` to create `/Users/jeroen/dev/pel/formae-plugin-ovh`
- Configured with name=ovh, namespace=OVH

### 2. Copied Implementation from Backup
- Source: `/Users/jeroen/dev/pel/formae-plugin-ovh.bak/`
- Copied:
  - `pkg/` - client, config, prov, registry, resources (8 resource types)
  - `ovh.go` - ResourcePlugin implementation
  - `schema/pkl/` - PKL schemas for all resources
  - `testdata/` - 30+ conformance test scenarios
  - `ovh_integration_test.go` - from internal repo

### 3. Fixed Makefile/Manifest Issues
- **Problem**: `pkl eval` failed because manifest used `amends "@formae/..."` which requires dependency resolution
- **Solution**: Simplified `formae-plugin.pkl` to flat structure without `amends`:
  ```pkl
  name = "ovh"
  version = "0.1.0"
  namespace = "OVH"
  license = "Apache-2.0"
  minFormaeVersion = "0.76.0"
  ```
- Fixed Makefile to use `namespace` instead of `spec.namespace`
- Applied same fixes to template

### 4. Fixed macOS sed Compatibility
- Changed `sed -i "..."` to `sed -i '' "..."` in `scripts/run-conformance-tests.sh`
- Applied to both OVH plugin and template

### 5. Implemented CI Hooks
- `scripts/ci/setup-credentials.sh` - validates OpenStack env vars
- `scripts/ci/clean-environment.sh` - cleans up test resources using openstack CLI

### 6. Created Credentials File
- `.env.ovh` with OVH OpenStack credentials (gitignored)
- Source before running tests: `source .env.ovh`

---

## Current State

### Conformance Tests Running
Tests were started but take time (downloads 175MB formae binary). Check with:
```bash
cd /Users/jeroen/dev/pel/formae-plugin-ovh
source .env.ovh
make conformance-test
```

### Build Status
- `make build` ✅ works
- `make lint` ✅ passes
- `make install` ✅ installs to `~/.pel/formae/plugins/OVH/v0.1.0/`

---

## Open Questions (Need Decisions)

### 1. Plugin Manifest Schema Pattern
Created `PluginManifest.pkl` in formae at `plugins/pkl/assets/formae/PluginManifest.pkl`, but:
- `pkl eval` in Makefile can't resolve `formae://` URIs (only works with formae's custom evaluator)
- **Current approach**: Keep flat standalone manifest, validate at runtime
- **Alternative**: Require PklProject at plugin root (more complex)

Research showed formae uses embedded FS with custom URI scheme (`formae:/Config.pkl`) for configs.

### 2. Struct Naming Convention
- Currently: `Plugin` struct in ovh.go
- Consider: Rename to `OVH` to avoid confusion with `plugin.ResourcePlugin` interface
- **Action**: Test with AWS plugin extraction

### 3. Test Strategy
- `make test-unit` uses `-tags=unit` but no tests have that tag
- Only have integration (`-tags=integration`) and conformance (`-tags=conformance`) tests

### 4. sed Cross-Platform Compatibility
- Current fix uses `sed -i ''` which works on macOS but NOT on Linux (GNU sed)
- Need a cross-platform solution in `scripts/run-conformance-tests.sh`
- Options:
  a. Detect OS and use appropriate sed syntax
  b. Use perl instead: `perl -i -pe 's/.../.../g'`
  c. Use temp file approach: `sed 's/.../.../g' file > tmp && mv tmp file`
- **Action**: Fix in both OVH plugin and template before CI runs on Linux

---

## Files Modified

### OVH Plugin
- `formae-plugin.pkl` - simplified flat manifest
- `ovh.go` - full implementation
- `Makefile` - fixed namespace extraction
- `scripts/run-conformance-tests.sh` - fixed sed
- `scripts/ci/setup-credentials.sh` - OpenStack validation
- `scripts/ci/clean-environment.sh` - resource cleanup
- `.env.ovh` - credentials (gitignored)
- `.plans/extraction-plan.md` - detailed plan

### Template (`/Users/jeroen/dev/pel/formae-plugin-template/`)
- `formae-plugin.pkl` - simplified
- `Makefile` - fixed namespace
- `scripts/run-conformance-tests.sh` - fixed sed

### Formae (`/Users/jeroen/dev/pel/formae/`)
- `plugins/pkl/assets/formae/PluginManifest.pkl` - created (usage TBD)

---

## Next Steps

1. **Verify conformance tests pass** locally
2. **Enable CI workflow** in `.github/workflows/ci.yml`
3. **Push to GitHub** as `platform-engineering-labs/formae-plugin-ovh`
4. **Decide on manifest schema pattern** (see open questions)
5. **Merge formae PR** (`refactor/plugin-externalization`)
6. **Extract AWS plugin** using same process

---

## Useful Commands

```bash
cd /Users/jeroen/dev/pel/formae-plugin-ovh

# Build
make build

# Run tests
source .env.ovh
make conformance-test

# Check installed plugin
ls -la ~/.pel/formae/plugins/OVH/v0.1.0/
```

---

## Reference Locations

- OVH Plugin: `/Users/jeroen/dev/pel/formae-plugin-ovh`
- Template: `/Users/jeroen/dev/pel/formae-plugin-template`
- Backup: `/Users/jeroen/dev/pel/formae-plugin-ovh.bak`
- Formae: `/Users/jeroen/dev/pel/formae`
- Internal (OVH branch): `/Users/jeroen/dev/pel/formae-internal` branch `feat/ovh-plugin`
