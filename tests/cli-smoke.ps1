Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
    Write-Output 'ok - PowerShell installer smoke test is Windows-only'
    exit 0
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$installer = Join-Path $repoRoot 'install.ps1'
if (-not (Test-Path -LiteralPath $installer)) {
    throw 'not ok - Windows bootstrap installer exists'
}
$windowsPowerShell = Get-Command powershell.exe -ErrorAction SilentlyContinue
if (-not $windowsPowerShell) {
    throw 'not ok - Windows PowerShell 5.1 is available for compatibility smoke coverage'
}

$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("jb-smoke-" + [guid]::NewGuid().ToString('N'))
$releaseDir = Join-Path $testRoot 'release'
$payloadDir = Join-Path $testRoot 'payload'
New-Item -ItemType Directory -Path $releaseDir, $payloadDir | Out-Null

$architecture = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { throw "Unsupported test architecture: $_" }
}
$assetName = "jb_windows_$architecture.zip"
$assetPath = Join-Path $releaseDir $assetName
Copy-Item -LiteralPath $env:ComSpec -Destination (Join-Path $payloadDir 'jb.exe')
Compress-Archive -LiteralPath (Join-Path $payloadDir 'jb.exe') -DestinationPath $assetPath
$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $assetPath).Hash.ToLowerInvariant()
Set-Content -LiteralPath "$assetPath.sha256" -Value "$hash  $assetName" -Encoding ascii

$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
$listener.Stop()
$serverOutput = Join-Path $testRoot 'server.out'
$serverError = Join-Path $testRoot 'server.err'
$server = Start-Process -FilePath python -ArgumentList @('-m', 'http.server', "$port", '--bind', '127.0.0.1', '--directory', "`"$releaseDir`"") -PassThru -WindowStyle Hidden -RedirectStandardOutput $serverOutput -RedirectStandardError $serverError

try {
    $releaseBaseUrl = "http://127.0.0.1:$port"
    for ($attempt = 0; $attempt -lt 100; $attempt++) {
        try {
            Invoke-WebRequest -Uri "$releaseBaseUrl/$assetName.sha256" -UseBasicParsing | Out-Null
            break
        } catch {
            Start-Sleep -Milliseconds 50
        }
    }

    $env:JB_RELEASE_BASE_URL = $releaseBaseUrl
    $installDir = Join-Path $testRoot 'user-bin'
    $output = & $windowsPowerShell.Source -NoProfile -File $installer -InstallDir $installDir 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) { throw "not ok - Windows installer failed: $output" }
    if (-not (Test-Path -LiteralPath (Join-Path $installDir 'jb.exe'))) { throw 'not ok - Windows installer writes the selected binary path' }
    if ($output -notlike "*$installDir\jb.exe*") { throw 'not ok - Windows installer reports the installed binary path' }
    Write-Output 'ok - Windows PowerShell 5.1 installer selects and installs the matching release asset'

    Set-Content -LiteralPath "$assetPath.sha256" -Value (("0" * 64) + "  $assetName") -Encoding ascii
    $badInstallDir = Join-Path $testRoot 'bad-bin'
    $savedErrorPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $badOutput = & $windowsPowerShell.Source -NoProfile -File $installer -InstallDir $badInstallDir 2>&1 | Out-String
        $badExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $savedErrorPreference
    }
    if ($badExitCode -eq 0) { throw 'not ok - Windows installer rejects a checksum mismatch' }
    if (Test-Path -LiteralPath (Join-Path $badInstallDir 'jb.exe')) { throw 'not ok - Windows installer installed an unverified binary' }
    if ($badOutput -notmatch '(?i)checksum') { throw 'not ok - Windows checksum failure explains the rejection' }
    Write-Output 'ok - Windows installer rejects checksum mismatches'
} finally {
    Remove-Item Env:JB_RELEASE_BASE_URL -ErrorAction SilentlyContinue
    if (-not $server.HasExited) { Stop-Process -Id $server.Id -Force }
    $server.Dispose()
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
