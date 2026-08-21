Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
    Write-Output 'ok - Windows installer architecture tests are Windows-only'
    exit 0
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$installer = Join-Path $repoRoot 'install.ps1'
$tokens = $null
$parseErrors = $null
$installerAst = [System.Management.Automation.Language.Parser]::ParseFile(
    $installer,
    [ref]$tokens,
    [ref]$parseErrors
)
if ($parseErrors.Count -ne 0) {
    throw "install.ps1 did not parse: $($parseErrors -join '; ')"
}

foreach ($functionName in @('Resolve-WindowsArchitecture', 'Get-WindowsRuntimeArchitecture')) {
    $functionAst = $installerAst.Find(
        {
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq $functionName
        },
        $true
    )
    if ($null -eq $functionAst) {
        throw "Could not find $functionName in install.ps1"
    }
    Invoke-Expression $functionAst.Extent.Text
}

function Assert-Equal {
    param(
        [object]$Actual,
        [object]$Expected,
        [string]$Case
    )

    if ($Actual -cne $Expected) {
        throw "$Case expected '$Expected' but got '$Actual'"
    }
}

Assert-Equal `
    (Resolve-WindowsArchitecture 'X64' 'ARM64' 'AMD64') `
    'X64' `
    'RuntimeInformation takes precedence'
Assert-Equal `
    (Resolve-WindowsArchitecture $null 'ARM64' 'AMD64') `
    'Arm64' `
    'WOW64 reports native ARM64'
Assert-Equal `
    (Resolve-WindowsArchitecture $null 'AMD64' 'x86') `
    'X64' `
    'WOW64 reports native x64'
Assert-Equal `
    (Resolve-WindowsArchitecture $null $null 'AMD64') `
    'X64' `
    'process architecture is the final fallback'
Assert-Equal `
    (Resolve-WindowsArchitecture $null $null 'x64') `
    'X64' `
    'x64 alias is accepted'

$unsupportedFailed = $false
try {
    Resolve-WindowsArchitecture $null $null 'x86'
} catch {
    $unsupportedFailed = $_.Exception.Message -eq 'Unsupported Windows architecture: x86'
}
if (-not $unsupportedFailed) {
    throw 'unsupported architectures must fail explicitly'
}

$installerSource = Get-Content -LiteralPath $installer -Raw
if ($installerSource -match '\[System\.Runtime\.InteropServices\.RuntimeInformation\]::OSArchitecture') {
    throw 'install.ps1 must not directly access RuntimeInformation.OSArchitecture'
}

$runtimeArchitecture = Get-WindowsRuntimeArchitecture
$resolvedArchitecture = Resolve-WindowsArchitecture `
    $runtimeArchitecture `
    ([Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITEW6432')) `
    ([Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITECTURE'))
if ($resolvedArchitecture -notin @('X64', 'Arm64')) {
    throw "unexpected resolved architecture: $resolvedArchitecture"
}

Write-Output "ok - Windows installer architecture detection resolves $resolvedArchitecture and supports missing RuntimeInformation.OSArchitecture"
