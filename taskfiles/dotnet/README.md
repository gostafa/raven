# .NET Tasks

Install the .NET 10 SDK and run common .NET commands through TaskOtter’s
[Nix](../../taskotter/nix/README.md) (macOS/Linux) and
[WinGet](../../taskotter/winget/README.md) (Windows) modules.

## Usage

From the repository root:

```sh
task dotnet:install
task dotnet:version
task dotnet:info
task dotnet:list:sdks
task dotnet:list:runtimes
task dotnet:tool:restore
```

Project commands use `DOTNET_DIR`, defaulting to the directory where you invoked
Task. Pass .NET arguments after `--`; quote paths containing spaces using your
platform’s shell syntax. Argument boundaries are preserved when invoking .NET.

```sh
task dotnet:restore DOTNET_DIR="/path/to/project"
task dotnet:build -- "My Project.csproj" --configuration Release
task dotnet:test -- --no-build
task dotnet:run -- --project "My Project.csproj" -- --help
task dotnet:publish -- --configuration Release --output "publish output"
task dotnet:clean
```

For standalone use:

```sh
task -t taskfiles/dotnet/Taskfile.yml --list
task -t taskfiles/dotnet/Taskfile.yml version
```

## Tasks

| Task | Command / behavior |
| --- | --- |
| `default` | List available tasks |
| `install` | Ensure SDK 10 is installed |
| `version` | `dotnet --version` |
| `info` | `dotnet --info` |
| `list:sdks` | `dotnet --list-sdks` |
| `list:runtimes` | `dotnet --list-runtimes` |
| `restore` | `dotnet restore` |
| `build` | `dotnet build` |
| `test` | `dotnet test` |
| `run` | `dotnet run` |
| `publish` | `dotnet publish` |
| `clean` | `dotnet clean` |
| `tool:restore` | `dotnet tool restore` using the local manifest |

Every operational task ensures installation first. Installation checks
`dotnet --list-sdks` for SDK major 10; another SDK or a runtime alone does not
satisfy it. An existing SDK 10 is reused without installing a package manager.
PATH is refreshed before checks and execution, including immediately after
installation. Installation failures and .NET command failures stop the task.
Use Task’s `--exit-code` flag to preserve a command’s exit code at the Task CLI.

## Package Overrides

| Variable | Default |
| --- | --- |
| `DOTNET_NIX_INSTALLABLE` | `nixpkgs#dotnet-sdk_10` |
| `DOTNET_WINGET_INSTALLABLE` | `Microsoft.DotNet.SDK.10` |
| `DOTNET_DIR` | Caller’s working directory |

Overrides must provide the .NET 10 SDK. To pin a Nix package, for example:

```sh
task dotnet:install DOTNET_NIX_INSTALLABLE=github:NixOS/nixpkgs/REVISION#dotnet-sdk_10
```

SDK upgrade and uninstall are outside this module’s scope. SDK selection still
follows .NET’s normal rules, including any `global.json`; installing SDK 10 does
not force it to become the active SDK.

## WiX Packaging

WiX tasks ensure SDK 10 is installed automatically on Windows, then validate the
active SDK and restore the repository-pinned WiX tool. If a different SDK remains
active, inspect `dotnet --info`, PATH, and any `global.json` selection. MSI
packaging requires Windows even though the .NET tasks support all three platforms.
