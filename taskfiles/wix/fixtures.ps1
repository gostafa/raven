param([Parameter(Mandatory = $true)][ValidateSet('amd64', 'arm64')][string]$Arch)
. "$PSScriptRoot/funcs.ps1"
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
Set-Location -LiteralPath $root
$destination = Join-Path $root "bin/msi-tests/$Arch"
New-Item -ItemType Directory -Path $destination -Force | Out-Null
foreach ($fixture in @(@('0.0.1', 'old.msi'), @('0.0.2', 'new.msi'))) {
    & task windows:package "ARCH=$Arch" "VERSION=$($fixture[0])"
    if ($LASTEXITCODE -ne 0) { throw 'Could not build MSI test fixture.' }
    Copy-Item -LiteralPath "$root/bin/raven-$($fixture[0])-windows-$Arch.msi" -Destination "$destination/$($fixture[1])" -Force
}
# Recompile to obtain a different ProductCode at the same version.
foreach ($action in @('build', 'verify')) {
    & task "wix:$action" "ARCH=$Arch" VERSION=0.0.2
    if ($LASTEXITCODE -ne 0) { throw 'Could not build same-version replacement test fixture.' }
}
Copy-Item -LiteralPath "$root/bin/.wix/$Arch/raven-0.0.2-windows-$Arch.msi" -Destination "$destination/replacement.msi" -Force
