# System Buildpacks

## Overview

System buildpacks are special buildpacks that are integrated into the builder and are automatically included in the build process. They run before (pre) and after (post) the regular buildpacks in the build order, providing platform-level functionality like shell profile scripts, service binding, and other capabilities that enhance the build process.

## Configuring System Buildpacks

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

### Configuration Options

- `system.pre.buildpacks`: Array of buildpacks to run before the regular buildpacks
- `system.post.buildpacks`: Array of buildpacks to run after the regular buildpacks
- Each buildpack entry requires:
  - `id`: The buildpack ID
  - `version`: The buildpack version
  - `optional`: Boolean indicating if detection failure should be tolerated
    - `false` (default): Detection failure will cause the build to fail
    - `true`: Detection can fail without failing the build

## How System Buildpacks Work

1. **Pre-System Buildpacks**: These are run before the regular buildpacks during the detection phase. They can set up environment variables, modify the filesystem, or perform other preparations needed by the main buildpacks.

2. **Regular Buildpacks**: The buildpacks specified in the builder's order are run as normal.

3. **Post-System Buildpacks**: These run after the regular buildpacks, allowing for post-processing tasks like finalizing configurations or adding additional layers.

## Detection Process

System buildpacks participate in the buildpack detection process:

- For each detection group, pre-system buildpacks are prepended and post-system buildpacks are appended
- Non-optional system buildpacks must pass detection for a group to be valid
- Optional system buildpacks can pass or fail without blocking group selection

## CLI Options

The behavior of system buildpacks can be controlled with the `--disable-system-buildpacks` flag:

```shell
pack build my-app --disable-system-buildpacks
```

When this flag is set, no system buildpacks will be included in the build process, regardless of what's configured in the builder.

## Use Cases

System buildpacks are ideal for:

1. **Platform Customization**: Adding default behaviors that should apply to all builds
2. **Profile Scripts**: Implementing shell profile scripts without requiring app modifications
3. **Service Binding**: Automatically connecting applications to required services 
4. **Monitoring/Observability**: Adding default monitoring capabilities to all applications
5. **Security Scanning**: Implementing security checks before or after the main build process

## Requirements for System Buildpacks

System buildpacks must:

1. Conform to the Buildpack API
2. Be regular buildpacks, not meta-buildpacks (cannot have order entries)
3. Be clearly identified as pre or post buildpacks in the builder configuration

## Limitations

- System buildpacks are only used during detection and build phases, not at runtime
- They cannot override buildpacks explicitly specified by the user
- They must be included in the builder image; they cannot be added at build time