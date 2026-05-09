# Contributing

This document covers local development for plugin authors. For user-facing
plugin docs (configuration, supported resources, examples), see
[README.md](README.md).

## Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html) 0.30+
- OVH Public Cloud credentials (for integration/conformance testing)

## Local Installation

```bash
make install
```

## Building

```bash
make build      # Build plugin binary
make test-unit  # Run unit tests
make lint       # Run linter
make install    # Build + install locally
```

## Local Testing

```bash
# Install plugin locally
make install

# Start formae agent
formae agent start

# Apply example resources
formae apply --mode reconcile --watch examples/lifeline/basic_infrastructure.pkl
```

## Conformance Testing

Run the full CRUD lifecycle + discovery tests:

```bash
make conformance-test                     # Latest formae version
make conformance-test VERSION=0.82.1      # Specific version
make conformance-test TEST=privatesubnet  # Filter by resource name
```

To skip the formae binary download (useful for slow connections or local
development), set `FORMAE_BINARY` to point at a local build:

```bash
FORMAE_BINARY=/path/to/formae/bin/formae make conformance-test TEST=privatesubnet
```

The `scripts/ci/clean-environment.sh` script cleans up test resources. It runs
before and after conformance tests and is idempotent.
