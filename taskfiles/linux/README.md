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
APT and Snapcraft from the `8.x/stable` channel. Snapcraft runs in destructive
mode: use a disposable Ubuntu 24.04 machine or GitHub runner for packaging Snap.
This mode installs build dependencies directly on the host.
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
Intermediate metadata, Snapcraft files, package candidates, extracted payloads,
and startup logs live under `bin/.linux/ARCH/`. Run packaging sequentially in a
checkout; CI uses a fresh checkout for each architecture. Individual
`linux:create:appimage`, `linux:create:deb`, `linux:create:rpm`,
`linux:create:snap`, and `linux:create:pacman` tasks take the same version and
architecture arguments. `linux:create:aur` remains an alias for pacman packaging;
it does not publish to AUR.

For `TAG=v1.2.3 ARCH=amd64`, promoted files are:

- `bin/raven-v1.2.3-linux-amd64.AppImage`
- `bin/raven-v1.2.3-linux-amd64.deb`
- `bin/raven-v1.2.3-linux-amd64.rpm`
- `bin/raven-v1.2.3-linux-amd64.snap`
- `bin/raven-v1.2.3-linux-amd64.pkg.tar.zst`

Each file has an adjacent `.sha256` sidecar. Verification checks package
metadata, architecture, extracted executable identity, desktop metadata, and
bundled WebKit subprocesses. Each format must show a Raven window under Xvfb
and remain alive before promotion. Snap is installed locally to exercise strict
confinement, then removed. Startup logs remain in the staging directory on
failure. No packages are promoted until every selected format passes.

When `GITHUB_OUTPUT` is set, release writes `artifact-name`, `package-paths`, and
`checksum-paths` only after all checks pass. The action uploads exactly those
packages and sidecars and exposes `artifact-id` and `artifact-url` as well.
Manual Release workflow runs upload artifacts without publishing. Stable tag
pushes publish only after both Windows and both Linux builds succeed. The
publisher verifies the checksums again before publishing installers and sidecars.
No Snap Store or AUR credentials are required.

## Installing Packages

Download the package for your architecture and its `.sha256` sidecar, then run
`sha256sum --check FILENAME.sha256` before installation. Examples:

```sh
chmod +x raven-v1.2.3-linux-amd64.AppImage
./raven-v1.2.3-linux-amd64.AppImage
sudo apt install ./raven-v1.2.3-linux-amd64.deb
sudo dnf install ./raven-v1.2.3-linux-amd64.rpm
sudo snap install --dangerous ./raven-v1.2.3-linux-amd64.snap
sudo snap connect raven:webkit-sandbox :browser-support
sudo pacman -U ./raven-v1.2.3-linux-amd64.pkg.tar.zst
```

Choose one format. `--dangerous` permits a locally downloaded Snap without a
Snap Store assertion; confinement remains strict. Snap requires snapd and its
GNOME runtime content snaps. AppImage requires a system compatible with the
Ubuntu 24.04 build baseline; on systems without FUSE, use
`APPIMAGE_EXTRACT_AND_RUN=1 ./FILENAME.AppImage`. Native packages require the
format-specific GTK4/WebKitGTK dependencies declared in their metadata.

The Snap's manual browser-support connection enables WebKit's own sandbox inside
Snap confinement. The startup check makes the same connection before launching.

Run portable packaging validation tests with `task linux:test`. Real package
builds and GUI checks run on the Linux architecture runners.
