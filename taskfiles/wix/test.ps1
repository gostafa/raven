. "$PSScriptRoot/funcs.ps1"

function Assert-Throws {
    param([scriptblock]$Body, [string]$Message)
    $failed = $false
    try { & $Body } catch { $failed = $true }
    if (!$failed) { throw "Expected failure: $Message" }
}

foreach ($arch in @('amd64', 'arm64')) {
    foreach ($version in @('0.0.0', '0.1.0', '255.255.65535')) {
        Assert-PackagingInput $version $arch 'false'
    }
}
foreach ($version in @('', 'v1.2.3', '01.2.3', '1.02.3', '1.2.03', '1.2', '1.2.3.0',
        '1.2.3-beta', '1.2.3+build', "1.2.3`n", '256.0.0', '0.256.0', '0.0.65536', '999999999999.0.0')) {
    Assert-Throws { Assert-PackagingInput $version 'amd64' 'false' } "invalid version $version"
}
foreach ($arch in @('', '386', 'x64', 'ARM64', '../arm64')) {
    Assert-Throws { Assert-PackagingInput '1.2.3' $arch 'false' } "invalid architecture $arch"
}
Assert-Throws { Assert-PackagingInput '1.2.3' 'amd64' 'true' } 'development build'

$temporary = Join-Path ([IO.Path]::GetTempPath()) "raven-wix-tests-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    $payload = Join-Path $temporary 'raven.exe'
    $msi = Join-Path $temporary 'raven.msi'
    $proof = Join-Path $temporary 'verified.json'
    [IO.File]::WriteAllBytes($payload, [byte[]](0, 1, 2, 3))
    [IO.File]::WriteAllBytes($msi, [byte[]](4, 5, 6, 7))
    if ((Get-PackagingHash $payload) -cne '054EDEC1D0211F624FED0CBCA9D4F9400B0E491C43742AF2C5B0ABEBF0C990D8') {
        throw 'Unexpected SHA-256 digest.'
    }
    Assert-Throws { Get-PackagingHash "$temporary/missing.exe" } 'missing hash input'
    Assert-Throws { Assert-Payload $payload '1.2.3' 'amd64' } 'invalid executable'
    Assert-Throws { Assert-Payload "$temporary/missing.exe" '1.2.3' 'amd64' } 'missing payload'
    Assert-Throws { Assert-VerificationProof $proof $msi $payload '1.2.3' 'amd64' } 'unverified package'
    @{
        Version = '1.2.3'
        Arch = 'amd64'
        MsiHash = Get-PackagingHash $msi
        PayloadHash = Get-PackagingHash $payload
    } | ConvertTo-Json | Set-Content -LiteralPath $proof -Encoding ascii
    Assert-VerificationProof $proof $msi $payload '1.2.3' 'amd64'
    Assert-Throws { Assert-VerificationProof $proof $msi $payload '1.2.4' 'amd64' } 'changed version'
    Assert-Throws { Assert-VerificationProof $proof $msi $payload '1.2.3' 'arm64' } 'changed architecture'
    [IO.File]::WriteAllBytes($msi, [byte[]](8, 9))
    Assert-Throws { Assert-VerificationProof $proof $msi $payload '1.2.3' 'amd64' } 'changed MSI'

    # Native command failures must propagate instead of allowing artifact promotion.
    function dotnet { $global:LASTEXITCODE = 42 }
    Assert-Throws { Invoke-Wix @('build', 'missing.wxs') } 'failed WiX compilation'
    Assert-Throws { Assert-WixVersion } 'failed WiX version check'
    Remove-Item Function:dotnet
    $originalPath = $env:PATH
    try {
        $env:PATH = $temporary
        Assert-Throws { Assert-PackagingHost } 'missing .NET SDK'
    } finally { $env:PATH = $originalPath }
} finally { Remove-Item -LiteralPath $temporary -Recurse -Force }
Write-Host 'WiX packaging validation tests passed.'
