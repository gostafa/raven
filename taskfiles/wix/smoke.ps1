param(
    [Parameter(Mandatory = $true)][ValidateSet('amd64', 'arm64')][string]$Arch,
    [Parameter(Mandatory = $true)][string]$Version
)
. "$PSScriptRoot/funcs.ps1"
Assert-PackagingInput $Version $Arch 'false'
$native = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
$expected = if ($Arch -eq 'amd64') { 'X64' } else { 'Arm64' }
if ($native -ne $expected) { throw "Native $Arch installation tests require a $expected host." }
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$logs = Join-Path $root "bin/msi-logs/$Arch"
New-Item -ItemType Directory -Path $logs -Force | Out-Null
$fixtures = Join-Path $root "bin/msi-tests/$Arch"
$candidate = Join-Path $root "dist/raven-$Version-windows-$Arch.msi"
$application = Join-Path $env:LOCALAPPDATA 'Programs/Raven/raven.exe'
$shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'Raven/Raven.lnk'
$sentinel = Join-Path $env:APPDATA "raven/msi-test-$([guid]::NewGuid()).txt"
$registryFixture = "Software\Raven\InstallerTests\$([guid]::NewGuid())"
$transform = [IO.Path]::GetFullPath((Join-Path $logs 'webview-test.mst'))
$testCopy = Join-Path $logs 'webview-test.msi'
$counter = 0

function Invoke-MsiTest {
    param([string]$Name, [string]$Operation, [string]$Package, [string[]]$Extra = @(), [int[]]$ExpectedCodes = @(0, 3010))
    $script:counter++
    # PowerShell accepts mixed separators; msiexec needs native absolute paths.
    $Package = [IO.Path]::GetFullPath($Package)
    $log = [IO.Path]::GetFullPath((Join-Path $logs "$script:counter-$Name.log"))
    $arguments = @($Operation, "`"$Package`"", '/qn', '/norestart', '/L*v', "`"$log`"") + $Extra
    $process = Start-Process -FilePath "$env:SystemRoot/System32/msiexec.exe" -ArgumentList $arguments -Wait -PassThru
    if ($process.ExitCode -notin $ExpectedCodes) { throw "$Name failed with exit code $($process.ExitCode); see $log." }
    $message = switch -Regex ($Name) {
        '^(missing|invalid)-webview' { 'Install the Microsoft Edge WebView2 Evergreen Runtime'; break }
        '^downgrade$' { 'A newer version of Raven is already installed.'; break }
        '^same-version-replacement$' { 'This version of Raven is already installed.'; break }
        default { '' }
    }
    if ($message -and !(Select-String -LiteralPath $log -SimpleMatch $message -Quiet)) {
        throw "$Name failed without the expected installer message: $message"
    }
}

function Assert-Installed {
    param([string]$InstalledVersion, [string]$InstalledArch)
    Assert-Payload $application $InstalledVersion $InstalledArch
    if (!(Test-Path -LiteralPath $shortcut)) { throw 'Start menu shortcut is missing.' }
    $registrations = @(Get-ChildItem 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall' |
        Get-ItemProperty | Where-Object { $_.PSObject.Properties['DisplayName'] -and $_.DisplayName -eq 'Raven' })
    if ($registrations.Count -ne 1 -or $registrations[0].DisplayVersion -ne $InstalledVersion) {
        throw 'Expected exactly one per-user Raven installation at the expected version.'
    }
}

function Assert-DataPreserved {
    if (!(Test-Path -LiteralPath $sentinel) -or (Get-Content -LiteralPath $sentinel -Raw).Trim() -ne 'preserve Raven data') {
        throw 'Installation changed user data.'
    }
}

function New-WebViewTestTransform {
    # Redirect only the two registry searches in a test transform. Never modify the host's runtime registration.
    Copy-Item -LiteralPath $candidate -Destination $testCopy
    $installer = New-Object -ComObject WindowsInstaller.Installer
    $original = $null
    $modified = $null
    try {
        $original = $installer.OpenDatabase($candidate, 0)
        $modified = $installer.OpenDatabase($testCopy, 1)
        foreach ($entry in @(@('WebViewUser', 'User'), @('WebViewMachine', 'Machine'))) {
            $query = 'UPDATE `RegLocator` SET `Root` = 1, `Key` = ''{0}\{1}'' WHERE `Signature_` = ''{2}''' -f $registryFixture, $entry[1], $entry[0]
            $view = $modified.OpenView($query)
            try { $view.Execute() } finally {
                $view.Close()
                [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($view)
            }
        }
        $modified.Commit()
        if (!$modified.GenerateTransform($original, $transform)) { throw 'Could not generate WebView2 test transform.' }
        $modified.CreateTransformSummaryInfo($original, 0, 0)
    } finally {
        if ($modified) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($modified) }
        if ($original) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($original) }
        [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($installer)
    }
}

if (Test-Path -LiteralPath $application) { throw 'Run MSI tests only on a disposable machine without Raven installed.' }
Assert-Msi $candidate $Version $Arch
$checksum = (Get-Content -LiteralPath "$candidate.sha256" -Raw).Trim()
$hash = (Get-PackagingHash $candidate).ToLowerInvariant()
if ($checksum -cne "$hash  $([IO.Path]::GetFileName($candidate))") { throw 'Candidate checksum mismatch.' }
New-Item -ItemType Directory -Path (Split-Path $sentinel) -Force | Out-Null
Set-Content -LiteralPath $sentinel -Value 'preserve Raven data' -Encoding ascii
$testFailed = $false
try {
    New-WebViewTestTransform
    $transforms = @("TRANSFORMS=`"$transform`"")
    Invoke-MsiTest missing-webview /i $candidate $transforms @(1603)
    foreach ($value in @('', '0.0.0.0')) {
        foreach ($location in @('User', 'Machine')) {
            $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey("$registryFixture\$location")
            try { $key.SetValue('pv', $value, [Microsoft.Win32.RegistryValueKind]::String) } finally { $key.Dispose() }
        }
        Invoke-MsiTest invalid-webview /i $candidate $transforms @(1603)
    }
    foreach ($location in @('User', 'Machine')) {
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey("$registryFixture\$location")
        try { $key.SetValue('pv', '1.0.0.0', [Microsoft.Win32.RegistryValueKind]::String) } finally { $key.Dispose() }
        Invoke-MsiTest "valid-webview-$location" /i $candidate $transforms
        Assert-Installed $Version $Arch
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($registryFixture, $false)
        Invoke-MsiTest missing-webview-repair /fa $candidate @() @(1603)
        Invoke-MsiTest uninstall-without-webview /x $candidate
        Assert-DataPreserved
    }
    # Exercise the unmodified release package against the runner's real prerequisite.
    Invoke-MsiTest release-install /i $candidate
    Assert-Installed $Version $Arch
    Remove-Item -LiteralPath $application -Force
    Remove-Item -LiteralPath $shortcut -Force
    Invoke-MsiTest release-repair /fa $candidate
    Assert-Installed $Version $Arch
    Invoke-MsiTest release-uninstall /x $candidate
    Assert-DataPreserved

    Invoke-MsiTest old-install /i "$fixtures/old.msi"
    Assert-Installed '0.0.1' $Arch
    Invoke-MsiTest upgrade /i "$fixtures/new.msi"
    Assert-Installed '0.0.2' $Arch
    Assert-DataPreserved
    Invoke-MsiTest downgrade /i "$fixtures/old.msi" @() @(1638, 1603)
    Invoke-MsiTest same-version-replacement /i "$fixtures/replacement.msi" @() @(1638, 1603)
    Assert-Installed '0.0.2' $Arch
    Invoke-MsiTest upgrade-uninstall /x "$fixtures/new.msi"
    if ($Arch -eq 'arm64') {
        Invoke-MsiTest x64-install /i "$root/bin/msi-tests/amd64/old.msi"
        Assert-Installed '0.0.1' 'amd64'
        Invoke-MsiTest native-arm64-upgrade /i "$fixtures/new.msi"
        Assert-Installed '0.0.2' 'arm64'
        Invoke-MsiTest native-arm64-uninstall /x "$fixtures/new.msi"
    }
    if ((Test-Path -LiteralPath $application) -or (Test-Path -LiteralPath $shortcut)) {
        throw 'Uninstall left application files or shortcuts behind.'
    }
    Assert-DataPreserved
} catch {
    $testFailed = $true
    throw
} finally {
    $cleanupFailure = $null
    foreach ($package in @($candidate, "$fixtures/new.msi", "$fixtures/old.msi", "$root/bin/msi-tests/amd64/old.msi")) {
        try {
            if (Test-Path -LiteralPath $package) {
                Invoke-MsiTest cleanup /x $package @() @(0, 3010, 1605)
            }
        } catch {
            if ($null -eq $cleanupFailure) { $cleanupFailure = $_ }
            Write-Warning "MSI cleanup failed: $_"
        }
    }
    try { [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($registryFixture, $false) } catch {
        if ($null -eq $cleanupFailure) { $cleanupFailure = $_ }
        Write-Warning "Registry cleanup failed: $_"
    }
    try { Remove-Item -LiteralPath $sentinel -Force } catch {
        if ($null -eq $cleanupFailure) { $cleanupFailure = $_ }
        Write-Warning "User data fixture cleanup failed: $_"
    }
    # Cleanup must fail a successful test, but must not replace the original test error.
    if (!$testFailed -and $null -ne $cleanupFailure) { throw $cleanupFailure }
}
Write-Host "Native $Arch MSI installation tests passed."
