# Work In Progress Test Files

These test files have been moved here because they currently don't pass conformance tests.

## Status Summary

| File | Issue |
|------|-------|
| `instance.pkl` | CRUD passes, but **destroy fails** - dependency ordering issue. Subnet deletion attempted before instance port cleanup completes. Error: "One or more ports have an IP allocation from this subnet" |
| `instance-update.pkl` | Same as above - part of instance CRUD cycle |
| `instance-replace.pkl` | Same as above - part of instance CRUD cycle |
| `securitygroup.pkl` | **Quota exceeded** - previous test runs left orphan security groups that weren't cleaned up, exhausting the quota |
| `securitygrouprule.pkl` | **Cascade failure** - depends on `securitygroup.pkl` succeeding first |
| `securitygrouprule-replace.pkl` | **Cascade failure** - depends on `securitygroup.pkl` succeeding first |

## Root Causes

### Instance Tests

Formae's destroy changeset doesn't respect the dependency graph (instance -> subnet -> network). The subnet can't be deleted while the instance still has a port allocated from it.

### Security Group Tests

Environment state issue - need to manually clean up orphan security groups in OVH console, or the cleanup script needs to handle them properly.
