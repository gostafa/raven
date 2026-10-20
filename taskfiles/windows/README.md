# Windows Release Tasks

From the repository root on Windows, with Task installed:

```sh
task windows:release TAG=v1.2.3 ARCH=amd64
task windows:release TAG=v1.2.3 ARCH=arm64
```

Run architectures sequentially in a local checkout. The GitHub release workflow
uses separate matrix runners.

`release` validates the stable tag and architecture before installing tools. It
uses TaskOtter installers for Go and Bun, selects the Go toolchain from `go.mod`,
requires the Bun version from `package.json`, and installs Wails at the module's
pinned version. The existing packaging tasks install .NET and restore local WiX.
All build and packaging logic stays in Taskfiles.

Packaging verifies the MSI and executable, promotes the MSI and SHA-256 checksum
to `bin/`, and requires clean tracked sources before reporting success. For
`TAG=v1.2.3 ARCH=amd64`, outputs are:

- Artifact name: `raven-v1.2.3-windows-amd64`
- Installer: `bin/raven-v1.2.3-windows-amd64.msi`
- Checksum: `bin/raven-v1.2.3-windows-amd64.msi.sha256`

The tag includes `v`; executable and installer version metadata use `1.2.3`.
Stable versions must fit MSI limits `255.255.65535` without leading zeroes.
Both `TAG` and `ARCH` are required for releases.

When `GITHUB_OUTPUT` is set, the task writes `artifact-name`, `msi-name`,
`checksum-name`, `msi-path`, and `checksum-path` after successful verification.
The local `build-windows` composite action installs Task, calls this task, uploads
both exact files, and exposes the names plus `artifact-id` and `artifact-url`.
Callers must check out the repository before invoking the local action.

Existing `task windows:package VERSION=1.2.3 ARCH=amd64` usage remains available
with numeric filenames and requires build tooling to be installed already.
