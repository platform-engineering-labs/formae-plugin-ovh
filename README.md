# Formae Plugin Template

Template repository for creating formae resource plugins.

> **Note:** Don't use GitHub's "Use this template" button. Instead, use the Formae CLI
> which will prompt for your plugin details and set everything up correctly:
>
> ```bash
> formae plugin init my-plugin
> ```

## Quick Start

1. **Create plugin**: `formae plugin init <name>` (prompts for namespace, license, etc.)
2. **Define resources** in `schema/pkl/*.pkl`
3. **Implement CRUD operations** in `plugin.go`
4. **Build and test**: `make build && make test`

## Project Structure

```
.
├── formae-plugin.pkl      # Plugin manifest (name, version, namespace)
├── plugin.go              # Your ResourcePlugin implementation
├── main.go                # Entry point (don't modify)
├── schema/pkl/            # Pkl resource schemas
│   ├── PklProject
│   └── example.pkl
├── examples/              # Usage examples
├── scripts/
│   ├── ci/                # CI hook scripts
│   │   ├── setup-credentials.sh
│   │   └── clean-environment.sh
│   └── run-conformance-tests.sh
├── go.mod
├── Makefile
└── README.md
```

## What You Implement

You only implement the `ResourcePlugin` interface in `plugin.go`:

```go
type Plugin struct{}

// Configuration
func (p *Plugin) RateLimit() plugin.RateLimitConfig { ... }
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter { ... }
func (p *Plugin) LabelConfig() plugin.LabelConfig { ... }

// CRUD Operations
func (p *Plugin) Create(ctx, req) (*CreateResult, error) { ... }
func (p *Plugin) Read(ctx, req) (*ReadResult, error) { ... }
func (p *Plugin) Update(ctx, req) (*UpdateResult, error) { ... }
func (p *Plugin) Delete(ctx, req) (*DeleteResult, error) { ... }
func (p *Plugin) Status(ctx, req) (*StatusResult, error) { ... }
func (p *Plugin) List(ctx, req) (*ListResult, error) { ... }
```

**The SDK handles everything else:**
- Plugin identity (name, version, namespace) → read from `formae-plugin.pkl`
- Schema extraction → auto-discovered from `schema/pkl/`
- Resource descriptors → generated from Pkl schemas

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)

### Building

```bash
make build      # Build plugin binary
make test       # Run unit tests
make lint       # Run linter (requires golangci-lint)
make install    # Build + install locally for testing
```

### Local Testing

```bash
# Install plugin and schemas locally
make install

# Start formae agent (discovers the plugin)
formae agent start

# Apply example resources
formae apply examples/basic/main.pkl
```

### Conformance Testing

Run the full conformance test suite (CRUD lifecycle + discovery) against a specific formae version:

```bash
# Run conformance tests with latest stable version
make conformance-test

# Run conformance tests with a specific version
make conformance-test VERSION=0.77.0
```

The conformance tests:
1. Call `setup-credentials` to provision cloud credentials
2. Call `clean-environment` to remove orphaned resources from previous runs
3. Build and install the plugin locally
4. Download the specified formae version (defaults to latest)
5. Run CRUD lifecycle tests for each resource type
6. Run discovery tests to verify resource detection
7. Call `clean-environment` to clean up test resources

### CI Hooks

The template includes hook scripts that you customize for your cloud provider:

#### `scripts/ci/setup-credentials.sh`

Provisions credentials for your cloud provider. Called before running conformance tests.

**Examples:**
- AWS: Verify `AWS_ACCESS_KEY_ID` is set or use OIDC
- OpenStack: Source your RC file and verify required env vars
- Azure: Run `az login` or verify OIDC credentials
- GCP: Run `gcloud auth` or verify workload identity

#### `scripts/ci/clean-environment.sh`

Cleans up test resources in your cloud environment. Called before AND after conformance tests to:
- Remove orphaned resources from previous failed runs (pre-cleanup)
- Clean up resources created during the test run (post-cleanup)

The script should be idempotent and delete all resources under the /testdata folder

#### GitHub Actions

The `.github/workflows/ci.yml` workflow includes a `conformance-tests` job that is
disabled by default. To enable it:

1. Configure credentials for your cloud provider in the workflow
2. Implement the hook scripts for local verification
3. Set `run_conformance` to `true` when triggering the workflow, or modify the `if` condition

See the workflow file for credential configuration examples for AWS, Azure, GCP, and OpenStack.

## Defining Resources (Pkl)

Create resource classes in `schema/pkl/`:

```pkl
@formae.ResourceHint {
    type = "MYPROVIDER::Service::Resource"
    identifier = "$.Id"
}
class MyResource extends formae.Resource {
    @formae.FieldHint {}
    name: String

    @formae.FieldHint { createOnly = true }
    region: String?
}
```

## Plugin Manifest

All plugin metadata lives in `formae-plugin.pkl`:

```pkl
amends "@formae/plugin-manifest.pkl"

name = "myprovider"           # Plugin identifier
version = "1.0.0"             # Semantic version
description = "My cloud provider plugin"

spec {
    protocolVersion = 1       # SDK protocol version
    namespace = "MYPROVIDER"  # Resource type prefix
    capabilities { "create"; "read"; "update"; "delete"; "list"; "discovery" }
}
```

## Async (long-running) Operations

All plugin operations return the `ProgressResult` struct. For async (long-running) operations
return `InProgress` with a `RequestID`. The formae agent will call the `Status` method on
a regular interval to request the status of the operation.

```go
func (p *Plugin) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
    operationID := startAsyncCreate(...)

    return &resource.CreateResult{
        ProgressResult: &resource.ProgressResult{
            Operation:       resource.OperationCreate,
            OperationStatus: resource.OperationStatusInProgress,
            RequestID:       operationID,
        },
    }, nil
}

func (p *Plugin) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
    status := checkOperation(req.RequestID)
    if status.Complete {
        return &resource.StatusResult{
            ProgressResult: &resource.ProgressResult{
                OperationStatus: resource.OperationStatusSuccess,
                NativeID:        status.ResourceID,
            },
        }, nil
    }
    // Still in progress - return InProgress status
}
```

## License

This template is licensed under FSL-1.1-ALv2 - See [LICENSE](LICENSE)

When creating your own plugin, choose an appropriate license for your project.
Common choices include:
- **MIT** - Most permissive
- **Apache-2.0** - Permissive with patent grant (recommended)
- **MPL-2.0** - Weak copyleft
- **FSL-1.1-ALv2** - Functional Source License

Replace the LICENSE file with your chosen license when you create your plugin.
