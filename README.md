# OVH Cloud Plugin for Formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-ovh/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-ovh/actions/workflows/ci.yml)
[![Nightly](https://github.com/platform-engineering-labs/formae-plugin-ovh/actions/workflows/nightly.yml/badge.svg?branch=main)](https://github.com/platform-engineering-labs/formae-plugin-ovh/actions/workflows/nightly.yml)

OVH Cloud resource plugin for [Formae](https://github.com/platform-engineering-labs/formae). This plugin enables Formae to manage OVH Public Cloud resources using the OpenStack APIs via [gophercloud](https://github.com/gophercloud/gophercloud).

## Installation

```bash
# Install the plugin
make install
```

## Supported Resources

This plugin supports **10 OVH Public Cloud resource types** across 3 services:

| Type | Discoverable | Extractable | Comment |
|------|--------------|-------------|----------|
| OVH::Compute::Instance | ✅ | ✅ |  |
| OVH::Compute::SSHKey | ✅ | ✅ |  |
| OVH::Compute::Volume | ✅ | ✅ |  |
| OVH::Compute::VolumeAttachment | ✅ | ✅ |  |
| OVH::Compute::VolumeSnapshot | ✅ | ✅ |  |
| OVH::DNS::Record | ✅ | ✅ |  |
| OVH::DNS::Redirection | ✅ | ✅ |  |
| OVH::DNS::Zone | ✅ | ✅ |  |
| OVH::Database::Database | ✅ | ✅ |  |
| OVH::Database::Integration | ✅ | ✅ |  |
| OVH::Database::IpRestriction | ✅ | ✅ |  |
| OVH::Database::KafkaAcl | ✅ | ✅ |  |
| OVH::Database::KafkaTopic | ✅ | ✅ |  |
| OVH::Database::PostgresqlConnectionPool | ✅ | ✅ |  |
| OVH::Database::Service | ✅ | ✅ |  |
| OVH::Database::User | ✅ | ✅ |  |
| OVH::Kube::Cluster | ✅ | ✅ |  |
| OVH::Kube::IpRestriction | ✅ | ✅ |  |
| OVH::Kube::NodePool | ✅ | ✅ |  |
| OVH::Kube::Oidc | ✅ | ✅ |  |
| OVH::Network::FloatingIP | ✅ | ✅ |  |
| OVH::Network::Gateway | ✅ | ✅ |  |
| OVH::Network::Network | ✅ | ✅ |  |
| OVH::Network::Port | ✅ | ✅ |  |
| OVH::Network::PrivateNetwork | ✅ | ✅ |  |
| OVH::Network::PrivateSubnet | ✅ | ✅ |  |
| OVH::Network::Router | ✅ | ✅ |  |
| OVH::Network::SecurityGroup | ✅ | ✅ |  |
| OVH::Network::SecurityGroupRule | ✅ | ✅ |  |
| OVH::Network::Subnet | ✅ | ✅ |  |
| OVH::Registry::IpRestriction | ✅ | ✅ |  |
| OVH::Registry::Oidc | ✅ | ✅ |  |
| OVH::Registry::Registry | ✅ | ✅ |  |
| OVH::Registry::User | ✅ | ✅ |  |
| OVH::Storage::Container | ✅ | ✅ |  |
| OVH::Storage::S3Bucket | ✅ | ✅ |  |

See [`schema/pkl/`](schema/pkl/) for the complete list of supported resource types.

## Configuration

### Target Configuration

Configure an OVH target in your Forma file:

```pkl
import "@formae/formae.pkl"
import "@ovh/ovh.pkl"

target: formae.Target = new formae.Target {
    label = "ovh-target"
    config = new ovh.Config {
        authURL = "https://auth.cloud.ovh.net/v3"  // EU regions
        // authURL = "https://auth.cloud.ovh.us/v3"  // US regions
        region = "GRA7"  // See supported regions below
    }
}
```

**Supported Regions:**
- `BHS5` - Beauharnois, Canada
- `DE1` - Frankfurt, Germany
- `GRA7`, `GRA9` - Gravelines, France
- `SBG5` - Strasbourg, France
- `UK1` - London, UK
- `WAW1` - Warsaw, Poland
- `US-EAST-VA-1` - Virginia, USA

### Credentials

This plugin requires **two sets of credentials**:

1. **OVH Cloud API** — for OVH-specific resources (DNS, Database, Kube, Registry)
2. **OpenStack API** — for infrastructure resources (Compute, Network, Storage)

#### OVH Cloud API Credentials

```bash
export OVH_ENDPOINT="ovh-eu"              # Optional: ovh-eu (default), ovh-ca, ovh-us
export OVH_APPLICATION_KEY="your-app-key"
export OVH_APPLICATION_SECRET="your-app-secret"
export OVH_CONSUMER_KEY="your-consumer-key"
export OVH_CLOUD_PROJECT_ID="your-project-id"
```

**Getting OVH API Credentials:**

| Region | Create Application | Create Token |
|--------|--------------------|-------------|
| EU | https://eu.api.ovh.com/createApp/ | https://eu.api.ovh.com/createToken/ |
| US | https://api.us.ovhcloud.com/createApp/ | https://api.us.ovhcloud.com/createToken/ |
| CA | https://ca.api.ovh.com/createApp/ | https://ca.api.ovh.com/createToken/ |

1. **Create an application** at the `/createApp/` URL for your region — note the Application Key and Secret
2. **Request a consumer key** using the API (the `/createToken/` web UI may not work for all account types):
   ```bash
   curl -X POST "https://eu.api.ovh.com/1.0/auth/credential" \
     -H "X-Ovh-Application: YOUR_APP_KEY" \
     -H "Content-Type: application/json" \
     -d '{"accessRules":[{"method":"GET","path":"/*"},{"method":"POST","path":"/*"},{"method":"PUT","path":"/*"},{"method":"DELETE","path":"/*"}]}'
   ```
   For US, replace the URL with `https://api.us.ovhcloud.com/1.0/auth/credential`.

   This returns a `consumerKey` and a `validationUrl`.
3. **Validate the consumer key** by opening the `validationUrl` in your browser and approving access. The consumer key will not work until validated.
4. Find your Cloud Project ID in the OVH Control Panel under Public Cloud:
   - EU: https://www.ovh.com/manager/
   - US: https://us.ovhcloud.com/manager/

> **Note:** Accounts are region-specific. EU credentials do not work on the US endpoint and vice versa. You must create the application and token on the same regional endpoint.

#### OpenStack API Credentials

```bash
export OS_USERNAME="your-openstack-username"
export OS_PASSWORD="your-openstack-password"
export OS_PROJECT_ID="your-project-id"
export OS_USER_DOMAIN_NAME="Default"       # Optional, defaults to "Default"
```

**Getting OpenStack Credentials:**
1. Go to the [OVH Control Panel](https://www.ovh.com/manager/)
2. Navigate to Public Cloud > Project > Users & Roles
3. Create a new user or use an existing one
4. Download the OpenStack RC file or note the credentials

## Examples

See the [examples/](examples/) directory for usage examples.

```bash
# Evaluate an example
formae eval examples/lifeline/basic_infrastructure.pkl

# Apply resources
formae apply --mode reconcile --watch examples/lifeline/basic_infrastructure.pkl
```

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html) 0.30+
- OVH Public Cloud credentials (for integration/conformance testing)

### Building

```bash
make build      # Build plugin binary
make test-unit  # Run unit tests
make lint       # Run linter
make install    # Build + install locally
```

### Local Testing

```bash
# Install plugin locally
make install

# Start formae agent
formae agent start

# Apply example resources
formae apply --mode reconcile --watch examples/lifeline/basic_infrastructure.pkl
```

### Conformance Testing

Run the full CRUD lifecycle + discovery tests:

```bash
make conformance-test                  # Latest formae version
make conformance-test VERSION=0.82.1   # Specific version
make conformance-test TEST=privatesubnet  # Filter by resource name
```

To skip the formae binary download (useful for slow connections or local development), set `FORMAE_BINARY` to point at a local build:

```bash
FORMAE_BINARY=/path/to/formae/bin/formae make conformance-test TEST=privatesubnet
```

The `scripts/ci/clean-environment.sh` script cleans up test resources. It runs before and after conformance tests and is idempotent.

## License

This plugin is licensed under the [Functional Source License, Version 1.1, ALv2 Future License (FSL-1.1-ALv2)](LICENSE).

Copyright 2025 Platform Engineering Labs Inc.
