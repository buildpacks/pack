## PR Description: Fix Digest Errors on Publish-Pull

### Problem
When using `pack` as a library within `octopilot-pipeline-tools` (`op`), we encountered digest mismatch errors during the `publish` phase, specifically in environments using `containerd` or when attempting to immediately pull a just-published image.

This issue (referenced as #2272 in upstream discussions) prevents reliable multi-arch builds and promotions.

### Changes
-   **Publish-Then-Pull Workaround**: Implemented an optional logic to handle the publish-then-pull sequence more robustly.
-   **Library Exposure**: Exposed internal `BuildOptions` and registry handling logic to allow `op` to configure authentication and lifecycle behavior programmatically.

### Verification
Verified integration within `op`. The tool can now successfully build images using buildpacks and push them to a registry without encountering digest errors, even in diverse container runtime environments.
