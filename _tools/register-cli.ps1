# Registers or removes unitystarter for the current user (PATH, App Paths, Start menu).
# Used by the NSIS installer and for local/dev installs from build output.
param(
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [switch]$Unregister
)

$ErrorActionPreference = 'Stop'
$installDir = $InstallDir.TrimEnd('\')
$bin = Join-Path $installDir 'bin'

if ($Unregister) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not [string]::IsNullOrEmpty($userPath)) {
        $normalized = $bin.TrimEnd('\')
        $parts = $userPath -split ';' | Where-Object { $_ -and ($_.TrimEnd('\') -ine $normalized) }
        [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
    }
    foreach ($name in @('unitystarter.exe', 'UnityStarter.exe')) {
        Remove-Item -LiteralPath "HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\$name" -Recurse -Force -ErrorAction SilentlyContinue
    }
    $shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'unitystarter.lnk'
    Remove-Item -LiteralPath $shortcut -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $bin 'unitystarter.cmd') -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $bin) {
        Remove-Item -LiteralPath $bin -Force -ErrorAction SilentlyContinue
    }
    Write-Host "Unregistered unitystarter CLI for: $installDir"
    exit 0
}
$exe = Join-Path $installDir 'UnityStarter.exe'
if (-not (Test-Path -LiteralPath $exe)) {
    throw "UnityStarter.exe not found: $exe"
}

New-Item -ItemType Directory -Force -Path $bin | Out-Null
$cmd = Join-Path $bin 'unitystarter.cmd'
Set-Content -LiteralPath $cmd -Encoding ASCII -Value "@echo off`r`n`"%~dp0\..\UnityStarter.exe`" %*`r`n"

foreach ($name in @('unitystarter.exe', 'UnityStarter.exe')) {
    $key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\$name"
    New-Item -Path $key -Force | Out-Null
    Set-ItemProperty -LiteralPath $key -Name '(default)' -Value $exe
    Set-ItemProperty -LiteralPath $key -Name 'Path' -Value $installDir
}

$programs = [Environment]::GetFolderPath('Programs')
$shortcut = Join-Path $programs 'unitystarter.lnk'
$wsh = New-Object -ComObject WScript.Shell
$link = $wsh.CreateShortcut($shortcut)
$link.TargetPath = $exe
$link.WorkingDirectory = $installDir
$link.IconLocation = "$exe,0"
$link.Description = 'UnityStarter CLI and Hub'
$link.Save()

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$normalized = $bin.TrimEnd('\')
if ([string]::IsNullOrEmpty($userPath)) {
    [Environment]::SetEnvironmentVariable('Path', $normalized, 'User')
} elseif (($userPath -split ';' | ForEach-Object { $_.TrimEnd('\') }) -notcontains $normalized) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $normalized), 'User')
}

Write-Host "Registered unitystarter CLI for: $installDir"
