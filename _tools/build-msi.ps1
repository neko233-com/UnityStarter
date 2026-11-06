param([string]$OutputDir = 'build/windows-amd64')
$ErrorActionPreference = 'Stop'
if (-not [Environment]::Is64BitProcess) {
    $native = Get-Command pwsh.exe -All | Where-Object { $_.Source -notmatch '\(x86\)' } | Select-Object -First 1 -ExpandProperty Source
    if (-not $native) { throw 'A 64-bit PowerShell 7 is required.' }
    & $native -NoProfile -File $PSCommandPath -OutputDir $OutputDir
    exit $LASTEXITCODE
}
$root = Split-Path -Parent $PSScriptRoot
$version = (Get-Content (Join-Path $root 'mygo.json') -Raw | ConvertFrom-Json).version
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "Invalid MSI version: $version" }
$output = [IO.Path]::GetFullPath((Join-Path $root $OutputDir))
$exe = Join-Path $output 'UnityStarter.exe'
if (-not (Test-Path -LiteralPath $exe)) { throw "Missing app executable: $exe" }
$msi = Join-Path $output "unitystarter-$version-amd64.msi"
Push-Location $root
try {
    dotnet tool restore
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    dotnet tool run wix build -arch x64 -d "Version=$version" -d "AppExe=$exe" -d "CliCmd=$(Join-Path $PSScriptRoot 'unitystarter.cmd')" -pdbtype none -o $msi (Join-Path $PSScriptRoot 'UnityStarter.wxs')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "Built $msi"
} finally { Pop-Location }
