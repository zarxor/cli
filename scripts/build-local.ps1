[CmdletBinding()]
param(
    [string]$Version = 'dev',
    [string]$OutputDir = (Join-Path ([System.IO.Path]::GetTempPath()) 'jb-release'),
    [string]$GoExe
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $GoExe) {
    $goCommand = Get-Command go -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($goCommand) {
        $GoExe = $goCommand.Source
    } else {
        $GoExe = Join-Path $repoRoot '.tools/go1.26.5/go/bin/go.exe'
    }
}
if (-not (Test-Path -LiteralPath $GoExe -PathType Leaf)) {
    throw "Verified Go executable not found: $GoExe"
}

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
$stagingRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("jb-build-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stagingRoot | Out-Null

function Get-Sha256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Write-Checksum([string]$AssetPath, [string]$AssetName) {
    $contents = "$(Get-Sha256 $AssetPath)  $AssetName`n"
    [System.IO.File]::WriteAllText("$AssetPath.sha256", $contents, [System.Text.Encoding]::ASCII)
}

function New-DeterministicTarGz([string]$BinaryPath, [string]$AssetPath) {
    $item = Get-Item -LiteralPath $BinaryPath
    $item.LastWriteTimeUtc = [datetime]::new(1980, 1, 1, 0, 0, 0, [System.DateTimeKind]::Utc)
    $tarPath = [System.IO.Path]::GetTempFileName()
    try {
        & tar.exe --format ustar -cf $tarPath -C (Split-Path -Parent $BinaryPath) jb
        if ($LASTEXITCODE -ne 0) { throw "tar failed while creating $AssetPath" }
        $tarBytes = [System.IO.File]::ReadAllBytes($tarPath)
        if ($tarBytes.Length -lt 512) { throw "tar output is too short: $tarPath" }
        [System.Text.Encoding]::ASCII.GetBytes("0000755`0").CopyTo($tarBytes, 100)
        for ($index = 148; $index -lt 156; $index++) { $tarBytes[$index] = 0x20 }
        $checksum = 0
        for ($index = 0; $index -lt 512; $index++) { $checksum += $tarBytes[$index] }
        [System.Text.Encoding]::ASCII.GetBytes(([Convert]::ToString($checksum, 8).PadLeft(6, '0'))).CopyTo($tarBytes, 148)
        $tarBytes[154] = 0
        $tarBytes[155] = 0x20
        [System.IO.File]::WriteAllBytes($tarPath, $tarBytes)
        $input = [System.IO.File]::OpenRead($tarPath)
        $output = [System.IO.File]::Create($AssetPath)
        try {
            $gzip = [System.IO.Compression.GZipStream]::new($output, [System.IO.Compression.CompressionMode]::Compress, $false)
            try {
                $input.CopyTo($gzip)
            } finally {
                $gzip.Dispose()
            }
        } finally {
            $output.Dispose()
            $input.Dispose()
        }
    } finally {
        Remove-Item -LiteralPath $tarPath -Force -ErrorAction SilentlyContinue
    }
}

function New-DeterministicZip([string]$BinaryPath, [string]$AssetPath) {
    $fileStream = [System.IO.File]::Create($AssetPath)
    try {
        $zip = [System.IO.Compression.ZipArchive]::new($fileStream, [System.IO.Compression.ZipArchiveMode]::Create, $false)
        try {
            $entry = $zip.CreateEntry('jb.exe', [System.IO.Compression.CompressionLevel]::Optimal)
            $entry.LastWriteTime = [System.DateTimeOffset]::new(1980, 1, 1, 0, 0, 0, [System.TimeSpan]::Zero)
            $input = [System.IO.File]::OpenRead($BinaryPath)
            $output = $entry.Open()
            try {
                $input.CopyTo($output)
            } finally {
                $output.Dispose()
                $input.Dispose()
            }
        } finally {
            $zip.Dispose()
        }
    } finally {
        $fileStream.Dispose()
    }
}

try {
    foreach ($target in @(
        @{ OS = 'linux'; Arch = 'amd64'; Asset = 'jb_linux_amd64.tar.gz'; Executable = 'jb' },
        @{ OS = 'linux'; Arch = 'arm64'; Asset = 'jb_linux_arm64.tar.gz'; Executable = 'jb' },
        @{ OS = 'windows'; Arch = 'amd64'; Asset = 'jb_windows_amd64.zip'; Executable = 'jb.exe' },
        @{ OS = 'windows'; Arch = 'arm64'; Asset = 'jb_windows_arm64.zip'; Executable = 'jb.exe' }
    )) {
        $binaryPath = Join-Path $stagingRoot $target.Executable
        $assetPath = Join-Path $OutputDir $target.Asset
        Remove-Item -LiteralPath $binaryPath, $assetPath, "$assetPath.sha256" -Force -ErrorAction SilentlyContinue

        $oldGoos = $env:GOOS
        $oldGoarch = $env:GOARCH
        $oldCgo = $env:CGO_ENABLED
        try {
            $env:GOOS = $target.OS
            $env:GOARCH = $target.Arch
            $env:CGO_ENABLED = '0'
            Push-Location -LiteralPath $repoRoot
            try {
                & $GoExe build -trimpath -buildvcs=false "-ldflags=-s -w -X github.com/zarxor/scripts/internal/version.Version=$Version" -o $binaryPath ./cmd/jb
            } finally {
                Pop-Location
            }
            if ($LASTEXITCODE -ne 0) { throw "Go build failed for $($target.OS)/$($target.Arch)." }
        } finally {
            $env:GOOS = $oldGoos
            $env:GOARCH = $oldGoarch
            $env:CGO_ENABLED = $oldCgo
        }

        if ($target.OS -eq 'linux') {
            New-DeterministicTarGz $binaryPath $assetPath
        } else {
            New-DeterministicZip $binaryPath $assetPath
        }
        Write-Checksum $assetPath $target.Asset
        Write-Output "built $assetPath"
    }
} finally {
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force -ErrorAction SilentlyContinue
}
