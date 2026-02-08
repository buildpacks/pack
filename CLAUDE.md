# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Pack is a CLI tool that helps build applications using Cloud Native Buildpacks. It provides functionality for app developers to convert code into runnable images, for buildpack authors to develop and package buildpacks, and for operators to package buildpacks and maintain applications.

Pack is an implementation of the [Platform](https://github.com/buildpacks/spec/blob/main/platform.md) specification at Cloud Native Buildpack. it is mandatory to its feature are complaince with the spec 

## Repository Structure

The repository follows a standard Go project structure:

- `/cmd` - Contains the main application entry point
- `/internal` - Internal packages only used within this project
  - `/commands` - CLI commands implementation
  - `/builder` - Builder-related functionality
  - `/config` - Configuration handling
  - `/inspectimage` - Image inspection logic
- `/pkg` - Shared packages that could be used by external projects
  - `/archive` - Archive handling utilities
  - `/blob` - Blob operations
  - `/buildpack` - Buildpack-related operations
  - `/client` - Client implementation for pack operations
  - `/image` - Image manipulation
  - `/logging` - Logging utilities
- `/acceptance` - Acceptance tests

## Development Workflow

### Building the Project

```bash
# Build the project
make build

# The binary will be available at out/pack
```

### Testing

```bash
# Run unit tests
make unit

# Run acceptance tests
make acceptance

# Run all tests (unit + acceptance)
make test

# Run full acceptance suite (including cross-compatibility for n-1 pack and lifecycle)
make acceptance-all
```

### Code Quality and Formatting

```bash
# Format the code
make format

# Tidy up the codebase and dependencies
make tidy

# Verify formatting and code quality
make verify

# Run all checks to prepare for a PR
make prepare-for-pr
```

## Development Environment Setup

### Prerequisites
- Git
- Go
- Docker
- Make

### Environment Variables for Building

| ENV_VAR      | Description                                                            | Default |
|--------------|------------------------------------------------------------------------|---------|
| GOCMD        | Change the `go` executable. For example, [richgo][rgo] for testing.    | go      |
| PACK_BIN     | Change the name or location of the binary relative to `out/`.          | pack    |
| PACK_VERSION | Tell `pack` what version to consider itself                            | `dev`   |

### Environment Variables for Acceptance Tests

| ENV_VAR      | Description                                                             | Default |
|--------------|-------------------------------------------------------------------------|---------|
| ACCEPTANCE_SUITE_CONFIG   | Configurations for acceptance tests                         | `[{"pack": "current", "pack_create_builder": "current", "lifecycle": "default"}]'` |
| COMPILE_PACK_WITH_VERSION | Tell `pack` what version to consider itself                 | `dev` |
| GITHUB_TOKEN | Github Token for downloading releases during test setup                 | "" |
| LIFECYCLE_IMAGE | Image reference for untrusted builder workflows                       | docker.io/buildpacksio/lifecycle:<lifecycle version> |
| LIFECYCLE_PATH | Path to a `.tgz` file with lifecycle binaries                         | Default version from Github |
| PACK_PATH | Path to a `pack` executable                                               | Compiled version of current branch |

## Common Patterns and Practices

1. The project uses the Cobra library for CLI command implementation
2. It follows Go idiomatic patterns for error handling 
3. The internal architecture separates CLI command handling from core functionality
4. Testing is done using the `sclevine/spec` library for test organization

## Key Components

1. Client (`pkg/client`) - Core functionality for pack operations
2. Commands (`internal/commands`) - Implementation of CLI commands 
3. Buildpack handling (`pkg/buildpack`) - Operations with buildpacks
4. Image management (`pkg/image`) - Operations with container images

## System Buildpacks Feature

System buildpacks are special buildpacks defined in the builder's configuration that are automatically included before (pre) and after (post) the regular buildpacks during a build. They provide functionality like shell profile scripts, service binding, and other platform-specific capabilities that enhance the build process.

### Configuration

System buildpacks are configured in the `builder.toml` file under the `[system]` section:

```toml
[system]
[system.pre]
buildpacks = [
  { id = "example/env-buildpack", version = "0.0.1", optional = false }
]

[system.post]
buildpacks = [
  { id = "example/read-env-buildpack", version = "0.0.1", optional = true }
]
```

- `system.pre.buildpacks`: Buildpacks to run before the regular buildpacks in order
- `system.post.buildpacks`: Buildpacks to run after the regular buildpacks in order
- The `optional` flag determines if detection failure of the buildpack will fail the entire build

### Build Process

1. During the build process, the lifecycle will first run all the pre-system buildpacks
2. Then the regular buildpacks from the builder's order are run
3. Finally, the post-system buildpacks are run

### CLI Options

The behavior of system buildpacks can be controlled with the `--disable-system-buildpacks` flag:

```shell
pack build my-app --disable-system-buildpacks
```

When this flag is set, no system buildpacks will be included in the build process.