# WiX Tasks

Repository-local WiX Toolset **5.0.2** automation for Windows MSI packaging.
All packaging operations are Taskfile tasks; no external PowerShell scripts or
global WiX installation are used. Tasks execute inline PowerShell commands on
Windows and reuse the [.NET module](../dotnet/README.md) to ensure SDK 10 is installed.

## Usage

From the repository root on Windows:

```sh
task wix:install
task wix:version
task wix:validate VERSION=0.1.0 ARCH=amd64
task windows:package VERSION=0.1.0 ARCH=amd64
```

The complete packaging flow remains `task windows:package` (or
`task package GOOS=windows`). It validates inputs, restores tooling, prepares
staging, builds the application, compiles the MSI, verifies it, and promotes
verified artifacts. Do not run packaging commands concurrently in one checkout.

Standalone installation and inspection are also supported:

```sh
task -t taskfiles/wix/Taskfile.yml --list
task -t taskfiles/wix/Taskfile.yml install
task -t taskfiles/wix/Taskfile.yml version
```

## Public Tasks

| Task | Behavior |
| --- | --- |
| `default` | List available WiX tasks |
| `install` | Ensure SDK 10 and restore the pinned local WiX tool |
| `restore` | Restore the repository tool manifest and check WiX 5.0.2 |
| `version` | Ensure installation and show the pinned local WiX version |
| `validate` | Validate Windows, the active SDK, and packaging inputs |
| `build` | Check the staged executable and compile its MSI |
| `verify` | Run ICE and MSI contract checks, then record verification proof |

`prepare` and `promote` remain internal stages of the Windows packaging flow.
Platform implementations and validation helpers are internal tasks.

## Inputs and Guarantees

- `VERSION` is required for packaging stages and must be `MAJOR.MINOR.PATCH`,
  without leading zeroes or suffixes, within MSI limits `255.255.65535`.
- `ARCH` defaults to `amd64`; supported values are `amd64` and `arm64`.
- Optional `TAG` must equal `vVERSION`; it includes the full tag in artifact filenames.
- `DEV` defaults to `false`; development packaging is rejected.
- The repository tool manifest owns the WiX version. SDK package overrides are
  provided by the .NET module.

Every command refreshes PATH before using newly installed tools. Staging stays
under `bin/.wix/ARCH`; output names remain `raven-VERSION-windows-ARCH.msi` and
the matching `.sha256` file. When `TAG` is supplied, names use
`raven-TAG-windows-ARCH.msi` instead; installer and executable metadata stay numeric.

Build and verification invalidate previous proof. Verification checks the PE
architecture and version, ICEs, installer scope and identity, cabinet and payload,
shortcut, launch conditions, and upgrade ordering. ICE91 is suppressed for the
intentional per-user paths; installer scope is checked independently. Promotion
recomputes both artifact hashes and rejects any change since verification.
