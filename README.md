# Raven

A desktop app for remembering what you learn. Save text, images, audio, and video in cards, organize them into folders, and review each item with spaced repetition.

Built with Go, Wails 3, and Svelte for macOS, Windows, and Linux.

## Features

- Mixed-media cards, folders, tags, and search.
- Scheduled reviews with **Again**, **Hard**, **Good**, and **Easy** ratings.
- A floating widget with due counts and quick reviews.
- Local storage, desktop reminders, and optional start at login.

Closing the main window keeps Raven running. Use **Quit Raven** in the widget or tray menu to exit.

## Development

Install Go **1.27.2**, Bun **1.4.2**, Node.js **26.12.0+**, Task, and the native dependencies for your platform:

| Platform | Requirements |
| --- | --- |
| macOS | macOS 13+ and Xcode Command Line Tools |
| Windows | Windows 10/11 and Microsoft Edge WebView2 Runtime |
| Linux | GTK4 and WebKitGTK 6.0 development libraries; Ubuntu 26.04 is the baseline |

From the repository root:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.29
bun install
bun install --cwd frontend
bun run start
```

Ensure your Go bin directory is on `PATH`. Development generates frontend bindings automatically. Wails 3 is a beta dependency.

## Build

Run on the target operating system:

```sh
bun run build
bun run package
```

Output is written to `bin/`.

### macOS Release DMGs

The Release workflow builds separate disk images for macOS 13 or later:

- Intel: `raven-vVERSION-macos-amd64.dmg`
- Apple Silicon: `raven-vVERSION-macos-arm64.dmg`

Download the image for your Mac, open it, and drag `raven.app` onto the Applications
shortcut. Quit any running copy of Raven before replacing it. The app uses ad-hoc
signing and is not notarized; macOS may require approval in **System Settings →
Privacy & Security** before its first launch.

DMGs are built and verified only by the release workflow. Local `bun run package`
continues to produce an `.app` bundle. Release artifacts include SHA-256 sidecars
for verification before publication; the GitHub release publishes the DMGs only.

### Linux Packaging

Linux releases support AMD64 and ARM64 with Debian (`.deb`), RPM,
and pacman (`.pkg.tar.zst`) packages. Build natively on Ubuntu 26.04:

```sh
task linux:install:deps ARCH=amd64
task linux:release TAG=v1.2.3 ARCH=amd64
```

Use `ARCH=arm64` on ARM64. All formats share one production build and must pass
package and GUI startup checks before promotion. See the
[Linux task documentation](taskfiles/linux/README.md) for local packaging,
dependencies, verification, filenames, and installation commands.

### Windows MSI Packaging

Package on Windows; the **.NET 10 SDK** is installed automatically through
TaskOtter’s WinGet module when missing. To install it ahead of time, run
`task dotnet:install`. See the [.NET tasks](taskfiles/dotnet/README.md) for
SDK information and common project commands. WiX Toolset **5.0.2** is restored
from the repository-local tool manifest; no global WiX installation is used.
From the repository root, build each architecture separately:

```sh
task windows:package ARCH=amd64 VERSION=0.1.0
task windows:package ARCH=arm64 VERSION=0.1.0
```

`task package GOOS=windows ARCH=amd64 VERSION=0.1.0` uses the same flow.
`VERSION` is required, must be `MAJOR.MINOR.PATCH`, and cannot exceed
`255.255.65535`. Development builds are rejected. Do not run packaging commands
concurrently in the same checkout.

Packaging validates inputs, restores pinned tooling, builds a versioned executable
in an architecture-specific staging directory, compiles the MSI, runs ICE and
package checks, and promotes only the verified MSI and its SHA-256 checksum to
`bin/`. Outputs are `raven-VERSION-windows-amd64.msi` and
`raven-VERSION-windows-arm64.msi`, each with an accompanying `.sha256` file.
The executable is an internal payload; Windows installers are distributed only as MSI files.

The MSI installs for the current user under `%LocalAppData%\Programs\Raven`
without administrator rights and adds a Start menu shortcut. Windows 10 or later
and the [Microsoft Edge WebView2 Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
are required. Install the runtime separately before installing or repairing Raven;
the MSI does not download or install prerequisites. Uninstall remains available
without WebView2. Quit Raven before installing, upgrading, repairing, or uninstalling.

For example, in PowerShell, these commands install, repair, and uninstall silently
with verbose logs (replace the filename with your version and architecture):

```powershell
msiexec /i "raven-0.1.0-windows-amd64.msi" /qn /norestart /L*v install.log
msiexec /fa "raven-0.1.0-windows-amd64.msi" /qn /norestart /L*v repair.log
msiexec /x "raven-0.1.0-windows-amd64.msi" /qn /norestart /L*v uninstall.log
```

Newer versions upgrade the existing per-user installation, including a switch
from x64 to ARM64 on ARM64 Windows. Downgrades and replacement packages with the
same version are blocked; use the original MSI for maintenance. Upgrade and
uninstall preserve `%AppData%\raven`. MSI packages are unsigned.

The release workflow validates Windows MSI packages, all three Linux package
formats, and macOS DMGs on AMD64 and ARM64 before publishing the installers.
SHA-256 sidecars are verified before publication and are not published as release assets.
Run the Release workflow manually with a version to execute its checks without
publishing; publication runs only for stable version tags. Existing releases are
not overwritten.

## Checks

Run from the repository root, in order:

```sh
task taskotter:ci:fix
task taskotter:ci
```

For frontend changes, also run:

```sh
bun run --cwd frontend check
bun run --cwd frontend test
bun run --cwd frontend build
```

### Frontend Lint Policy

`bun run lint` checks all hand-written JavaScript, TypeScript, and Svelte files,
including tooling, configuration, release helpers, and tests, with zero warnings.
Generated bindings, build output, dependencies, and `taskotter/` are excluded.

The production ESLint limits follow the comparable rules in `.golangci.yml`:

| Go policy | Frontend enforcement |
| --- | --- |
| `cyclop` / `gocyclo`: complexity 5 | `complexity`: 5 |
| `revive` argument limit: 3 | `max-params`: 3 |
| `funlen`: 20 lines, 10 statements | `max-lines-per-function`: 20, `max-statements`: 10 |
| `nestif`: early nesting checks | `max-depth`: 2 (an approximation) |
| `lll`: 120 columns | `max-len`: 120 in JS/TS source; Prettier formats components |
| `errcheck`, `errorlint`, exhaustive switches | Strict typed promise, error, assertion, and exhaustiveness rules |
| `gosec`, `forbidigo`, `unused` | Security rules, no console/debugger/dynamic evaluation, strict unused checks |

Strict and stylistic type-aware TypeScript rules apply to application TS/JS and
Svelte scripts. Browser and Node globals are scoped separately. Svelte-aware
rules preserve reactive props and template narrowing; `$bindable()` declarations
are exempt from the TypeScript redundant-default rule because they establish a
binding contract. Inline exceptions must remain narrowly justified, and unused
exceptions are errors. Tests retain correctness and security checks but omit
production function-size limits, matching Go's test exclusions.

Go-specific metrics such as package distance, type reusability, coverage, and
reachable vulnerability analysis have no direct ESLint equivalents. ESLint does
not replace those Go checks or the separate frontend tests, typecheck, and build.

## Your Data

Raven stores its database and managed media in the user configuration directory:

| Platform | Location |
| --- | --- |
| macOS | `~/Library/Application Support/raven/` |
| Windows | `%AppData%\raven\` |
| Linux | `$XDG_CONFIG_HOME/raven/` or `~/.config/raven/` |

Quit Raven before backing up **both `raven.db` and the entire `media/` directory**. Imported media is copied into Raven; moving the original files does not affect saved cards.
