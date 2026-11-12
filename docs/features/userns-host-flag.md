# UserNS Host Mode Control

## Overview

Pack 0.35.0 introduced a change that enabled user namespace isolation for build containers by default. While this enhanced security, some Docker plugins that prevent using "userns as host" were not compatible with this change, causing users to be limited to older versions of Pack.

Starting with Pack 0.38.0, user namespace isolation is disabled by default, and can be enabled using the `--userns-host` flag.

## Usage

To enable user namespace isolation when building:

```bash
pack build my-app --userns-host
```

## Technical Details

When the `--userns-host` flag is used, Pack will set the Docker container's `UsernsMode` to `host`, which instructs Docker to use the host's user namespace for the container. This can provide better isolation between the container and the host.

Without this flag, the user namespace setting will not be specified, allowing normal operation for users with Docker plugins that don't support this feature.

## Compatibility

This flag is compatible with all supported versions of Docker, but whether it works successfully depends on your specific Docker configuration and plugins.