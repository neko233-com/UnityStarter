$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$version = (Get-Content (Join-Path $root 'mygo.json') -Raw | ConvertFrom-Json).version
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "Invalid MSI version: $version" }
$output = Join-Path $root 'build/windows-amd64'
$env:MSI_PATH = Join-Path $output "unitystarter-$version-amd64.msi"
$env:MSI_SHA256 = (Get-FileHash -LiteralPath $env:MSI_PATH -Algorithm SHA256).Hash
$env:EXE_SHA256 = (Get-FileHash -LiteralPath (Join-Path $output 'UnityStarter.exe') -Algorithm SHA256).Hash
$env:UNITYSTARTER_FORCE_CLOSE = '1'
& (Join-Path $PSScriptRoot 'install-msi.ps1')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
