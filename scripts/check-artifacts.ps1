[CmdletBinding()]
param(
    [string]$Version = 'dev',
    [string]$ArtifactDir = (Join-Path ([System.IO.Path]::GetTempPath()) 'jb-release')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

function Get-Sha256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Get-TarMembers([string]$AssetPath) {
    $members = @(& tar.exe -tzf $AssetPath)
    if ($LASTEXITCODE -ne 0) { throw "tar failed while reading $AssetPath" }
    return $members
}

function Get-TarListing([string]$AssetPath) {
    $listing = @(& tar.exe -tvzf $AssetPath)
    if ($LASTEXITCODE -ne 0) { throw "tar failed while inspecting $AssetPath" }
    return $listing
}

function Get-ZipMembers([string]$AssetPath) {
    $zip = [System.IO.Compression.ZipFile]::OpenRead($AssetPath)
    try {
        return @($zip.Entries | ForEach-Object FullName)
    } finally {
        $zip.Dispose()
    }
}

function Test-Checksum([string]$AssetPath, [string]$AssetName) {
    $checksumPath = "$AssetPath.sha256"
    if (-not (Test-Path -LiteralPath $checksumPath -PathType Leaf)) {
        throw "Missing checksum: $checksumPath"
    }
    $contents = [System.IO.File]::ReadAllText($checksumPath, [System.Text.Encoding]::ASCII).TrimEnd("`r", "`n")
    if ($contents -notmatch "^([0-9a-f]{64})  $([regex]::Escape($AssetName))$") {
        throw "Invalid checksum format: $checksumPath"
    }
    if ($Matches[1] -ne (Get-Sha256 $AssetPath)) {
        throw "Checksum does not match: $checksumPath"
    }
}

$hostOs = if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) { 'windows' } else { 'linux' }
$hostArch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { 'unsupported' }
}

foreach ($target in @(
    @{ OS = 'linux'; Arch = 'amd64'; Asset = 'jb_linux_amd64.tar.gz'; Executable = 'jb' },
    @{ OS = 'linux'; Arch = 'arm64'; Asset = 'jb_linux_arm64.tar.gz'; Executable = 'jb' },
    @{ OS = 'windows'; Arch = 'amd64'; Asset = 'jb_windows_amd64.zip'; Executable = 'jb.exe' },
    @{ OS = 'windows'; Arch = 'arm64'; Asset = 'jb_windows_arm64.zip'; Executable = 'jb.exe' }
)) {
    $assetPath = Join-Path $ArtifactDir $target.Asset
    if (-not (Test-Path -LiteralPath $assetPath -PathType Leaf)) {
        throw "Missing release asset: $assetPath"
    }

    $members = if ($target.OS -eq 'linux') { Get-TarMembers $assetPath } else { Get-ZipMembers $assetPath }
    if (@($members).Count -ne 1 -or @($members)[0] -ne $target.Executable) {
        throw "Unexpected archive members in $($target.Asset): $($members -join ', ')"
    }
    if ($target.OS -eq 'linux') {
        $listing = Get-TarListing $assetPath
        if (@($listing).Count -ne 1 -or @($listing)[0] -notmatch '^-rwxr-xr-x\s+') {
            throw "Linux executable is not mode 0755 in $($target.Asset): $($listing -join ', ')"
        }
    }
    Test-Checksum $assetPath $target.Asset

    if ($target.OS -eq $hostOs -and $target.Arch -eq $hostArch) {
        $extractRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("jb-check-" + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $extractRoot | Out-Null
        try {
            if ($target.OS -eq 'linux') {
                & tar.exe -xzf $assetPath -C $extractRoot
                if ($LASTEXITCODE -ne 0) { throw "tar failed while extracting $assetPath" }
            } else {
                [System.IO.Compression.ZipFile]::ExtractToDirectory($assetPath, $extractRoot)
            }
            $output = & (Join-Path $extractRoot $target.Executable) version 2>&1 | Out-String
            $versionPattern = '^Johan Bostrom CLI ' + [regex]::Escape($Version) + '$'
            if ($LASTEXITCODE -ne 0 -or $output.Trim() -notmatch $versionPattern) {
                throw "Unexpected version output from $($target.Asset): $output"
            }
        } finally {
            Remove-Item -LiteralPath $extractRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    Write-Output "ok - $($target.Asset)"
}
