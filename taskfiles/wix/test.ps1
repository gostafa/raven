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
    # Load the real smoke-test launcher without running installations on the test host.
    $tokens = $null
    $parseErrors = $null
    $smoke = [Management.Automation.Language.Parser]::ParseFile(
        (Join-Path $PSScriptRoot 'smoke.ps1'), [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -ne 0) { throw 'Smoke test script has PowerShell syntax errors.' }
    $launcher = $smoke.Find({ param($node)
        $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Invoke-MsiTest'
    }, $true)
    . ([scriptblock]::Create($launcher.Extent.Text))
    $logs = "$temporary/log directory"
    $counter = 0
    $script:msiExitCode = 0
    function Start-Process {
        param($FilePath, $ArgumentList, [switch]$Wait, [switch]$PassThru)
        $script:msiArguments = $ArgumentList
        return [pscustomobject]@{ ExitCode = $script:msiExitCode }
    }
    try {
        $package = "$temporary/package directory/raven.msi"
        Invoke-MsiTest launcher /i $package
        $expectedPackage = '"' + $package.Replace('/', '\') + '"'
        $expectedLog = '"' + "$logs/1-launcher.log".Replace('/', '\') + '"'
        if ($msiArguments[1] -cne $expectedPackage -or $msiArguments[5] -cne $expectedLog) {
            throw 'msiexec package and log paths must use native separators and preserve spaces.'
        }
        $script:msiExitCode = 1619
        Assert-Throws { Invoke-MsiTest launcher /i $package } 'unreadable MSI must fail'
        $script:msiExitCode = 1605
        Invoke-MsiTest cleanup /x $package @() @(0, 3010, 1605)
    } finally { Remove-Item Function:Start-Process }

    # Exercise the actual COM output stream: Execute/Close must not become rows.
    $installer = New-Object -ComObject WindowsInstaller.Installer
    $database = $null
    try {
        $database = $installer.OpenDatabase((Join-Path $temporary 'rows.msi'), 3)
        foreach ($query in @(
            'CREATE TABLE `Property` (`Property` CHAR(72) NOT NULL, `Value` CHAR(0) PRIMARY KEY `Property`)',
            "INSERT INTO ``Property`` (``Property``, ``Value``) VALUES ('ProductVersion', '1.2.3')"
        )) {
            $view = $database.OpenView($query)
            try { [void]$view.Execute() } finally {
                [void]$view.Close()
                [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($view)
            }
        }
        $rows = @(Read-MsiRows $database 'SELECT `Property`, `Value` FROM `Property`' 2)
        if ($rows.Count -ne 1 -or $null -eq $rows[0] -or
            $rows[0][0] -cne 'ProductVersion' -or $rows[0][1] -cne '1.2.3') {
            throw 'MSI queries must return only data rows, without COM return values.'
        }
        $rows = @(Read-MsiRows $database 'SELECT `Property` FROM `Property`' 1)
        if ($rows.Count -ne 1 -or $rows[0] -isnot [array] -or $rows[0][0] -cne 'ProductVersion') {
            throw 'Single-column MSI rows must remain arrays.'
        }
        $rows = @(Read-MsiRows $database "SELECT ``Property`` FROM ``Property`` WHERE ``Property`` = 'Missing'" 1)
        if ($rows.Count -ne 0) { throw 'Empty MSI queries must not emit COM return values.' }

        # Build a minimal real MSI for the smoke test's registry-search transform.
        foreach ($query in @(
            "INSERT INTO ``Property`` (``Property``, ``Value``) VALUES ('ProductCode', '{74CE072D-0DA0-40A3-8700-7AE3A65B9769}')",
            'CREATE TABLE `RegLocator` (`Signature_` CHAR(72) NOT NULL, `Root` SHORT NOT NULL, `Key` CHAR(255) NOT NULL, `Name` CHAR(255), `Type` SHORT PRIMARY KEY `Signature_`)',
            "INSERT INTO ``RegLocator`` (``Signature_``, ``Root``, ``Key``, ``Name``, ``Type``) VALUES ('WebViewUser', 1, 'Software\Original\User', 'pv', 2)",
            "INSERT INTO ``RegLocator`` (``Signature_``, ``Root``, ``Key``, ``Name``, ``Type``) VALUES ('WebViewMachine', 2, 'Software\Original\Machine', 'pv', 2)"
        )) {
            $view = $database.OpenView($query)
            try { [void]$view.Execute() } finally {
                [void]$view.Close()
                [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($view)
            }
        }
        $summary = $database.SummaryInformation(1)
        try {
            $summary.Property(7) = 'x64;1033'
            [void]$summary.Persist()
        } finally { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($summary) }
        [void]$database.Commit()
    } finally {
        if ($null -ne $database) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($database) }
        [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($installer)
    }

    $transformHelper = $smoke.Find({ param($node)
        $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'New-WebViewTestTransform'
    }, $true)
    . ([scriptblock]::Create($transformHelper.Extent.Text))
    $candidate = Join-Path $temporary 'rows.msi'
    $testCopy = Join-Path $temporary 'webview-test.msi'
    $transform = Join-Path $temporary 'webview-test.mst'
    $registryFixture = 'Software\Raven\InstallerTests\Transform'
    New-WebViewTestTransform
    $installer = New-Object -ComObject WindowsInstaller.Installer
    $database = $null
    try {
        $summary = $installer.SummaryInformation($transform, 0)
        try {
            if (!$summary.Property(9)) { throw 'Transform summary must contain product identity information.' }
        } finally { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($summary) }
        $database = $installer.OpenDatabase($candidate, 1)
        [void]$database.ApplyTransform($transform, 0)
        $rows = @(Read-MsiRows $database 'SELECT `Signature_`, `Root`, `Key` FROM `RegLocator`' 3)
        if ($rows.Count -ne 2) { throw 'Transform must preserve both WebView2 searches.' }
        foreach ($row in $rows) {
            $location = if ($row[0] -eq 'WebViewUser') { 'User' } else { 'Machine' }
            if ($row[1] -ne '1' -or $row[2] -cne "$registryFixture\$location") {
                throw 'Transform must redirect WebView2 searches to the test registry fixture.'
            }
        }
    } finally {
        if ($null -ne $database) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($database) }
        [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($installer)
    }
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
