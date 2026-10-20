Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-PackagingHash {
    param([string]$Path)
    # Avoid depending on Get-FileHash module discovery in nested PowerShell hosts.
    $algorithm = [Security.Cryptography.SHA256]::Create()
    $stream = $null
    try {
        $stream = [IO.File]::OpenRead($Path)
        return [BitConverter]::ToString($algorithm.ComputeHash($stream)).Replace('-', '')
    } finally {
        if ($null -ne $stream) { $stream.Dispose() }
        $algorithm.Dispose()
    }
}

function Assert-PackagingInput {
    param([string]$Version, [string]$Arch, [string]$Dev)
    if ($Arch -cnotin @('amd64', 'arm64')) { throw 'ARCH must be amd64 or arm64.' }
    if ($Dev -and $Dev -cne 'false') { throw 'MSI packaging requires a production build (DEV=false).' }
    if ($Version -cnotmatch '\A(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,4})\z') {
        throw 'VERSION is required and must be MAJOR.MINOR.PATCH without leading zeroes or suffixes.'
    }
    $parts = $Version.Split('.')
    if ([int]$parts[0] -gt 255 -or [int]$parts[1] -gt 255 -or [int]$parts[2] -gt 65535) {
        throw 'MSI VERSION limits are 255.255.65535.'
    }
}

function Assert-PackagingHost {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'MSI packaging requires Windows.' }
    if (!(Get-Command dotnet -ErrorAction SilentlyContinue)) { throw 'Install the .NET 10 SDK before packaging.' }
    $sdk = & dotnet --version
    if ($LASTEXITCODE -ne 0 -or $sdk -notmatch '^10\.') { throw 'The active dotnet SDK must be .NET 10.' }
}

function Invoke-Wix {
    param([string[]]$Arguments)
    & dotnet tool run wix --allow-roll-forward -- @Arguments
    if ($LASTEXITCODE -ne 0) { throw "WiX failed with exit code $LASTEXITCODE." }
}

function Assert-WixVersion {
    $version = & dotnet tool run wix --allow-roll-forward -- --version
    if ($LASTEXITCODE -ne 0 -or $version -notmatch '^5\.0\.2(?:\+|$)') {
        throw 'Restore the repository-pinned WiX Toolset 5.0.2 with task wix:restore.'
    }
}

function Assert-Payload {
    param([string]$Path, [string]$Version, [string]$Arch)
    $stream = [IO.File]::OpenRead($Path)
    $reader = New-Object IO.BinaryReader($stream)
    try {
        if ($reader.ReadUInt16() -ne 0x5a4d) { throw 'Payload is not a Windows executable.' }
        $stream.Position = 0x3c
        $offset = $reader.ReadInt32()
        if ($offset -lt 0x40 -or $offset -gt ($stream.Length - 6)) { throw 'Invalid PE header offset.' }
        $stream.Position = $offset
        if ($reader.ReadUInt32() -ne 0x4550) { throw 'Payload has no PE signature.' }
        $expected = if ($Arch -eq 'amd64') { 0x8664 } else { 0xaa64 }
        if ($reader.ReadUInt16() -ne $expected) { throw "Payload architecture does not match $Arch." }
    } finally {
        $reader.Dispose()
        $stream.Dispose()
    }
    $info = [Diagnostics.FileVersionInfo]::GetVersionInfo($Path)
    foreach ($actual in @($info.FileVersion, $info.ProductVersion)) {
        if ($actual -notin @($Version, "$Version.0")) { throw "Payload version '$actual' does not match $Version." }
    }
}

function Read-MsiRows {
    param([object]$Database, [string]$Query, [int]$Columns)
    $view = $Database.OpenView($Query)
    try {
        # COM methods can emit null return values into PowerShell's output stream.
        # Only records belong in the rows returned to callers.
        [void]$view.Execute()
        while ($null -ne ($record = $view.Fetch())) {
            try {
                $row = @()
                for ($column = 1; $column -le $Columns; $column++) { $row += $record.StringData($column) }
                # Keep each row as one array, including single-column rows.
                ,$row
            } finally { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($record) }
        }
    } finally {
        [void]$view.Close()
        [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($view)
    }
}

function Assert-Msi {
    param([string]$Path, [string]$Version, [string]$Arch)
    $installer = New-Object -ComObject WindowsInstaller.Installer
    $database = $null
    $summary = $null
    try {
        $database = $installer.OpenDatabase($Path, 0)
        $summary = $database.SummaryInformation(0)
        $expected = if ($Arch -eq 'amd64') { 'x64;1033' } else { 'Arm64;1033' }
        if ($summary.Property(7) -cne $expected) { throw 'MSI platform does not match ARCH.' }
        if (($summary.Property(15) -band 8) -eq 0) { throw 'MSI must not require elevation.' }
        $properties = @{}
        foreach ($row in (Read-MsiRows $database 'SELECT `Property`, `Value` FROM `Property`' 2)) {
            $properties[$row[0]] = $row[1]
        }
        if ($properties.ProductVersion -cne $Version) { throw 'MSI version does not match VERSION.' }
        if ($properties.UpgradeCode -ne '{74CE072D-0DA0-40A3-8700-7AE3A65B9769}') { throw 'Unexpected UpgradeCode.' }
        if ($properties['ALLUSERS']) { throw 'MSI must install per user.' }
        if ($properties.MSIRESTARTMANAGERCONTROL -ne 'DisableShutdown') { throw 'MSI must not force application shutdown.' }
        $files = @(Read-MsiRows $database 'SELECT `FileName`, `Version` FROM `File`' 2)
        if ($files.Count -ne 1 -or $files[0][0] -notmatch '(^|\|)raven\.exe$' -or
            $files[0][1] -notin @($Version, "$Version.0")) { throw 'Unexpected MSI payload or payload version.' }
        $media = @(Read-MsiRows $database 'SELECT `Cabinet` FROM `Media`' 1)
        if ($media.Count -ne 1 -or !$media[0][0].StartsWith('#')) { throw 'MSI must contain one embedded cabinet.' }
        $tables = @(Read-MsiRows $database 'SELECT `Name` FROM `_Tables`' 1)
        if (@($tables | Where-Object { $_[0] -eq 'CustomAction' }).Count -gt 0) {
            $actions = @(Read-MsiRows $database 'SELECT `Action` FROM `CustomAction`' 1)
            if ($actions.Count -gt 0) { throw 'Custom installer actions are not permitted.' }
        }
        $shortcuts = @(Read-MsiRows $database 'SELECT `Target` FROM `Shortcut`' 1)
        if ($shortcuts.Count -ne 1 -or $shortcuts[0][0] -ne '[#RavenExe]') { throw 'Unexpected Start menu shortcut.' }
        $conditions = @(Read-MsiRows $database 'SELECT `Condition` FROM `LaunchCondition`' 1)
        # Three explicit Launch elements plus the downgrade guard generated by MajorUpgrade.
        if ($conditions.Count -ne 4) { throw "Expected 4 installer launch conditions; found $($conditions.Count)." }
        $sequence = @(Read-MsiRows $database 'SELECT `Action`, `Sequence` FROM `InstallExecuteSequence`' 2)
        $order = @{}
        foreach ($row in $sequence) { $order[$row[0]] = [int]$row[1] }
        if ($order.RemoveExistingProducts -le $order.InstallInitialize -or
            $order.RemoveExistingProducts -ge $order.InstallFiles) { throw 'Upgrade removal must be inside the transaction before file installation.' }
    } finally {
        if ($summary) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($summary) }
        if ($database) { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($database) }
        [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($installer)
    }
}

function Assert-VerificationProof {
    param([string]$ProofPath, [string]$MsiPath, [string]$PayloadPath, [string]$Version, [string]$Arch)
    $proof = Get-Content -LiteralPath $ProofPath -Raw | ConvertFrom-Json
    if ($proof.Version -cne $Version -or $proof.Arch -cne $Arch -or
        $proof.MsiHash -cne (Get-PackagingHash $MsiPath) -or
        $proof.PayloadHash -cne (Get-PackagingHash $PayloadPath)) {
        throw 'Package or payload changed after verification; run task wix:verify again.'
    }
}
