param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('validate', 'restore', 'prepare', 'build', 'verify', 'promote')]
    [string]$Action
)

. "$PSScriptRoot/funcs.ps1"
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
Set-Location -LiteralPath $root
try {
    Assert-PackagingHost
    if ($Action -eq 'restore') {
        & dotnet tool restore --tool-manifest "$root/.config/dotnet-tools.json"
        if ($LASTEXITCODE -ne 0) { throw 'Could not restore the pinned WiX tool.' }
        Assert-WixVersion
        exit 0
    }
    $version = $env:RAVEN_WIX_VERSION
    $arch = $env:RAVEN_WIX_ARCH
    Assert-PackagingInput $version $arch $env:RAVEN_WIX_DEV
    $stage = Join-Path $root "bin/.wix/$arch"
    $payload = Join-Path $stage 'payload/raven.exe'
    $name = "raven-$version-windows-$arch.msi"
    $msi = Join-Path $stage $name
    $proofPath = Join-Path $stage 'verified.json'
    switch ($Action) {
        validate { }
        prepare {
            if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
            New-Item -ItemType Directory -Path "$stage/payload" -Force | Out-Null
            # Do not leave a previous successful build available after a failed rebuild.
            foreach ($artifact in @("$root/bin/$name", "$root/bin/$name.sha256")) {
                if (Test-Path -LiteralPath $artifact) { Remove-Item -LiteralPath $artifact -Force }
            }
            $info = Get-Content -LiteralPath "$root/build/windows/info.json" -Raw | ConvertFrom-Json
            $info.fixed.file_version = $version
            $info.fixed.product_version = $version
            $info.info.'0409'.FileVersion = $version
            $info.info.'0409'.ProductVersion = $version
            $json = $info | ConvertTo-Json -Depth 10
            [IO.File]::WriteAllText("$stage/info.json", $json, (New-Object Text.UTF8Encoding($false)))
        }
        build {
            Assert-WixVersion
            if (Test-Path -LiteralPath $proofPath) { Remove-Item -LiteralPath $proofPath -Force }
            if (Test-Path -LiteralPath $msi) { Remove-Item -LiteralPath $msi -Force }
            Assert-Payload $payload $version $arch
            $platform = if ($arch -eq 'amd64') { 'x64' } else { 'arm64' }
            Invoke-Wix @('build', "$root/build/windows/wix/Package.wxs", '-arch', $platform,
                '-d', "Version=$version", '-d', "BinaryPath=$payload", '-d', "IconPath=$root/build/windows/icon.ico",
                '-intermediateFolder', "$stage/obj", '-wx', '-out', $msi)
        }
        verify {
            Assert-WixVersion
            if (Test-Path -LiteralPath $proofPath) { Remove-Item -LiteralPath $proofPath -Force }
            Assert-Payload $payload $version $arch
            Invoke-Wix @('msi', 'validate', $msi, '-wx')
            Assert-Msi $msi $version $arch
            @{
                Version = $version
                Arch = $arch
                MsiHash = Get-PackagingHash $msi
                PayloadHash = Get-PackagingHash $payload
            } | ConvertTo-Json | Set-Content -LiteralPath $proofPath -Encoding ascii
        }
        promote {
            Assert-VerificationProof $proofPath $msi $payload $version $arch
            $hash = (Get-PackagingHash $msi).ToLowerInvariant()
            "$hash  $name" | Set-Content -LiteralPath "$stage/$name.sha256" -Encoding ascii
            Move-Item -LiteralPath $msi -Destination "$root/bin/$name" -Force
            Move-Item -LiteralPath "$stage/$name.sha256" -Destination "$root/bin/$name.sha256" -Force
            Write-Host "Verified MSI: bin/$name"
        }
    }
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
}
