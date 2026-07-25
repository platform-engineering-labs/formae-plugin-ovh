# Changelog

All notable changes to the formae OVHcloud plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install ovh` on the host that runs the
formae agent.

## [Unreleased]

### Changed

- The `oidcClientSecret` on `OVH::Registry::Oidc` is now typed `formae.SecretValue` so its value is hashed at rest end-to-end (previously stored in cleartext on the read/actual-state path). Requires a formae agent on the matching release; `minFormaeVersion` is bumped to 0.88.0.

## [0.1.4]

### Fixed

- The OVH API client now sets an HTTP timeout on outgoing requests. Previously,
  an unresponsive OVH endpoint could cause apply, destroy, or sync operations to
  hang indefinitely instead of failing and surfacing a clear error.

## [0.1.3]

### Changed

- The `ovhEndpoint`, `applicationKey`, `applicationSecret`, and `consumerKey`
  fields are now mutable; changing them updates the target in place without
  recreating resources. The `region` and `projectId` fields remain immutable;
  changing them triggers a full target replace as before.

## [0.1.2]

### Fixed

- Volumes deleted outside of formae are now reliably removed from the inventory
  after synchronization. Previously, out-of-band volume deletions could go
  undetected indefinitely because the OVH API reports deleted volumes with a
  `deleted` status rather than returning a `404`.
- Private subnet provisioning no longer fails intermittently with "network does
  not exist" errors. The plugin now verifies the parent network is fully
  propagated across OVH API endpoints before reporting it as ready, preventing a
  race condition caused by eventual consistency between the network and subnet
  APIs.
- Spurious diffs during updates and synchronization for provider-populated
  fields across multiple resource types: network `shared` and `mtu`, port
  `security_groups`, `description` and `mac_address`, subnet `allocation_pools`,
  private network `vlanId`, and S3 bucket `ownerId` and `objectLock`.

## [0.1.1]

### Added

- S3-compatible bucket support with schema definitions and discovery for OVH
  Object Storage buckets.

### Fixed

- Bucket storage operations after field renames in the upstream API.
- Private subnet reads that returned incomplete data.
- The `ResponseTransformer` in apply status handling, which could cause commands
  to report incorrect status.

## [0.1.0]

### Added

- Initial release of the OVH plugin as a standalone package built on the formae
  Plugin SDK.
