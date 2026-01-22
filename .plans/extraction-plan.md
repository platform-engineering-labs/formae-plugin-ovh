# OVH Plugin Extraction Plan

## Goal
Extract the OVH plugin into a standalone repository using `formae plugin init` and the plugin template.

## Status: In Progress - Conformance Tests Running

---

## Completed Tasks

1. **Scaffolded OVH plugin** with `formae plugin init`
2. **Copied implementation code** from backup:
   - `pkg/` - client, config, prov, registry, resources
   - `ovh.go` - ResourcePlugin implementation
   - `schema/pkl/` - resource schemas
   - `testdata/` - conformance test data
3. **Updated go.mod** with gophercloud and formae SDK dependencies
4. **Implemented CI hooks**:
   - `scripts/ci/setup-credentials.sh` - validates OS_* env vars
   - `scripts/ci/clean-environment.sh` - cleans up test resources
5. **Fixed Makefile pkl eval issue**:
   - Simplified `formae-plugin.pkl` to flat structure (no `amends`)
   - Changed Makefile to use `namespace` instead of `spec.namespace`
6. **Updated template** with same fixes
7. **Fixed sed macOS compatibility** in `run-conformance-tests.sh` (`sed -i ''`)
8. **Created `.env.ovh`** credentials file (gitignored)

---

## Pending Tasks

### Immediate
- [ ] Complete conformance tests locally
- [ ] Enable CI workflow and push to GitHub

### Open Questions

1. **Plugin manifest schema pattern**
   - Created `PluginManifest.pkl` in formae (`plugins/pkl/assets/formae/`)
   - Challenge: `pkl eval` in Makefile can't resolve `formae://` URIs (only works with formae's custom evaluator)
   - Options:
     a. Keep flat standalone manifest (current approach, validated at runtime)
     b. Require PklProject at plugin root
     c. Use different Makefile approach (grep/sed)
   - Research showed formae uses embedded FS with custom URI scheme for configs

2. **Struct naming convention**
   - Currently uses `Plugin` struct name
   - Consider renaming to actual plugin name (e.g., `OVH`)
   - Test with AWS plugin extraction

3. **Test strategy**
   - `make test-unit` uses `-tags=unit` but no tests have that tag
   - Only have integration and conformance tests

4. **sed cross-platform compatibility**
   - Current fix (`sed -i ''`) works on macOS but NOT on Linux
   - Need to fix before CI runs on Linux runners
   - Options: detect OS, use perl, or temp file approach

---

## Key Files Modified

### OVH Plugin (`/Users/jeroen/dev/pel/formae-plugin-ovh/`)
- `formae-plugin.pkl` - Flat manifest (name, version, namespace, license, minFormaeVersion)
- `ovh.go` - ResourcePlugin implementation with registry pattern
- `Makefile` - Fixed namespace extraction
- `scripts/run-conformance-tests.sh` - Fixed sed macOS compatibility
- `scripts/ci/setup-credentials.sh` - OpenStack credential validation
- `scripts/ci/clean-environment.sh` - Resource cleanup script
- `.env.ovh` - Local credentials (gitignored)

### Template (`/Users/jeroen/dev/pel/formae-plugin-template/`)
- `formae-plugin.pkl` - Simplified to flat structure
- `Makefile` - Fixed namespace extraction
- `scripts/run-conformance-tests.sh` - Fixed sed macOS compatibility

### Formae (`/Users/jeroen/dev/pel/formae/`)
- `plugins/pkl/assets/formae/PluginManifest.pkl` - Created (pending usage decision)

---

## Credentials

OVH credentials stored in `.env.ovh`:
```bash
source .env.ovh
make conformance-test
```

Required env vars: `OS_AUTH_URL`, `OS_USERNAME`, `OS_PASSWORD`, `OS_PROJECT_ID`, `OS_REGION_NAME`

---

## Post-Extraction Steps

1. Merge formae PR (`refactor/plugin-externalization`)
2. Update template dependencies to released versions (not commit hashes)
3. Test plugin e2e with formae agent
4. Extract AWS plugin using same process
