# Linux Release Tasks

Use a native Ubuntu 24.04 host matching the target architecture: AMD64 or ARM64.
Install Task, the Go version in `go.mod`, and the Bun version in `package.json`.
From the repository root:

```sh
task linux:install:deps ARCH=amd64
task linux:release TAG=v1.2.3 ARCH=amd64
```

Use `ARCH=arm64` on ARM64. Cross compilation is rejected because Raven links
GTK4 and WebKitGTK 6.0 through CGO. GitHub builds on separate `ubuntu-24.04` and
`ubuntu-24.04-arm` runners. The composite `build-linux` action requires checkout
first, sets up pinned Go/Bun versions and Task, caches dependencies, installs
native tools, then calls `linux:release`.

`install:deps` uses sudo to install native libraries and packaging tools through
APT.
`install:tooling` installs the module-pinned Wails CLI in `.tools/`; Linux tasks
add this directory to their own PATH.

`release` requires a clean tracked working tree and a stable `vMAJOR.MINOR.PATCH`
tag, with no leading zeroes or suffixes. Versions share Windows' limits of
`255.255.65535`. `DEV=true`, unsupported architectures, and host mismatches fail
before building. For local packaging with tooling already installed:

```sh
task linux:install:tooling ARCH=amd64
task linux:package VERSION=1.2.3 ARCH=amd64
```

The frontend and production executable build once. All formats use that payload.
Intermediate metadata, package candidates, extracted payloads,
and startup logs live under `bin/.linux/ARCH/`. Run packaging sequentially in a
checkout; CI uses a fresh checkout for each architecture. Individual
`linux:create:deb`, `linux:create:rpm`, and `linux:create:pacman` tasks take the same version and
architecture arguments. `linux:create:aur` remains an alias for pacman packaging;
it does not publish to AUR.

For `TAG=v1.2.3 ARCH=amd64`, promoted files are:

- `bin/raven-v1.2.3-linux-amd64.deb`
- `bin/raven-v1.2.3-linux-amd64.rpm`
- `bin/raven-v1.2.3-linux-amd64.pkg.tar.zst`

Each file has an adjacent `.sha256` sidecar. Verification checks package
metadata, architecture, extracted executable identity, and desktop metadata.
Each format must show a Raven window under Xvfb and remain alive before
promotion. Startup logs remain in the staging directory on
failure. No packages are promoted until every selected format passes.

When `GITHUB_OUTPUT` is set, release writes `artifact-name`, `package-paths`, and
`checksum-paths` only after all checks pass. The action uploads exactly those
packages and sidecars and exposes `artifact-id` and `artifact-url` as well.
Manual Release workflow runs upload artifacts without publishing. Stable tag
pushes publish only after both Windows and both Linux builds succeed. The
publisher verifies the checksums again before publishing installers and sidecars.
No AUR credentials are required.

## Installing Packages

Download the package for your architecture and its `.sha256` sidecar, then run
`sha256sum --check FILENAME.sha256` before installation. Examples:

```sh
sudo apt install ./raven-v1.2.3-linux-amd64.deb
sudo dnf install ./raven-v1.2.3-linux-amd64.rpm
sudo pacman -U ./raven-v1.2.3-linux-amd64.pkg.tar.zst
```

Choose one format. Native packages require the format-specific GTK4/WebKitGTK
dependencies declared in their metadata.

Run portable packaging validation tests with `task linux:test`. Real package
builds and GUI checks run on the Linux architecture runners.
