param(
    [string]$InstallDir,
    [switch]$Machine
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Test-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Test-MachineWidePath([string]$Path) {
    $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd('\')
    $roots = @($env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:SystemRoot) | Where-Object { $_ }
    foreach ($root in $roots) {
        $fullRoot = [System.IO.Path]::GetFullPath($root).TrimEnd('\')
        if ($fullPath.Equals($fullRoot, [StringComparison]::OrdinalIgnoreCase) -or
            $fullPath.StartsWith($fullRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }
    return $false
}

function Quote-ProcessArgument([string]$Value) {
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Request-Elevation([string]$Destination) {
    if (-not $PSCommandPath) {
        throw 'Save install.ps1 to disk before requesting a machine-wide installation.'
    }
    $hostPath = (Get-Process -Id $PID).Path
    $arguments = @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-File', (Quote-ProcessArgument $PSCommandPath),
        '-InstallDir', (Quote-ProcessArgument $Destination),
        '-Machine'
    ) -join ' '
    $process = Start-Process -FilePath $hostPath -Verb RunAs -ArgumentList $arguments -Wait -PassThru
    exit $process.ExitCode
}

function Resolve-WindowsArchitecture {
    param(
        [AllowNull()]
        [string]$RuntimeArchitecture,
        [AllowNull()]
        [string]$Wow64Architecture,
        [AllowNull()]
        [string]$ProcessArchitecture
    )

    $rawArchitecture = if (-not [string]::IsNullOrWhiteSpace($RuntimeArchitecture)) {
        $RuntimeArchitecture
    } elseif (-not [string]::IsNullOrWhiteSpace($Wow64Architecture)) {
        $Wow64Architecture
    } else {
        $ProcessArchitecture
    }

    switch -Regex ([string]$rawArchitecture) {
        '^(?i:arm64)$' { return 'Arm64' }
        '^(?i:(?:amd64|x64))$' { return 'X64' }
        default { throw "Unsupported Windows architecture: $rawArchitecture" }
    }
}

function Get-WindowsRuntimeArchitecture {
    try {
        $runtimeInformation = [type]::GetType(
            'System.Runtime.InteropServices.RuntimeInformation, System.Runtime.InteropServices.RuntimeInformation',
            $false
        )
        if ($null -eq $runtimeInformation) {
            $runtimeInformation = [AppDomain]::CurrentDomain.GetAssemblies() |
                ForEach-Object { $_.GetType('System.Runtime.InteropServices.RuntimeInformation', $false) } |
                Where-Object { $null -ne $_ } |
                Select-Object -First 1
        }
        if ($null -eq $runtimeInformation) {
            return $null
        }

        $bindingFlags = [System.Reflection.BindingFlags]::Public -bor [System.Reflection.BindingFlags]::Static
        $property = $runtimeInformation.GetProperty('OSArchitecture', $bindingFlags)
        if ($null -eq $property) {
            return $null
        }

        return $property.GetValue($null).ToString()
    } catch {
        return $null
    }
}

function Get-ReleaseArchitecture {
    $architecture = Resolve-WindowsArchitecture `
        (Get-WindowsRuntimeArchitecture) `
        ([Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITEW6432')) `
        ([Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITECTURE'))

    switch ($architecture) {
        'X64' { return 'amd64' }
        'Arm64' { return 'arm64' }
        default { throw "Unsupported Windows architecture: $architecture" }
    }
}

try {
    if ($env:OS -ne 'Windows_NT') {
        throw 'install.ps1 supports Windows only.'
    }

    if (-not $InstallDir) {
        if ($env:JB_INSTALL_DIR) {
            $InstallDir = $env:JB_INSTALL_DIR
        } elseif ($Machine) {
            $InstallDir = Join-Path $env:ProgramFiles 'Johan Bostrom CLI\bin'
        } else {
            $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\Johan Bostrom CLI\bin'
        }
    }

    $machineWide = $Machine -or (Test-MachineWidePath $InstallDir)
    if ($machineWide -and -not (Test-Administrator)) {
        Request-Elevation $InstallDir
    }

    $releaseBaseUrl = if ($env:JB_RELEASE_BASE_URL) {
        $env:JB_RELEASE_BASE_URL.TrimEnd('/')
    } else {
        'https://github.com/zarxor/cli/releases/latest/download'
    }
    $architecture = Get-ReleaseArchitecture
    $asset = "jb_windows_$architecture.zip"
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("jb-install-" + [guid]::NewGuid().ToString('N'))
    $archive = Join-Path $tempDir $asset
    $checksumFile = "$archive.sha256"
    $extractDir = Join-Path $tempDir 'extracted'

    New-Item -ItemType Directory -Path $tempDir | Out-Null
    try {
        Write-Output "[jb installer] Downloading $asset"
        Invoke-WebRequest -Uri "$releaseBaseUrl/$asset" -OutFile $archive -UseBasicParsing
        Invoke-WebRequest -Uri "$releaseBaseUrl/$asset.sha256" -OutFile $checksumFile -UseBasicParsing

        $expectedChecksum = ((Get-Content -LiteralPath $checksumFile -Raw).Trim() -split '\s+')[0]
        if ($expectedChecksum -notmatch '^[0-9a-fA-F]{64}$') {
            throw "Invalid checksum file for $asset."
        }
        $actualChecksum = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash
        if (-not $actualChecksum.Equals($expectedChecksum, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Checksum verification failed for $asset."
        }
        Write-Output '[jb installer] Checksum verified.'

        Expand-Archive -LiteralPath $archive -DestinationPath $extractDir
        $source = Join-Path $extractDir 'jb.exe'
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            throw "$asset does not contain jb.exe."
        }
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        $destination = Join-Path $InstallDir 'jb.exe'
        Copy-Item -LiteralPath $source -Destination $destination -Force
    } finally {
        Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    Write-Output "[jb installer] Installed $destination"
    $pathEntries = $env:Path -split ';'
    if ($pathEntries -contains $InstallDir) {
        Write-Output '[jb installer] jb is already on PATH.'
    } else {
        Write-Output "[jb installer] Add it to this shell: `$env:Path = '$InstallDir;' + `$env:Path"
        Write-Output "[jb installer] Persist it for your user: [Environment]::SetEnvironmentVariable('Path', '$InstallDir;' + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
    }
} catch {
    Write-Error "[jb installer] $($_.Exception.Message)"
    exit 1
}
